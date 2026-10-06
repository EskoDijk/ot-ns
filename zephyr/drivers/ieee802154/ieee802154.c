/*
 * Copyright (c) 2026, The OTNS Authors.
 * All rights reserved.
 *
 * Redistribution and use in source and binary forms, with or without
 * modification, are permitted provided that the following conditions are met:
 * 1. Redistributions of source code must retain the above copyright
 *    notice, this list of conditions and the following disclaimer.
 * 2. Redistributions in binary form must reproduce the above copyright
 *    notice, this list of conditions and the following disclaimer in the
 *    documentation and/or other materials provided with the distribution.
 * 3. Neither the name of the copyright holder nor the
 *    names of its contributors may be used to endorse or promote products
 *    derived from this software without specific prior written permission.
 *
 * THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
 * AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
 * IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
 * ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
 * LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
 * CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
 * SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
 * INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
 * CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
 * ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
 * POSSIBILITY OF SUCH DAMAGE.
 *
 * Embedded (Zephyr CPU) side of the OTNS virtual IEEE 802.15.4 radio driver.
 *
 * This side implements the Zephyr ieee802154_radio_api and translates the radio
 * operations into calls to the runner ("bottom") side, which owns the real Unix
 * domain socket to OTNS. Received radio events are delivered from the runner
 * side through IEEE802154_OTNS_IRQ and processed by isr() (ieee802154_isr.c).
 */

#define DT_DRV_COMPAT zephyr_ieee802154_otns

#define LOG_MODULE_NAME ieee802154_otns
#if defined(CONFIG_IEEE802154_OTNS_LOG_LEVEL)
#define LOG_LEVEL CONFIG_IEEE802154_OTNS_LOG_LEVEL
#else
#define LOG_LEVEL LOG_LEVEL_INF
#endif

#include <zephyr/logging/log.h>
LOG_MODULE_REGISTER(LOG_MODULE_NAME);

#include <errno.h>
#include <string.h>

#include <zephyr/device.h>
#include <zephyr/irq.h>
#include <zephyr/kernel.h>
#include <zephyr/net/ieee802154_radio.h>
#include <zephyr/net/net_if.h>
#include <zephyr/net/net_pkt.h>
#include <zephyr/sys/byteorder.h>

#if defined(CONFIG_NET_L2_OPENTHREAD)
#include <zephyr/net/openthread.h>
#endif

#include "ieee802154_data.h"
#include "ieee802154_frame.h"
#include "ieee802154_isr.h"
#include "ieee802154_priv.h"
#include "radio.h"
#include <openthread/link.h>
#include <openthread/platform/otns.h>

#define CCA_TIMEOUT_MS 10

#define DEFAULT_NODE_ID 1

#define EUI64_BYTE_0 0x18
#define EUI64_BYTE_1 0xb4
#define EUI64_BYTE_2 0x30
#define EUI64_BYTE_3 0x00

#define ACK_ALLOWANCE_US 1000U

#define OPENTHREAD_MTU 1280

struct otns_radio_data data;

const struct device *radio_dev;

/* ------------------------------------------------------------------------- */
/* ieee802154_radio_api                                                      */
/* ------------------------------------------------------------------------- */

static enum ieee802154_hw_caps get_capabilities(const struct device *dev)
{
    ARG_UNUSED(dev);

    return IEEE802154_HW_FCS | IEEE802154_HW_FILTER | IEEE802154_HW_ENERGY_SCAN | IEEE802154_HW_TX_RX_ACK |
           IEEE802154_HW_RX_TX_ACK | IEEE802154_HW_TX_SEC;
}

static int energy_scan(const struct device *dev, uint16_t duration, energy_scan_done_cb_t done_cb)
{
    ARG_UNUSED(dev);
    ARG_UNUSED(duration);

    if (!data.started || !nsi_otns_bottom_is_connected())
    {
        return -EIO;
    }
    if (data.ed_scan_pending)
    {
        return -EBUSY;
    }

    data.ed_scan_pending = true;
    data.ed_done_cb      = done_cb;

    if (nsi_otns_bottom_cca(data.channel) < 0)
    {
        data.ed_scan_pending = false;
        data.ed_done_cb      = NULL;
        return -EIO;
    }

    return 0;
}

