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
 * side through IEEE802154_OTNS_IRQ and processed by otns_isr().
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

#include "ieee802154_priv.h"
#include "radio.h"
#include <openthread/link.h>
#include <openthread/platform/otns.h>
#include <openthread/platform/time.h>

/* IEEE 802.15.4 MAC framing constants. */
#define FCS_SIZE 2
#define FCF_SIZE 2
#define MIN_FRAME_SIZE 3
#define SEQ_NUM_SIZE 1
#define SHORT_ADDR_SIZE 2
#define PAN_ID_SIZE sizeof(otPanId)
#define ACK_FRAME_SIZE 5
#define FCF_FRAME_TYPE_MASK 0x0007
#define FCF_FRAME_TYPE_ACK 0x0002
#define FCF_ACK_CONTROL_BYTE_0 0x02
#define FCF_ACK_CONTROL_BYTE_1 0x00
#define FCF_FRAME_PENDING_BIT 0x0010
#define FCF_ACK_REQ_BIT 0x0020
#define FCF_PAN_COMPR_BIT 0x0040
#define FCF_DST_MODE_MASK 0x0c00
#define FCF_DST_MODE_SHIFT 10
#define FCF_SRC_MODE_MASK 0xc000
#define FCF_SRC_MODE_SHIFT 14
#define FCF_VERSION_MASK 0x3000
#define FCF_VERSION_SHIFT 12
#define FCF_SEQ_SUPPR_BIT 0x0100
#define IEEE802154_VERSION_2015 2
#define CRC_POLY_KERMIT 0x8408U
#define CRC_INIT 0

#define ADDR_MODE_NONE 0
#define ADDR_MODE_SHORT 2
#define ADDR_MODE_EXT 3

/* Header IE (Information Element), IEEE 802.15.4-2015 section 7.4.2.1. */
#define HEADER_IE_LEN_MASK 0x7f
#define HEADER_IE_ID_SHIFT 7
#define FCF_IE_PRESENT_BIT 0x0080 /* bit 7 */
#define HEADER_IE_ID_CSL 0x1a

/* Thread vendor-specific Enhanced-ACK Probing (Link Metrics) IE, Thread 1.2 4.11.3.4.4.6. */
#define LM_TOKEN_RSSI 0x01
#define LM_TOKEN_MARGIN 0x02
#define LM_TOKEN_LQI 0x03
/* Vendor IE layout: content[0..2]=OUI, [3]=subtype, [4..]=LM_TOKEN_* placeholders. */
#define LM_VENDOR_IE_TOKEN_OFFSET 4
/* Thread Link Metrics dBm-to-byte linear mapping range (Thread 1.2 4.11.3.4.4.6). */
#define LM_METRIC_RANGE_DBM 130

/* Max stored Enhanced-ACK header IE templates (IEEE802154_CONFIG_ENH_ACK_HEADER_IE). */
#define MAX_ACK_IES 4
#define ACK_IE_MAX_CONTENT (OT_ACK_IE_MAX_SIZE - 2)

#define LQI_PERFECT 255

#define CCA_TIMEOUT_MS 10

#define MAX_FPB_ENTRIES 32

#define DEFAULT_NODE_ID 1

#define EUI64_BYTE_0 0x18
#define EUI64_BYTE_1 0xb4
#define EUI64_BYTE_2 0x30
#define EUI64_BYTE_3 0x00

#define AIFS_TURNAROUND_US ((uint32_t)OT_RADIO_AIFS_TIME_US)

#define ACK_ALLOWANCE_US 1000U

#define OPENTHREAD_MTU 1280

struct enh_ack_ie
{
    bool     valid;
    uint8_t  element_id;
    uint8_t  content_len;
    uint8_t  content[ACK_IE_MAX_CONTENT];
    bool     has_short_filter;
    uint16_t short_addr;
    bool     has_ext_filter;
    uint8_t  ext_addr_be[OT_EXT_ADDRESS_SIZE];
};

struct ctx
{
    struct net_if *iface;
    uint8_t        mac_addr[8];

    struct k_sem tx_wait;
    struct k_sem cca_wait;
    struct k_sem ack_tx_done; /* given when a pending deferred auto-ACK transmission completes */

    volatile int  tx_result;
    volatile bool cca_channel_free;

    bool              tx_wants_ack;
    uint8_t           tx_seq;
    uint8_t           ack_psdu[OT_RADIO_FRAME_MAX_SIZE];
    volatile uint16_t ack_len;

    uint8_t pan_id[2];
    uint8_t short_addr[2];
    uint8_t ext_addr[8];

    bool     auto_ack_fpb_enabled;
    uint16_t fpb_short[MAX_FPB_ENTRIES];
    uint8_t  fpb_short_count;
    uint8_t  fpb_ext[MAX_FPB_ENTRIES][OT_EXT_ADDRESS_SIZE];
    uint8_t  fpb_ext_count;

