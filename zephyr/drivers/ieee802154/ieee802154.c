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
 *
 * Frame parsing / FPB list / Enhanced-ACK building live in ieee802154_frame.c;
 * RX delivery and ISR/event handling live in ieee802154_isr.c; shared state is
 * declared in ieee802154_data.h.
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

/*
 * This driver only ever instantiates one device (NET_DEVICE_DT_INST_DEFINE(0, ...) below), so
 * `data` is accessed directly everywhere (ISR included) rather than via dev->data indirection.
 */
struct otns_radio_data data;

const struct device *radio_dev;



/* ------------------------------------------------------------------------- */
/* ieee802154_radio_api                                                      */
/* ------------------------------------------------------------------------- */

static enum ieee802154_hw_caps get_capabilities(const struct device *dev)
{
    ARG_UNUSED(dev);

    return IEEE802154_HW_FCS | IEEE802154_HW_FILTER | IEEE802154_HW_ENERGY_SCAN | IEEE802154_HW_TX_RX_ACK |
           IEEE802154_HW_RX_TX_ACK;
}

static int energy_scan(const struct device *dev, uint16_t duration, energy_scan_done_cb_t done_cb)
{
    /* The simulated radio reports a single sample per request; requested scan duration is not honored. */
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
        LOG_INF("TX seq %u type %d -> ext %02x%02x%02x%02x%02x%02x%02x%02x", tx_info.seq,
                tx_info.fcf & FCF_FRAME_TYPE_MASK, dst_ext_be[0], dst_ext_be[1], dst_ext_be[2], dst_ext_be[3],
                dst_ext_be[4], dst_ext_be[5], dst_ext_be[6], dst_ext_be[7]);
    }
    else if (tx_info.dst_mode == ADDR_MODE_SHORT)
    {
        LOG_INF("TX seq %u type %d -> short 0x%04x", tx_info.seq, tx_info.fcf & FCF_FRAME_TYPE_MASK,
                sys_get_le16(&psdu[tx_info.dst_off]));
    }
}

static int tx(const struct device *dev, enum ieee802154_tx_mode mode, struct net_pkt *pkt, struct net_buf *frag)
{
    uint8_t  psdu[OT_RADIO_FRAME_MAX_SIZE];
    uint16_t len = frag->len;
    uint16_t fcs;
    int      ret;
    int      rc = 0;

    ARG_UNUSED(pkt);

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

    if (mode == IEEE802154_TX_MODE_CCA)
    {
        ret = cca(dev);
        if (ret != 0)
        {
            return ret;
        }
    }

    memcpy(psdu, frag->data, len);
    fcs           = crc16(psdu, len);
    psdu[len]     = fcs & 0xff;
    psdu[len + 1] = fcs >> 8;

    log_tx_frame(psdu, len);

    data.tx_wants_ack = (len >= 1) && (frag->data[0] & FCF_BYTE0_ACK_REQ_BIT);
    data.tx_seq       = (len >= MIN_FRAME_SIZE) ? frag->data[2] : 0;
    data.ack_len      = 0;
    data.tx_result    = 0;

    k_sem_reset(&data.tx_wait);

    /*
     * A previously received frame may still have a deferred auto-ACK armed
     * (schedule_ack()/nsi_otns_bottom_tx_after()), which transmits on its own
     * timer independently of this thread. Starting our own TX while that ACK
     * is still outstanding would make this node issue two overlapping
     * RADIO_COMM_START events, which OTNS's radio model does not expect from
     * a single node and reports as an internal error. Wait for it to finish
     * first; the irq_lock() makes the pending-check/clear-to-send atomic with
     * respect to a new ACK being armed from the ISR in between.
     */
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

    ret = nsi_otns_bottom_tx(data.channel, data.txpower, psdu, len + FCS_SIZE);
    if (ret < 0)
    {
        rc = -EIO;
        goto out;
    }

    nsi_otns_bottom_set_state(OT_RADIO_STATE_RECEIVE, data.channel);

    if (data.tx_wants_ack)
    {
        uint32_t frame_us =
            (OT_RADIO_SHR_PHR_LENGTH_BYTES + len + FCS_SIZE) * (OT_RADIO_SYMBOLS_PER_OCTET * OT_RADIO_SYMBOL_TIME);

        if (k_sem_take(&data.tx_wait, K_USEC(frame_us + ACK_ALLOWANCE_US)) != 0)
        {
            rc = -ENOMSG;
            goto out;
        }
    }
    else
    {
        (void)k_sem_take(&data.tx_wait, K_FOREVER);
    }

    if (data.tx_result != 0)
    {
        rc = data.tx_result;
        goto out;
    }

    if (data.tx_wants_ack)
    {
        if (data.ack_len == 0)
        {
            rc = -ENOMSG;
            goto out;
        }

        struct net_pkt *ack_pkt;
        uint16_t ack_mac_len = IS_ENABLED(CONFIG_IEEE802154_L2_PKT_INCL_FCS) ? data.ack_len : data.ack_len - FCS_SIZE;

        ack_pkt = net_pkt_rx_alloc_with_buffer(data.iface, ack_mac_len, NET_AF_UNSPEC, 0, K_NO_WAIT);
        if (ack_pkt != NULL)
        {
            if (net_pkt_write(ack_pkt, data.ack_psdu, ack_mac_len) == 0)
            {
                net_pkt_set_ieee802154_lqi(ack_pkt, LQI_PERFECT);
                net_pkt_set_ieee802154_rssi_dbm(ack_pkt, 0);
                net_pkt_cursor_init(ack_pkt);
                if (ieee802154_handle_ack(data.iface, ack_pkt) != NET_OK)
                {
                    LOG_DBG("ACK not handled");
                }
                /* Frame Pending bit tells a polling child whether the parent has buffered data. */
                LOG_INF("ACK for seq %u: fcf=0x%04x frame_pending=%d", data.tx_seq, sys_get_le16(data.ack_psdu),
                        (data.ack_psdu[0] & FCF_BYTE0_FRAME_PENDING_BIT) != 0);
            }
            net_pkt_unref(ack_pkt);
        }
    }

out:
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
    unsigned int key;
    bool         ack_pending;

    ARG_UNUSED(dev);

    if (!data.started)
    {
        return -EALREADY;
    }

    data.started = false;

    /* irq_lock() makes the check-and-set atomic with respect to isr() clearing ack_tx_pending. */
    key         = irq_lock();
    ack_pending = data.ack_tx_pending;
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

    return RFSIM_CSL_ACCURACY_DEFAULT_PPM;
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