static int cca(const struct device *dev)
{
    ARG_UNUSED(dev);

    if (!data.started)
    {
        return -EIO;
    }
    if (!nsi_otns_bottom_is_connected())
    {
        return -EIO;
    }

    k_sem_reset(&data.cca_wait);
    data.cca_channel_free = true;

    if (nsi_otns_bottom_cca(data.channel) < 0)
    {
        return -EIO;
    }

    if (k_sem_take(&data.cca_wait, K_MSEC(CCA_TIMEOUT_MS)) != 0)
    {
        return 0;
    }

    return data.cca_channel_free ? 0 : -EBUSY;
}

static int set_channel(const struct device *dev, uint16_t channel)
{
    ARG_UNUSED(dev);

    if (channel < kMinChannel || channel > kMaxChannel)
    {
        return channel < kMinChannel ? -ENOTSUP : -EINVAL;
    }

    data.channel = (uint8_t)channel;

    if (data.started)
    {
        nsi_otns_bottom_set_state(OT_RADIO_STATE_RECEIVE, data.channel);
    }

    return 0;
}

static int filter(const struct device            *dev,
                  bool                            set,
                  enum ieee802154_filter_type     type,
                  const struct ieee802154_filter *filter)
{
    ARG_UNUSED(dev);

    if (!set)
    {
        return -ENOTSUP;
    }

    switch (type)
    {
    case IEEE802154_FILTER_TYPE_IEEE_ADDR:
    {
        uint8_t ext_addr_be[OT_EXT_ADDRESS_SIZE];

        memcpy(data.ext_addr, filter->ieee_addr, OT_EXT_ADDRESS_SIZE);

        for (int i = 0; i < OT_EXT_ADDRESS_SIZE; i++)
        {
            ext_addr_be[i] = data.ext_addr[OT_EXT_ADDRESS_SIZE - 1 - i];
        }
        nsi_otns_bottom_send_ext_addr(ext_addr_be);
        return 0;
    }
    case IEEE802154_FILTER_TYPE_SHORT_ADDR:
        sys_put_le16(filter->short_addr, data.short_addr);
        return 0;
    case IEEE802154_FILTER_TYPE_PAN_ID:
        sys_put_le16(filter->pan_id, data.pan_id);
        return 0;
    default:
        return -ENOTSUP;
    }
}

static int set_txpower(const struct device *dev, int16_t dbm)
{
    ARG_UNUSED(dev);

    data.txpower = (int8_t)dbm;
    return 0;
}

static void log_tx_frame(const uint8_t *psdu, uint16_t len)
{
    struct frame_addr_info tx_info;

    if (parse_frame(psdu, len, &tx_info) < 0 || tx_info.dst_off == FRAME_OFF_NONE)
    {
        return;
    }

    if (tx_info.dst_mode == ADDR_MODE_EXT)
    {
        uint8_t dst_ext_be[OT_EXT_ADDRESS_SIZE];

        for (int i = 0; i < OT_EXT_ADDRESS_SIZE; i++)
        {
            dst_ext_be[i] = psdu[tx_info.dst_off + OT_EXT_ADDRESS_SIZE - 1 - i];
        }
        LOG_DBG("TX seq %u type %d -> ext %02x%02x%02x%02x%02x%02x%02x%02x", tx_info.seq,
                tx_info.fcf & FCF_FRAME_TYPE_MASK, dst_ext_be[0], dst_ext_be[1], dst_ext_be[2], dst_ext_be[3],
                dst_ext_be[4], dst_ext_be[5], dst_ext_be[6], dst_ext_be[7]);
    }
    else if (tx_info.dst_mode == ADDR_MODE_SHORT)
    {
        LOG_DBG("TX seq %u type %d -> short 0x%04x", tx_info.seq, tx_info.fcf & FCF_FRAME_TYPE_MASK,
                sys_get_le16(&psdu[tx_info.dst_off]));
    }
}