    bool                  ed_scan_pending;
    energy_scan_done_cb_t ed_done_cb;

    bool ack_tx_pending;
    bool sleep_pending;

    uint32_t csl_period;
    int64_t  csl_expected_rx_time_ns;

    struct enh_ack_ie ack_ies[MAX_ACK_IES];

    uint8_t channel;
    int8_t  txpower;
    bool    started;
};

static struct ctx data;

static const struct device *radio_dev;

static uint16_t crc16(const uint8_t *data, size_t len)
{
    uint16_t crc = CRC_INIT;

    for (size_t i = 0; i < len; i++)
    {
        crc ^= data[i];
        for (int b = 0; b < 8; b++)
        {
            crc = (crc & 1U) ? (crc >> 1) ^ CRC_POLY_KERMIT : (crc >> 1);
        }
    }

    return crc;
}

static int err_to_errno(uint8_t error)
{
    switch (error)
    {
    case OT_ERROR_NONE:
        return 0;
    case OT_ERROR_CHANNEL_ACCESS_FAILURE:
        return -EBUSY;
    case OT_ERROR_ABORT:
        return -EIO;
    default:
        return -EIO;
    }
}

struct frame_addr_info
{
    uint16_t fcf;
    int      version;
    uint8_t  seq;
    int      dst_mode;
    int      dst_off;
    bool     dst_pan_present;
    uint16_t dst_pan;
    int      src_mode;
    int      src_off;
    bool     src_pan_present;
    uint16_t src_pan;
};

static int parse_frame(const uint8_t *psdu, uint16_t len, struct frame_addr_info *info)
{
    int off;

    if (len < MIN_FRAME_SIZE)
    {
        return -1;
    }

    memset(info, 0, sizeof(*info));
    info->fcf      = sys_get_le16(psdu);
    info->version  = (info->fcf & FCF_VERSION_MASK) >> FCF_VERSION_SHIFT;
    info->dst_mode = (info->fcf & FCF_DST_MODE_MASK) >> FCF_DST_MODE_SHIFT;
    info->src_mode = (info->fcf & FCF_SRC_MODE_MASK) >> FCF_SRC_MODE_SHIFT;
    info->dst_off  = -1;
    info->src_off  = -1;

    off = FCF_SIZE;
    if (!(info->version == IEEE802154_VERSION_2015 && (info->fcf & FCF_SEQ_SUPPR_BIT)))
    {
        if (off >= len)
        {
            return -1;
        }
        info->seq = psdu[off];
        off += SEQ_NUM_SIZE;
    }

    if (info->dst_mode != ADDR_MODE_NONE)
    {
        if (off + PAN_ID_SIZE > len)
        {
            return -1;
        }
        info->dst_pan         = sys_get_le16(&psdu[off]);
        info->dst_pan_present = true;
        off += PAN_ID_SIZE;

        info->dst_off = off;
        off += (info->dst_mode == ADDR_MODE_EXT) ? OT_EXT_ADDRESS_SIZE : SHORT_ADDR_SIZE;
        if (off > len)
        {
            return -1;
        }
    }

    if (info->src_mode != ADDR_MODE_NONE)
    {
        if (!(info->fcf & FCF_PAN_COMPR_BIT))
        {
            if (off + PAN_ID_SIZE > len)
            {
                return -1;
            }
            info->src_pan         = sys_get_le16(&psdu[off]);
            info->src_pan_present = true;
            off += PAN_ID_SIZE;
        }
        info->src_off = off;
        off += (info->src_mode == ADDR_MODE_EXT) ? OT_EXT_ADDRESS_SIZE : SHORT_ADDR_SIZE;
        if (off > len)
        {
            return -1;
        }
    }

    return 0;
}

static bool frame_is_for_me(const uint8_t *psdu, uint16_t len, struct frame_addr_info *info)
{
    if (parse_frame(psdu, len, info) < 0)
    {
        return false;
    }

    if (info->dst_mode == ADDR_MODE_SHORT)
    {
        if (info->dst_off + SHORT_ADDR_SIZE > len)
        {
            return false;
        }
        if (sys_get_le16(&psdu[info->dst_off]) == OT_RADIO_BROADCAST_SHORT_ADDR)
        {
            return false;
        }
        return memcmp(&psdu[info->dst_off], data.short_addr, SHORT_ADDR_SIZE) == 0;
    }

    if (info->dst_mode == ADDR_MODE_EXT)
    {
        if (info->dst_off + OT_EXT_ADDRESS_SIZE > len)
        {
            return false;
        }
        return memcmp(&psdu[info->dst_off], data.ext_addr, OT_EXT_ADDRESS_SIZE) == 0;
    }

    return false;
}

static bool fpb_short_contains(uint16_t addr)
{
    for (int i = 0; i < data.fpb_short_count; i++)
    {
        if (data.fpb_short[i] == addr)
        {
            return true;
        }
    }
    return false;
}

