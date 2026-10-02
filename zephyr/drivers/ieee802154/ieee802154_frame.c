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
 * See ieee802154_frame.h.
 */

#include <errno.h>
#include <string.h>

#include <zephyr/irq.h>
#include <zephyr/net/ieee802154_radio.h>
#include <zephyr/sys/byteorder.h>

#include <openthread/link.h>
#include <openthread/platform/time.h>

#include "ieee802154_data.h"
#include "ieee802154_frame.h"
#include "ieee802154_priv.h"

/* ------------------------------------------------------------------------- */
/* Framing helpers (CRC)                                                     */
/* ------------------------------------------------------------------------- */

uint16_t crc16(const uint8_t *data, size_t len)
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

/* ------------------------------------------------------------------------- */
/* 802.15.4 frame parsing                                                    */
/* ------------------------------------------------------------------------- */

int parse_frame(const uint8_t *psdu, uint16_t len, struct frame_addr_info *info)
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
    info->dst_off  = FRAME_OFF_NONE;
    info->src_off  = FRAME_OFF_NONE;

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

bool frame_is_for_me(const uint8_t *psdu, uint16_t len, struct frame_addr_info *info)
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

/* ------------------------------------------------------------------------- */
/* Auto-ACK frame-pending-bit (FPB) address list                             */
/* ------------------------------------------------------------------------- */

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

/*
 * The fpb_* mutators below run in thread context (via configure()) while
 * frame_pending_for() reads the same arrays from isr()/handle_rx(). irq_lock()
 * excludes the ISR for the duration of the mutation, same pattern as tx().
 */
int fpb_short_add(uint16_t addr)
{
    unsigned int key = irq_lock();
    int          ret = 0;

    if (fpb_short_contains(addr))
    {
        goto out;
    }
    if (data.fpb_short_count >= MAX_FPB_ENTRIES)
    {
        ret = -ENOMEM;
        goto out;
    }
    data.fpb_short[data.fpb_short_count++] = addr;

out:
    irq_unlock(key);
    return ret;
}

int fpb_short_remove(uint16_t addr)
{
    unsigned int key = irq_lock();
    int          ret = -ENOENT;

    for (int i = 0; i < data.fpb_short_count; i++)
    {
        if (data.fpb_short[i] == addr)
        {
            data.fpb_short[i] = data.fpb_short[--data.fpb_short_count];
            ret               = 0;
            break;
        }
    }

    irq_unlock(key);
    return ret;
}

void fpb_short_clear(void)
{
    unsigned int key = irq_lock();

    data.fpb_short_count = 0;
    irq_unlock(key);
}

int fpb_ext_add(const uint8_t *addr)
{
    unsigned int key = irq_lock();
    int          ret = 0;

    if (fpb_ext_contains(addr))
    {
        goto out;
    }
    if (data.fpb_ext_count >= MAX_FPB_ENTRIES)
    {
        ret = -ENOMEM;
        goto out;
    }
    memcpy(data.fpb_ext[data.fpb_ext_count++], addr, OT_EXT_ADDRESS_SIZE);

out:
    irq_unlock(key);
    return ret;
}

int fpb_ext_remove(const uint8_t *addr)
{
    unsigned int key = irq_lock();
    int          ret = -ENOENT;

    for (int i = 0; i < data.fpb_ext_count; i++)
    {
        if (memcmp(data.fpb_ext[i], addr, OT_EXT_ADDRESS_SIZE) == 0)
        {
            memcpy(data.fpb_ext[i], data.fpb_ext[--data.fpb_ext_count], OT_EXT_ADDRESS_SIZE);
            ret = 0;
            break;
        }
    }

    irq_unlock(key);
    return ret;
}

void fpb_ext_clear(void)
{
    unsigned int key = irq_lock();

    data.fpb_ext_count = 0;
    irq_unlock(key);
}

bool frame_pending_for(const uint8_t *psdu, uint16_t len, const struct frame_addr_info *info)
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

/* ------------------------------------------------------------------------- */
/* Enhanced-ACK building (CSL phase, Thread Link-Metrics IEs)                */
/* ------------------------------------------------------------------------- */

/* Caller must guard against data.csl_period == 0 (CSL disabled) to avoid division by zero below. */
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

static void fill_csl_ie_content(uint8_t *content)
{
    sys_put_le16(csl_phase(), &content[0]); /* content[2..3] = period, unchanged */
}

static void fill_link_metrics_ie_content(uint8_t *content, uint8_t content_len, int8_t rssi)
{
    /* Vendor IE: content[0..2]=OUI, [3]=subtype, [4..]=LM_TOKEN_* placeholders. */
    for (int j = LM_VENDOR_IE_TOKEN_OFFSET; j < content_len; j++)
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
            fill_csl_ie_content(content);
        }
        else
        {
            fill_link_metrics_ie_content(content, ie->content_len, rssi);
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

    if (info->src_mode == ADDR_MODE_NONE || info->src_off == FRAME_OFF_NONE)
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

void schedule_ack(const uint8_t *rx_psdu, uint16_t rx_len, int8_t rssi, const struct frame_addr_info *info)
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

int configure_enh_ack_ie(const struct ieee802154_config *config)
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
    unsigned int   key;
    int            ret = 0;

    if (!remove_all_for_filter)
    {
        uint16_t hdr = sys_get_le16(raw);

        content_len = hdr & HEADER_IE_LEN_MASK;
        element_id  = (hdr >> HEADER_IE_ID_SHIFT) & 0xff;

        if (content_len > ACK_IE_MAX_CONTENT)
        {
            return -EINVAL;
        }
    }

    /* Locked: data.ack_ies[] is also read by schedule_ack()/build_enh_ack_ies() from the ISR. */
    key = irq_lock();

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
        goto out;
    }

    if (slot < 0)
    {
        slot = free_slot;
    }
    if (slot < 0)
    {
        ret = -ENOMEM;
        goto out;
    }

    {
        struct enh_ack_ie *ie = &data.ack_ies[slot];

        ie->element_id  = element_id;
        ie->content_len = content_len;
        memcpy(ie->content, raw + HEADER_IE_HDR_SIZE, content_len);
        ie->has_short_filter = has_short_filter;
        ie->short_addr       = short_addr;
        ie->has_ext_filter   = has_ext_filter;
        if (has_ext_filter)
        {
            memcpy(ie->ext_addr_be, ext_addr_be, OT_EXT_ADDRESS_SIZE);
        }
        ie->valid = true; /* set last: the ISR must never observe a partially-filled entry */
    }

out:
    irq_unlock(key);
    return ret;
}