static int tx_check_preconditions(enum ieee802154_tx_mode mode, uint16_t len)
{
    if (mode != IEEE802154_TX_MODE_DIRECT && mode != IEEE802154_TX_MODE_CCA)
    {
        LOG_ERR("TX mode %d not supported", mode);
        return -ENOTSUP;
    }

    if (!data.started || !nsi_otns_bottom_is_connected())
    {
        return -EIO;
    }

    if (len + FCS_SIZE > OT_RADIO_FRAME_MAX_SIZE)
    {
        return -EMSGSIZE;
    }

    return 0;
}

static int tx_build_psdu(struct net_buf *frag, uint16_t len, uint8_t *psdu)
{
    memcpy(psdu, frag->data, len);
    if (encrypt_tx_frame(psdu, len) < 0)
    {
        return -EIO;
    }

    uint16_t fcs = crc16(psdu, len);

    psdu[len]     = fcs & 0xff;
    psdu[len + 1] = fcs >> 8;

    log_tx_frame(psdu, len);

    return 0;
}

static int tx_send_frame(const uint8_t *psdu, uint16_t air_len)
{
    for (;;)
    {
        unsigned int key = irq_lock();

        if (!data.ack_tx_pending)
        {
            nsi_otns_bottom_set_state(OT_RADIO_STATE_TRANSMIT, data.channel);
            irq_unlock(key);
            break;
        }

        irq_unlock(key);
        k_sem_take(&data.ack_tx_done, K_FOREVER);
    }

    if (nsi_otns_bottom_tx(data.channel, data.txpower, psdu, air_len) < 0)
    {
        return -EIO;
    }

    nsi_otns_bottom_set_state(OT_RADIO_STATE_RECEIVE, data.channel);

    return 0;
}

static int tx_wait_for_result(uint16_t len)
{
    if (data.tx_wants_ack)
    {
        uint32_t frame_us =
            (OT_RADIO_SHR_PHR_LENGTH_BYTES + len + FCS_SIZE) * (OT_RADIO_SYMBOLS_PER_OCTET * OT_RADIO_SYMBOL_TIME);

        if (k_sem_take(&data.tx_wait, K_USEC(frame_us + ACK_ALLOWANCE_US)) != 0)
        {
            return -ENOMSG;
        }
    }
    else
    {
        (void)k_sem_take(&data.tx_wait, K_FOREVER);
    }

    return data.tx_result;
}

static void tx_deliver_ack(void)
{
    struct net_pkt *ack_pkt;
    uint16_t ack_mac_len = IS_ENABLED(CONFIG_IEEE802154_L2_PKT_INCL_FCS) ? data.ack_len : data.ack_len - FCS_SIZE;

    ack_pkt = net_pkt_rx_alloc_with_buffer(data.iface, ack_mac_len, NET_AF_UNSPEC, 0, K_NO_WAIT);
    if (ack_pkt == NULL)
    {
        return;
    }

    if (net_pkt_write(ack_pkt, data.ack_psdu, ack_mac_len) == 0)
    {
        net_pkt_set_ieee802154_lqi(ack_pkt, LQI_PERFECT);
        net_pkt_set_ieee802154_rssi_dbm(ack_pkt, 0);
        net_pkt_set_timestamp_ns(ack_pkt, k_ticks_to_ns_floor64(k_uptime_ticks()));
        net_pkt_cursor_init(ack_pkt);
        if (ieee802154_handle_ack(data.iface, ack_pkt) != NET_OK)
        {
            LOG_DBG("ACK not handled");
        }
        LOG_DBG("ACK for seq %u: fcf=0x%04x frame_pending=%d", data.tx_seq, sys_get_le16(data.ack_psdu),
                (data.ack_psdu[0] & FCF_BYTE0_FRAME_PENDING_BIT) != 0);
    }
    net_pkt_unref(ack_pkt);
}

static int tx_await_outcome(uint16_t len)
{
    int rc = tx_wait_for_result(len);

    if (rc != 0)
    {
        return rc;
    }

    if (!data.tx_wants_ack)
    {
        return 0;
    }

    if (data.ack_len == 0)
    {
        return -ENOMSG;
    }

    tx_deliver_ack();
    return 0;
}