static bool fpb_ext_contains(const uint8_t *addr)
{
    for (int i = 0; i < data.fpb_ext_count; i++)
    {
        if (memcmp(data.fpb_ext[i], addr, OT_EXT_ADDRESS_SIZE) == 0)
        {
            return true;
        }
    }
    return false;
}

static int fpb_short_add(uint16_t addr)
{
    if (fpb_short_contains(addr))
    {
        return 0;
    }
    if (data.fpb_short_count >= MAX_FPB_ENTRIES)
    {
        return -ENOMEM;
    }
    data.fpb_short[data.fpb_short_count++] = addr;
    return 0;
}

static int fpb_short_remove(uint16_t addr)
{
    for (int i = 0; i < data.fpb_short_count; i++)
    {
        if (data.fpb_short[i] == addr)
        {
            data.fpb_short[i] = data.fpb_short[--data.fpb_short_count];
            return 0;
        }
    }
    return -ENOENT;
}

static int fpb_ext_add(const uint8_t *addr)
{
    if (fpb_ext_contains(addr))
    {
        return 0;
    }
    if (data.fpb_ext_count >= MAX_FPB_ENTRIES)
    {
        return -ENOMEM;
    }
    memcpy(data.fpb_ext[data.fpb_ext_count++], addr, OT_EXT_ADDRESS_SIZE);
    return 0;
}

static int fpb_ext_remove(const uint8_t *addr)
{
    for (int i = 0; i < data.fpb_ext_count; i++)
    {
        if (memcmp(data.fpb_ext[i], addr, OT_EXT_ADDRESS_SIZE) == 0)
        {
            memcpy(data.fpb_ext[i], data.fpb_ext[--data.fpb_ext_count], OT_EXT_ADDRESS_SIZE);
            return 0;
        }
    }
    return -ENOENT;
}

static bool frame_pending_for(const uint8_t *psdu, uint16_t len, const struct frame_addr_info *info)
{
    if (!data.auto_ack_fpb_enabled)
    {
        return true;
    }

    if (info->src_mode == ADDR_MODE_SHORT)
    {
        if (info->src_off + SHORT_ADDR_SIZE > len)
        {
            return false;
        }
        return fpb_short_contains(sys_get_le16(&psdu[info->src_off]));
    }

    if (info->src_mode == ADDR_MODE_EXT)
    {
        if (info->src_off + OT_EXT_ADDRESS_SIZE > len)
        {
            return false;
        }
        return fpb_ext_contains(&psdu[info->src_off]);
    }

    return false;
}

static uint16_t csl_phase(void)
{
    uint32_t period_us = data.csl_period * OT_US_PER_TEN_SYMBOLS;
    uint32_t phr_us    = OT_RADIO_SYMBOLS_PER_OCTET * OT_RADIO_SYMBOL_TIME;
    uint32_t anchor_us = (uint32_t)(data.csl_expected_rx_time_ns / NSEC_PER_USEC) + phr_us;
    uint32_t tx_mhr_us = (uint32_t)otPlatTimeGet() + AIFS_TURNAROUND_US + OT_RADIO_SHR_PHR_DURATION_US;
    uint32_t diff      = ((anchor_us % period_us) - (tx_mhr_us % period_us) + period_us) % period_us;

    if (diff % OT_US_PER_TEN_SYMBOLS > 0)
    {
        diff += OT_US_PER_TEN_SYMBOLS;
    }
    return (uint16_t)(diff / OT_US_PER_TEN_SYMBOLS);
}

static bool ack_ie_matches_dst(const struct enh_ack_ie *ie, const uint8_t *psdu, const struct frame_addr_info *info)
{
    if (!ie->has_short_filter && !ie->has_ext_filter)
    {
        return true;
    }
    if (ie->has_short_filter && info->src_mode == ADDR_MODE_SHORT &&
        sys_get_le16(&psdu[info->src_off]) == ie->short_addr)
    {
        return true;
    }
    if (ie->has_ext_filter && info->src_mode == ADDR_MODE_EXT)
    {
        uint8_t ext_be[OT_EXT_ADDRESS_SIZE];

        for (int i = 0; i < OT_EXT_ADDRESS_SIZE; i++)
        {
            ext_be[i] = psdu[info->src_off + OT_EXT_ADDRESS_SIZE - 1 - i];
        }
        return memcmp(ext_be, ie->ext_addr_be, OT_EXT_ADDRESS_SIZE) == 0;
    }
    return false;
}

static uint16_t build_enh_ack_ies(const uint8_t                *rx_psdu,
                                  const struct frame_addr_info *info,
                                  int8_t                        rssi,
                                  uint8_t                      *out,
                                  uint16_t                      out_max)
{
    uint16_t total = 0;

    for (int i = 0; i < MAX_ACK_IES; i++)
    {
        struct enh_ack_ie *ie = &data.ack_ies[i];
        uint8_t            content[ACK_IE_MAX_CONTENT];
        uint16_t           hdr;

        if (!ie->valid || !ack_ie_matches_dst(ie, rx_psdu, info))
        {
            continue;
        }
        if (ie->element_id == HEADER_IE_ID_CSL && data.csl_period == 0)
        {
            continue;
        }
        if ((uint16_t)(total + sizeof(hdr) + ie->content_len) > out_max)
        {
            break;
        }

        memcpy(content, ie->content, ie->content_len);

        if (ie->element_id == HEADER_IE_ID_CSL)
        {
            sys_put_le16(csl_phase(), &content[0]); /* content[2..3] = period, unchanged */
        }
        else
        {
            /* Vendor IE: content[0..2]=OUI, [3]=subtype, [4..]=LM_TOKEN_* placeholders. */
            for (int j = LM_VENDOR_IE_TOKEN_OFFSET; j < ie->content_len; j++)
            {
                if (content[j] == LM_TOKEN_LQI)
                {
                    content[j] = LQI_PERFECT;
                }
                else if (content[j] == LM_TOKEN_RSSI)
                {
                    int r = rssi < -LM_METRIC_RANGE_DBM ? -LM_METRIC_RANGE_DBM : (rssi > 0 ? 0 : rssi);

                    content[j] = (uint8_t)((r + LM_METRIC_RANGE_DBM) * UINT8_MAX / LM_METRIC_RANGE_DBM);
                }
                else if (content[j] == LM_TOKEN_MARGIN)
                {
                    int margin = (int)rssi - RFSIM_RX_SENSITIVITY_DEFAULT_DBM;

                    margin     = margin < 0 ? 0 : (margin > LM_METRIC_RANGE_DBM ? LM_METRIC_RANGE_DBM : margin);
                    content[j] = (uint8_t)(margin * UINT8_MAX / LM_METRIC_RANGE_DBM);
                }
            }
        }

        hdr = (ie->content_len & HEADER_IE_LEN_MASK) | ((uint16_t)ie->element_id << HEADER_IE_ID_SHIFT);
        sys_put_le16(hdr, &out[total]);
        memcpy(&out[total + sizeof(hdr)], content, ie->content_len);
        total += sizeof(hdr) + ie->content_len;
    }

    return total;
}

static int build_enh_ack(const uint8_t                *rx_psdu,
                         uint16_t                      rx_len,
                         const struct frame_addr_info *info,
                         int8_t                        rssi,
                         uint8_t                      *ack,
                         uint16_t                     *ack_len_out)
{
    uint16_t fcf;
    uint16_t off = FCF_SIZE + SEQ_NUM_SIZE;
    uint16_t dst_pan;
    uint16_t ie_len;
    uint16_t fcs;
    uint8_t  addr_len;

    if (info->src_mode == ADDR_MODE_NONE || info->src_off < 0)
    {
        return -1;
    }
    if (!info->src_pan_present && !info->dst_pan_present)
    {
        return -1;
    }
    dst_pan  = info->src_pan_present ? info->src_pan : info->dst_pan;
    addr_len = (info->src_mode == ADDR_MODE_EXT) ? OT_EXT_ADDRESS_SIZE : SHORT_ADDR_SIZE;

    fcf = FCF_FRAME_TYPE_ACK | ((uint16_t)IEEE802154_VERSION_2015 << FCF_VERSION_SHIFT) |
          ((uint16_t)info->src_mode << FCF_DST_MODE_SHIFT);
    if (frame_pending_for(rx_psdu, rx_len, info))
    {
        fcf |= FCF_FRAME_PENDING_BIT;
    }

    ack[FCF_SIZE] = info->seq;
    sys_put_le16(dst_pan, &ack[off]);
    off += PAN_ID_SIZE;
    memcpy(&ack[off], &rx_psdu[info->src_off], addr_len);
    off += addr_len;

    ie_len = build_enh_ack_ies(rx_psdu, info, rssi, &ack[off], (uint16_t)(OT_RADIO_FRAME_MAX_SIZE - off - FCS_SIZE));
    if (ie_len > 0)
    {
        fcf |= FCF_IE_PRESENT_BIT;
    }
    off += ie_len;

    sys_put_le16(fcf, &ack[0]);

    fcs          = crc16(ack, off);
    ack[off]     = fcs & 0xff;
    ack[off + 1] = fcs >> 8;
    *ack_len_out = off + FCS_SIZE;
    return 0;
}