static int tx(const struct device *dev, enum ieee802154_tx_mode mode, struct net_pkt *pkt, struct net_buf *frag)
{
    int      rc;
    uint16_t len = frag->len;

    ARG_UNUSED(pkt);

    rc = tx_check_preconditions(mode, len);
    if (rc != 0)
    {
        return rc;
    }

    if (mode == IEEE802154_TX_MODE_CCA)
    {
        rc = cca(dev);
        if (rc != 0)
        {
            return rc;
        }
    }

    uint8_t psdu[OT_RADIO_FRAME_MAX_SIZE];

    rc = tx_build_psdu(frag, len, psdu);
    if (rc != 0)
    {
        return rc;
    }

    data.tx_wants_ack = (len >= 1) && (frag->data[0] & FCF_BYTE0_ACK_REQ_BIT);
    data.tx_seq       = (len >= MIN_FRAME_SIZE) ? frag->data[2] : 0;
    data.ack_len      = 0;
    data.tx_result    = 0;

    k_sem_reset(&data.tx_wait);

    rc = tx_send_frame(psdu, len + FCS_SIZE);
    if (rc == 0)
    {
        rc = tx_await_outcome(len);
    }

    nsi_otns_bottom_set_state(OT_RADIO_STATE_RECEIVE, data.channel);
    return rc;
}

static int start(const struct device *dev)
{
    ARG_UNUSED(dev);

    if (data.started)
    {
        return -EALREADY;
    }

    data.started       = true;
    data.sleep_pending = false;
    nsi_otns_bottom_set_state(OT_RADIO_STATE_RECEIVE, data.channel);

    return 0;
}

static int stop(const struct device *dev)
{
    ARG_UNUSED(dev);

    if (!data.started)
    {
        return -EALREADY;
    }

    data.started = false;

    unsigned int key         = irq_lock();
    bool         ack_pending = data.ack_tx_pending;

    if (ack_pending)
    {
        data.sleep_pending = true;
    }
    irq_unlock(key);

    if (!ack_pending)
    {
        nsi_otns_bottom_set_state(OT_RADIO_STATE_SLEEP, data.channel);
    }

    return 0;
}

static int configure(const struct device *dev, enum ieee802154_config_type type, const struct ieee802154_config *config)
{
    ARG_UNUSED(dev);

    switch (type)
    {
    case IEEE802154_CONFIG_AUTO_ACK_FPB:
        data.auto_ack_fpb_enabled = config->auto_ack_fpb.enabled;
        return 0;

    case IEEE802154_CONFIG_ACK_FPB:
        if (config->ack_fpb.addr == NULL)
        {
            if (config->ack_fpb.extended)
            {
                fpb_ext_clear();
            }
            else
            {
                fpb_short_clear();
            }
            return 0;
        }
        if (config->ack_fpb.extended)
        {
            return config->ack_fpb.enabled ? fpb_ext_add(config->ack_fpb.addr) : fpb_ext_remove(config->ack_fpb.addr);
        }
        return config->ack_fpb.enabled ? fpb_short_add(sys_get_le16(config->ack_fpb.addr))
                                       : fpb_short_remove(sys_get_le16(config->ack_fpb.addr));

    case IEEE802154_CONFIG_CSL_PERIOD:
        data.csl_period = config->csl_period;
        return 0;

    case IEEE802154_CONFIG_EXPECTED_RX_TIME:
        data.csl_expected_rx_time_ns = config->expected_rx_time;
        return 0;

    case IEEE802154_CONFIG_ENH_ACK_HEADER_IE:
        return configure_enh_ack_ie(config);

    case IEEE802154_CONFIG_MAC_KEYS:
        set_mac_keys(config->mac_keys);
        return 0;

    case IEEE802154_CONFIG_FRAME_COUNTER:
        set_frame_counter(config->frame_counter, false);
        return 0;

    case IEEE802154_CONFIG_FRAME_COUNTER_IF_LARGER:
        set_frame_counter(config->frame_counter, true);
        return 0;

    default:
        return 0;
    }
}

IEEE802154_DEFINE_PHY_SUPPORTED_CHANNELS(drv_attr, kMinChannel, kMaxChannel);

static int attr_get(const struct device *dev, enum ieee802154_attr attr, struct ieee802154_attr_value *value)
{
    ARG_UNUSED(dev);

    return ieee802154_attr_get_channel_page_and_range(
        attr, IEEE802154_ATTR_PHY_CHANNEL_PAGE_ZERO_OQPSK_2450_BPSK_868_915, &drv_attr.phy_supported_channels, value);
}