static void schedule_ack(const uint8_t *rx_psdu, uint16_t rx_len, int8_t rssi, const struct frame_addr_info *info)
{
    uint8_t  ack[OT_RADIO_FRAME_MAX_SIZE];
    uint16_t ack_len;
    uint16_t fcs;

    if (info->version == IEEE802154_VERSION_2015)
    {
        if (build_enh_ack(rx_psdu, rx_len, info, rssi, ack, &ack_len) < 0)
        {
            return;
        }
    }
    else
    {
        ack[0] = FCF_ACK_CONTROL_BYTE_0; /* FCF: ACK */
        if (frame_pending_for(rx_psdu, rx_len, info))
        {
            ack[0] |= FCF_FRAME_PENDING_BIT;
        }
        ack[1] = FCF_ACK_CONTROL_BYTE_1;
        ack[2] = info->seq;

        fcs     = crc16(ack, MIN_FRAME_SIZE);
        ack[3]  = fcs & 0xff;
        ack[4]  = fcs >> 8;
        ack_len = ACK_FRAME_SIZE;
    }

    if (nsi_otns_bottom_tx_after(data.channel, data.txpower, ack, ack_len, AIFS_TURNAROUND_US) == 0)
    {
        data.ack_tx_pending = true;
    }
}

static void deliver_rx(const struct otns_radio_event *ev, bool acked_with_fpb)
{
    struct net_pkt *pkt;
    uint16_t        mac_len;

    if (ev->psdu_len < FCS_SIZE)
    {
        return;
    }

    if (IS_ENABLED(CONFIG_IEEE802154_L2_PKT_INCL_FCS))
    {
        mac_len = ev->psdu_len;
    }
    else
    {
        mac_len = ev->psdu_len - FCS_SIZE;
    }

    pkt = net_pkt_rx_alloc_with_buffer(data.iface, mac_len, NET_AF_UNSPEC, 0, K_NO_WAIT);
    if (pkt == NULL)
    {
        LOG_WRN("No RX pkt available, dropping frame");
        return;
    }

    if (net_pkt_write(pkt, ev->psdu, mac_len) < 0)
    {
        LOG_WRN("Failed to write RX frame");
        net_pkt_unref(pkt);
        return;
    }

    net_pkt_set_ieee802154_lqi(pkt, LQI_PERFECT);
    net_pkt_set_ieee802154_rssi_dbm(pkt, ev->data.mPower);
    /* Tell OT whether the ACK we (auto-)sent for this frame had Frame Pending set,
     * so DataPollHandler::HandleDataPoll() actually triggers the indirect Tx. */
    net_pkt_set_ieee802154_ack_fpb(pkt, acked_with_fpb);

    if (net_recv_data(data.iface, pkt) < 0)
    {
        LOG_DBG("RX frame dropped by net stack");
        net_pkt_unref(pkt);
    }
}

static void handle_rx(const struct otns_radio_event *ev)
{
    struct frame_addr_info info;
    uint16_t               fcf;
    bool                   for_me;

    if (ev->psdu_len < MIN_FRAME_SIZE)
    {
        return;
    }

    fcf = sys_get_le16(ev->psdu);

    if ((fcf & FCF_FRAME_TYPE_MASK) == FCF_FRAME_TYPE_ACK)
    {
        if (data.tx_wants_ack && ev->psdu[2] == data.tx_seq)
        {
            uint16_t l = ev->psdu_len;

            if (l > sizeof(data.ack_psdu))
            {
                l = sizeof(data.ack_psdu);
            }
            memcpy(data.ack_psdu, ev->psdu, l);
            data.ack_len = l;
            k_sem_give(&data.tx_wait);
        }
        return;
    }

    for_me = frame_is_for_me(ev->psdu, ev->psdu_len, &info);

    if (for_me && (info.fcf & FCF_ACK_REQ_BIT))
    {
        bool fpb = frame_pending_for(ev->psdu, ev->psdu_len, &info);

        deliver_rx(ev, fpb);
        schedule_ack(ev->psdu, ev->psdu_len, ev->data.mPower, &info);
    }
    else
    {
        deliver_rx(ev, false);
    }
}

/*
 * Applies the runner side's clock-drift-derived alarm offset (see
 * ieee802154_otns_bottom.c's update_alarm_drift_offset()) to OpenThread's own
 * notion of elapsed time. alarm_milli_set_time_offset_ms() lives in Zephyr's
 * OpenThread platform layer (zephyr/modules/openthread/platform/alarm_milli.c)
 * and can only be called from the embedded side: it is otherwise unreferenced
 * in our build (none of our Kconfig options enable its other callers in
 * alarm_micro.c/alarm_counter.c), so the embedded image's own link would
 * discard it before the runner side ever gets a chance to call it.
 */
extern void alarm_milli_set_time_offset_ms(int32_t offset_ms);

static void apply_pending_drift_offset(void)
{
    static int32_t last_offset_ms;
    int32_t        offset_ms = nsi_otns_bottom_get_drift_offset_ms();

    if (offset_ms != last_offset_ms)
    {
        last_offset_ms = offset_ms;
        alarm_milli_set_time_offset_ms(offset_ms);
    }
}

static void isr(const void *arg)
{
    struct otns_radio_event ev;

    ARG_UNUSED(arg);

    apply_pending_drift_offset();

    while (nsi_otns_bottom_get_event(&ev))
    {
        switch (ev.type)
        {
        case OT_SIM_EVENT_RADIO_RX_DONE:
            handle_rx(&ev);
            break;

        case OT_SIM_EVENT_RADIO_TX_DONE:
            if (data.ack_tx_pending)
            {
                data.ack_tx_pending = false;
                if (data.sleep_pending)
                {
                    data.sleep_pending = false;
                    nsi_otns_bottom_set_state(OT_RADIO_STATE_SLEEP, data.channel);
                }
                k_sem_give(&data.ack_tx_done);
                break;
            }
            data.tx_result = err_to_errno(ev.data.mError);
            if (!data.tx_wants_ack || data.tx_result != 0)
            {
                k_sem_give(&data.tx_wait);
            }
            break;

        case OT_SIM_EVENT_RADIO_CHAN_SAMPLE:
            if (data.ed_scan_pending)
            {
                energy_scan_done_cb_t cb = data.ed_done_cb;

                data.ed_scan_pending = false;
                data.ed_done_cb      = NULL;
                if (cb != NULL)
                {
                    cb(radio_dev, ev.data.mPower);
                }
                break;
            }
            data.cca_channel_free = (ev.data.mPower == (int8_t)OT_RADIO_RSSI_INVALID) ||
                                    (ev.data.mPower < RFSIM_CCA_ED_THRESHOLD_DEFAULT_DBM);
            k_sem_give(&data.cca_wait);
            break;

        default:
            break;
        }
    }
}

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
    struct ctx *ctx = dev->data;

    ARG_UNUSED(duration);

    if (!ctx->started || !nsi_otns_bottom_is_connected())
    {
        return -EIO;
    }
    if (ctx->ed_scan_pending)
    {
        return -EBUSY;
    }

    ctx->ed_scan_pending = true;
    ctx->ed_done_cb      = done_cb;

    if (nsi_otns_bottom_cca(ctx->channel) < 0)
    {
        ctx->ed_scan_pending = false;
        ctx->ed_done_cb      = NULL;
        return -EIO;
    }

    return 0;
}

static int cca(const struct device *dev)
{
    struct ctx *ctx = dev->data;

    if (!ctx->started)
    {
        return -EIO;
    }
    if (!nsi_otns_bottom_is_connected())
    {
        return -EIO;
    }

    k_sem_reset(&ctx->cca_wait);
    ctx->cca_channel_free = true;

    if (nsi_otns_bottom_cca(ctx->channel) < 0)
    {
        return -EIO;
    }

    if (k_sem_take(&ctx->cca_wait, K_MSEC(CCA_TIMEOUT_MS)) != 0)
    {
        return 0;
    }

    return ctx->cca_channel_free ? 0 : -EBUSY;
}

static int set_channel(const struct device *dev, uint16_t channel)
{
    struct ctx *ctx = dev->data;

    if (channel < kMinChannel || channel > kMaxChannel)
    {
        return channel < kMinChannel ? -ENOTSUP : -EINVAL;
    }

    ctx->channel = (uint8_t)channel;

    if (ctx->started)
    {
        nsi_otns_bottom_set_state(OT_RADIO_STATE_RECEIVE, ctx->channel);
    }

    return 0;
}

static int filter(const struct device            *dev,
                  bool                            set,
                  enum ieee802154_filter_type     type,
                  const struct ieee802154_filter *filter)
{
    struct ctx *ctx = dev->data;

    if (!set)
    {
        return -ENOTSUP;
    }

    switch (type)
    {
    case IEEE802154_FILTER_TYPE_IEEE_ADDR:
    {
        uint8_t ext_addr_be[OT_EXT_ADDRESS_SIZE];

        memcpy(ctx->ext_addr, filter->ieee_addr, OT_EXT_ADDRESS_SIZE);

        for (int i = 0; i < OT_EXT_ADDRESS_SIZE; i++)
        {
            ext_addr_be[i] = ctx->ext_addr[OT_EXT_ADDRESS_SIZE - 1 - i];
        }
        nsi_otns_bottom_send_ext_addr(ext_addr_be);
        return 0;
    }
    case IEEE802154_FILTER_TYPE_SHORT_ADDR:
        sys_put_le16(filter->short_addr, ctx->short_addr);
        return 0;
    case IEEE802154_FILTER_TYPE_PAN_ID:
        sys_put_le16(filter->pan_id, ctx->pan_id);
        return 0;
    default:
        return -ENOTSUP;
    }
}

static int set_txpower(const struct device *dev, int16_t dbm)
{
    struct ctx *ctx = dev->data;

    ctx->txpower = (int8_t)dbm;
    return 0;
}