static void get_mac(void)
{
    int node_id = nsi_otns_bottom_get_node_id();

    if (node_id <= 0)
    {
        node_id = DEFAULT_NODE_ID;
    }

    data.mac_addr[0] = EUI64_BYTE_0;
    data.mac_addr[1] = EUI64_BYTE_1;
    data.mac_addr[2] = EUI64_BYTE_2;
    data.mac_addr[3] = EUI64_BYTE_3;
    data.mac_addr[4] = (node_id >> 24) & 0xff;
    data.mac_addr[5] = (node_id >> 16) & 0xff;
    data.mac_addr[6] = (node_id >> 8) & 0xff;
    data.mac_addr[7] = node_id & 0xff;
}

static void iface_init(struct net_if *iface)
{
    get_mac();
    memcpy(data.ext_addr, data.mac_addr, OT_EXT_ADDRESS_SIZE);

    net_if_set_link_addr(iface, data.mac_addr, OT_EXT_ADDRESS_SIZE, NET_LINK_IEEE802154);

    data.iface = iface;
    radio_dev  = net_if_get_device(iface);

    ieee802154_init(iface);
}

static int init(const struct device *dev)
{
    ARG_UNUSED(dev);

    k_sem_init(&data.tx_wait, 0, 1);
    k_sem_init(&data.cca_wait, 0, 1);
    k_sem_init(&data.ack_tx_done, 0, 1);

    data.channel = kMinChannel;
    data.txpower = 0;
    data.started = false;

    IRQ_CONNECT(IEEE802154_OTNS_IRQ, 0, isr, NULL, 0);
    irq_enable(IEEE802154_OTNS_IRQ);

    LOG_INF("OTNS IEEE 802.15.4 driver initialized (node id %d)", nsi_otns_bottom_get_node_id());

    return 0;
}

static uint8_t get_sch_acc(const struct device *dev)
{
    ARG_UNUSED(dev);

    return nsi_otns_bottom_get_csl_accuracy();
}

static const struct ieee802154_radio_api radio_api = {
    .iface_api.init = iface_init,

    .get_capabilities = get_capabilities,
    .cca              = cca,
    .set_channel      = set_channel,
    .filter           = filter,
    .set_txpower      = set_txpower,
    .tx               = tx,
    .start            = start,
    .stop             = stop,
    .configure        = configure,
    .attr_get         = attr_get,
    .ed_scan          = energy_scan,
    .get_sch_acc      = get_sch_acc,
};

void otPlatOtnsStatus(const char *aStatus)
{
    if (aStatus == NULL)
    {
        return;
    }

    nsi_otns_bottom_send_status(aStatus, (uint16_t)strlen(aStatus));
}

#if defined(CONFIG_NET_L2_IEEE802154)
#define RADIO_L2 IEEE802154_L2
#define RADIO_L2_CTX_TYPE NET_L2_GET_CTX_TYPE(IEEE802154_L2)
#define RADIO_MTU IEEE802154_MTU
#elif defined(CONFIG_NET_L2_OPENTHREAD)
#define RADIO_L2 OPENTHREAD_L2
#define RADIO_L2_CTX_TYPE NET_L2_GET_CTX_TYPE(OPENTHREAD_L2)
#define RADIO_MTU OPENTHREAD_MTU
#elif defined(CONFIG_NET_L2_CUSTOM_IEEE802154)
#define RADIO_L2 CUSTOM_IEEE802154_L2
#define RADIO_L2_CTX_TYPE NET_L2_GET_CTX_TYPE(CUSTOM_IEEE802154_L2)
#define RADIO_MTU CONFIG_NET_L2_CUSTOM_IEEE802154_MTU
#endif

#if defined(CONFIG_NET_L2_PHY_IEEE802154)
NET_DEVICE_DT_INST_DEFINE(0,
                          init,
                          NULL,
                          &data,
                          NULL,
                          CONFIG_KERNEL_INIT_PRIORITY_DEFAULT,
                          &radio_api,
                          RADIO_L2,
                          RADIO_L2_CTX_TYPE,
                          RADIO_MTU);
#endif