static int tx(const struct device *dev, enum ieee802154_tx_mode mode, struct net_pkt *pkt, struct net_buf *frag)
{
    struct ctx *ctx = dev->data;
    uint8_t     psdu[OT_RADIO_FRAME_MAX_SIZE];
    uint16_t    len = frag->len;
    uint16_t    fcs;
    int         ret;
    int         rc = 0;

    ARG_UNUSED(pkt);

    if (mode != IEEE802154_TX_MODE_DIRECT && mode != IEEE802154_TX_MODE_CCA)
    {
        LOG_ERR("TX mode %d not supported", mode);
        return -ENOTSUP;
    }

    if (!ctx->started || !nsi_otns_bottom_is_connected())
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

    {
        struct frame_addr_info tx_info;

        if (parse_frame(psdu, len, &tx_info) == 0 && tx_info.dst_off >= 0)
        {
            uint8_t dst_ext_be[OT_EXT_ADDRESS_SIZE];

            if (tx_info.dst_mode == ADDR_MODE_EXT)
            {
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
    }

    ctx->tx_wants_ack = (len >= 1) && (frag->data[0] & (FCF_ACK_REQ_BIT & 0xff));
    ctx->tx_seq       = (len >= MIN_FRAME_SIZE) ? frag->data[2] : 0;
    ctx->ack_len      = 0;
    ctx->tx_result    = 0;

    k_sem_reset(&ctx->tx_wait);

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

        if (!ctx->ack_tx_pending)
        {
            nsi_otns_bottom_set_state(OT_RADIO_STATE_TRANSMIT, ctx->channel);
            irq_unlock(key);
            break;
        }

        irq_unlock(key);
        k_sem_take(&ctx->ack_tx_done, K_FOREVER);
    }

    ret = nsi_otns_bottom_tx(ctx->channel, ctx->txpower, psdu, len + FCS_SIZE);
    if (ret < 0)
    {
        rc = -EIO;
        goto out;
    }

    nsi_otns_bottom_set_state(OT_RADIO_STATE_RECEIVE, ctx->channel);

    if (ctx->tx_wants_ack)
    {
        uint32_t frame_us =
            (OT_RADIO_SHR_PHR_LENGTH_BYTES + len + FCS_SIZE) * (OT_RADIO_SYMBOLS_PER_OCTET * OT_RADIO_SYMBOL_TIME);

        if (k_sem_take(&ctx->tx_wait, K_USEC(frame_us + ACK_ALLOWANCE_US)) != 0)
        {
            rc = -ENOMSG;
            goto out;
        }
    }
    else
    {
        (void)k_sem_take(&ctx->tx_wait, K_FOREVER);
    }

    if (ctx->tx_result != 0)
    {
        rc = ctx->tx_result;
        goto out;
    }

    if (ctx->tx_wants_ack)
    {
        if (ctx->ack_len == 0)
        {
            rc = -ENOMSG;
            goto out;
        }

        struct net_pkt *ack_pkt;
        uint16_t ack_mac_len = IS_ENABLED(CONFIG_IEEE802154_L2_PKT_INCL_FCS) ? ctx->ack_len : ctx->ack_len - FCS_SIZE;

        ack_pkt = net_pkt_rx_alloc_with_buffer(ctx->iface, ack_mac_len, NET_AF_UNSPEC, 0, K_NO_WAIT);
        if (ack_pkt != NULL)
        {
            if (net_pkt_write(ack_pkt, ctx->ack_psdu, ack_mac_len) == 0)
            {
                net_pkt_set_ieee802154_lqi(ack_pkt, LQI_PERFECT);
                net_pkt_set_ieee802154_rssi_dbm(ack_pkt, 0);
                net_pkt_cursor_init(ack_pkt);
                if (ieee802154_handle_ack(ctx->iface, ack_pkt) != NET_OK)
                {
                    LOG_DBG("ACK not handled");
                }
                /* Frame Pending bit tells a polling child whether the parent has buffered data. */
                LOG_INF("ACK for seq %u: fcf=0x%04x frame_pending=%d", ctx->tx_seq, sys_get_le16(ctx->ack_psdu),
                        (ctx->ack_psdu[0] & (FCF_FRAME_PENDING_BIT & 0xff)) != 0);
            }
            net_pkt_unref(ack_pkt);
        }
    }

out:
    nsi_otns_bottom_set_state(OT_RADIO_STATE_RECEIVE, ctx->channel);
    return rc;
}

static int start(const struct device *dev)
{
    struct ctx *ctx = dev->data;

    if (ctx->started)
    {
        return -EALREADY;
    }

    ctx->started       = true;
    ctx->sleep_pending = false;
    nsi_otns_bottom_set_state(OT_RADIO_STATE_RECEIVE, ctx->channel);

    return 0;
}

static int stop(const struct device *dev)
{
    struct ctx *ctx = dev->data;

    if (!ctx->started)
    {
        return -EALREADY;
    }

    ctx->started = false;

    if (ctx->ack_tx_pending)
    {
        ctx->sleep_pending = true;
    }
    else
    {
        nsi_otns_bottom_set_state(OT_RADIO_STATE_SLEEP, ctx->channel);
    }

    return 0;
}

static int configure_enh_ack_ie(const struct ieee802154_config *config)
{
    const uint8_t *raw                   = (const uint8_t *)config->ack_ie.header_ie;
    uint16_t       short_addr            = config->ack_ie.short_addr;
    const uint8_t *ext_addr_be           = config->ack_ie.ext_addr;
    bool           has_short_filter      = short_addr != OT_RADIO_BROADCAST_SHORT_ADDR;
    bool           has_ext_filter        = ext_addr_be != NULL;
    bool           remove_all_for_filter = config->ack_ie.purge_ie || raw == NULL;
    uint8_t        element_id            = 0;
    uint8_t        content_len           = 0;
    int            slot                  = -1;
    int            free_slot             = -1;

    if (!remove_all_for_filter)
    {
        uint16_t hdr = sys_get_le16(raw);

        content_len = hdr & HEADER_IE_LEN_MASK;
        element_id  = (hdr >> HEADER_IE_ID_SHIFT) & 0xff;
    }

    for (int i = 0; i < MAX_ACK_IES; i++)
    {
        struct enh_ack_ie *ie = &data.ack_ies[i];
        bool               same_filter;

        if (!ie->valid)
        {
            if (free_slot < 0)
            {
                free_slot = i;
            }
            continue;
        }

        same_filter = ie->has_short_filter == has_short_filter && (!has_short_filter || ie->short_addr == short_addr) &&
                      ie->has_ext_filter == has_ext_filter &&
                      (!has_ext_filter || memcmp(ie->ext_addr_be, ext_addr_be, OT_EXT_ADDRESS_SIZE) == 0);

        if (!same_filter)
        {
            continue;
        }
        if (remove_all_for_filter)
        {
            ie->valid = false;
        }
        else if (ie->element_id == element_id)
        {
            slot = i;
        }
    }

    if (remove_all_for_filter || content_len == 0)
    {
        if (slot >= 0)
        {
            data.ack_ies[slot].valid = false;
        }
        return 0;
    }

    if (content_len > ACK_IE_MAX_CONTENT)
    {
        return -EINVAL;
    }
    if (slot < 0)
    {
        slot = free_slot;
    }
    if (slot < 0)
    {
        return -ENOMEM;
    }

    struct enh_ack_ie *ie = &data.ack_ies[slot];

    ie->valid       = true;
    ie->element_id  = element_id;
    ie->content_len = content_len;
    memcpy(ie->content, raw + 2, content_len);
    ie->has_short_filter = has_short_filter;
    ie->short_addr       = short_addr;
    ie->has_ext_filter   = has_ext_filter;
    if (has_ext_filter)
    {
        memcpy(ie->ext_addr_be, ext_addr_be, OT_EXT_ADDRESS_SIZE);
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
                data.fpb_ext_count = 0;
            }
            else
            {
                data.fpb_short_count = 0;
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

static void get_mac(struct ctx *ctx)
{
    int node_id = nsi_otns_bottom_get_node_id();

    if (node_id <= 0)
    {
        node_id = DEFAULT_NODE_ID;
    }

    ctx->mac_addr[0] = EUI64_BYTE_0;
    ctx->mac_addr[1] = EUI64_BYTE_1;
    ctx->mac_addr[2] = EUI64_BYTE_2;
    ctx->mac_addr[3] = EUI64_BYTE_3;
    ctx->mac_addr[4] = (node_id >> 24) & 0xff;
    ctx->mac_addr[5] = (node_id >> 16) & 0xff;
    ctx->mac_addr[6] = (node_id >> 8) & 0xff;
    ctx->mac_addr[7] = node_id & 0xff;
}

static void iface_init(struct net_if *iface)
{
    const struct device *dev = net_if_get_device(iface);
    struct ctx          *ctx = dev->data;

    get_mac(ctx);
    memcpy(ctx->ext_addr, ctx->mac_addr, OT_EXT_ADDRESS_SIZE);

    net_if_set_link_addr(iface, ctx->mac_addr, OT_EXT_ADDRESS_SIZE, NET_LINK_IEEE802154);

    ctx->iface = iface;
    radio_dev  = dev;

    ieee802154_init(iface);
}

static int init(const struct device *dev)
{
    struct ctx *ctx = dev->data;

    k_sem_init(&ctx->tx_wait, 0, 1);
    k_sem_init(&ctx->cca_wait, 0, 1);
    k_sem_init(&ctx->ack_tx_done, 0, 1);

    ctx->channel = kMinChannel;
    ctx->txpower = 0;
    ctx->started = false;

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
