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
 */

#include <errno.h>
#include <string.h>

#include <zephyr/irq.h>
#include <zephyr/net/ieee802154_radio.h>
#include <zephyr/sys/byteorder.h>

#include <openthread/link.h>
#include <openthread/platform/time.h>

#include <psa/crypto.h>

#include "ieee802154_data.h"
#include "ieee802154_frame.h"
#include "ieee802154_priv.h"

#define LOG_MODULE_NAME ieee802154_otns
#include <zephyr/logging/log.h>
LOG_MODULE_DECLARE(LOG_MODULE_NAME);

#define SEC_CTRL_LEVEL_MASK 0x07
#define SEC_CTRL_KEY_ID_MODE_SHIFT 3
#define SEC_CTRL_KEY_ID_MODE_MASK 0x03
#define SEC_LEVEL_ENC_BIT 0x04
#define SEC_CTRL_FIELD_SIZE 1
#define FRAME_COUNTER_FIELD_SIZE 4
#define CMD_FRAME_ID_SIZE 1
#define CCM_NONCE_SIZE (OT_EXT_ADDRESS_SIZE + FRAME_COUNTER_FIELD_SIZE + SEC_CTRL_FIELD_SIZE)

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

static bool dst_pan_id_present_2015(uint8_t dst_mode, uint8_t src_mode, bool pc)
{
    if (dst_mode == ADDR_MODE_NONE)
    {
        return src_mode == ADDR_MODE_NONE && pc;
    }
    if (src_mode == ADDR_MODE_NONE || (src_mode == ADDR_MODE_EXT && dst_mode == ADDR_MODE_EXT))
    {
        return !pc;
    }
    return true;
}

static bool src_pan_id_present_2015(uint8_t dst_mode, uint8_t src_mode, bool pc)
{
    if (dst_mode == ADDR_MODE_EXT && src_mode == ADDR_MODE_EXT)
    {
        return false;
    }
    return src_mode != ADDR_MODE_NONE && !pc;
}

int parse_frame(const uint8_t *psdu, uint16_t len, struct frame_addr_info *info)
{
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

    bool pc  = (info->fcf & FCF_PAN_COMPR_BIT) != 0;
    int  off = FCF_SIZE;

    if (!(info->version == IEEE802154_VERSION_2015 && (info->fcf & FCF_SEQ_SUPPR_BIT)))
    {
        if (off >= len)
        {
            return -1;
        }
        info->seq = psdu[off];
        off += SEQ_NUM_SIZE;
    }

    bool dst_pan_present;
    bool src_pan_present;
    if (info->version == IEEE802154_VERSION_2015)
    {
        dst_pan_present = dst_pan_id_present_2015(info->dst_mode, info->src_mode, pc);
        src_pan_present = src_pan_id_present_2015(info->dst_mode, info->src_mode, pc);
    }
    else
    {
        dst_pan_present = info->dst_mode != ADDR_MODE_NONE;
        src_pan_present = info->src_mode != ADDR_MODE_NONE && !pc;
    }

    if (info->dst_mode != ADDR_MODE_NONE)
    {
        if (dst_pan_present)
        {
            if (off + PAN_ID_SIZE > len)
            {
                return -1;
            }
            info->dst_pan         = sys_get_le16(&psdu[off]);
            info->dst_pan_present = true;
            off += PAN_ID_SIZE;
        }

        info->dst_off = off;
        off += (info->dst_mode == ADDR_MODE_EXT) ? OT_EXT_ADDRESS_SIZE : SHORT_ADDR_SIZE;
        if (off > len)
        {
            return -1;
        }
    }

    if (info->src_mode != ADDR_MODE_NONE)
    {
        if (src_pan_present)
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

static void fill_csl_ie_content(uint8_t *content) { sys_put_le16(csl_phase(), &content[0]); }

static void fill_link_metrics_ie_content(uint8_t *content, uint8_t content_len, int8_t rssi)
{
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
    uint16_t off = FCF_SIZE + SEQ_NUM_SIZE;
    uint16_t dst_pan;

    if (info->src_mode == ADDR_MODE_NONE || info->src_off == FRAME_OFF_NONE)
    {
        return -1;
    }
    if (info->src_pan_present)
    {
        dst_pan = info->src_pan;
    }
    else if (info->dst_pan_present)
    {
        dst_pan = info->dst_pan;
    }
    else
    {
        dst_pan = sys_get_le16(data.pan_id);
    }

    uint8_t  addr_len = (info->src_mode == ADDR_MODE_EXT) ? OT_EXT_ADDRESS_SIZE : SHORT_ADDR_SIZE;
    uint16_t fcf      = FCF_FRAME_TYPE_ACK | ((uint16_t)IEEE802154_VERSION_2015 << FCF_VERSION_SHIFT) |
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

    uint16_t ie_len =
        build_enh_ack_ies(rx_psdu, info, rssi, &ack[off], (uint16_t)(OT_RADIO_FRAME_MAX_SIZE - off - FCS_SIZE));

    if (ie_len > 0)
    {
        fcf |= FCF_IE_PRESENT_BIT;
    }
    off += ie_len;

    sys_put_le16(fcf, &ack[0]);

    uint16_t fcs = crc16(ack, off);

    ack[off]     = fcs & 0xff;
    ack[off + 1] = fcs >> 8;
    *ack_len_out = off + FCS_SIZE;
    return 0;
}

void schedule_ack(const uint8_t *rx_psdu, uint16_t rx_len, int8_t rssi, const struct frame_addr_info *info)
{
    uint8_t  ack[OT_RADIO_FRAME_MAX_SIZE];
    uint16_t ack_len;

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

        uint16_t fcs = crc16(ack, MIN_FRAME_SIZE);

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
    int            ret                   = 0;

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

    unsigned int key = irq_lock();

    for (int i = 0; i < MAX_ACK_IES; i++)
    {
        struct enh_ack_ie *ie = &data.ack_ies[i];

        if (!ie->valid)
        {
            if (free_slot < 0)
            {
                free_slot = i;
            }
            continue;
        }

        bool same_filter = ie->has_short_filter == has_short_filter &&
                           (!has_short_filter || ie->short_addr == short_addr) &&
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
        ie->valid = true;
    }

out:
    irq_unlock(key);
    return ret;
}

/* ------------------------------------------------------------------------- */
/* Software transmit security (IEEE802154_HW_TX_SEC)                        */
/* ------------------------------------------------------------------------- */

void set_mac_keys(const struct ieee802154_key *keys)
{
    unsigned int lock = irq_lock();

    for (int i = 0; i < MAX_MAC_KEYS; i++)
    {
        data.mac_keys[i].valid = false;
    }

    for (int i = 0; keys != NULL && i < MAX_MAC_KEYS && keys[i].key_value != NULL; i++)
    {
        data.mac_keys[i].key_id_mode = keys[i].key_id_mode;
        data.mac_keys[i].key_id      = (keys[i].key_id_mode != 0) ? *keys[i].key_id : 0;
        memcpy(data.mac_keys[i].key, keys[i].key_value, MAC_KEY_SIZE);
        data.mac_keys[i].valid = true;
    }

    irq_unlock(lock);
}

void set_frame_counter(uint32_t counter, bool only_if_larger)
{
    unsigned int lock = irq_lock();

    if (!only_if_larger || counter > data.mac_frame_counter)
    {
        data.mac_frame_counter = counter;
    }

    irq_unlock(lock);
}

static const uint8_t *find_mac_key(uint8_t key_id_mode, uint8_t key_id)
{
    for (int i = 0; i < MAX_MAC_KEYS; i++)
    {
        if (data.mac_keys[i].valid && data.mac_keys[i].key_id_mode == key_id_mode && data.mac_keys[i].key_id == key_id)
        {
            return data.mac_keys[i].key;
        }
    }
    return NULL;
}

static int aux_sec_header_len(uint8_t key_id_mode)
{
    static const uint8_t key_id_field_len[4] = {0, 1, 5, 9};

    return SEC_CTRL_FIELD_SIZE + FRAME_COUNTER_FIELD_SIZE + key_id_field_len[key_id_mode & SEC_CTRL_KEY_ID_MODE_MASK];
}

static int find_payload_index(const uint8_t *psdu, uint16_t len, int ie_start, int footer_len, bool ie_present)
{
    int index = ie_start;

    if (!ie_present)
    {
        return index;
    }

    for (;;)
    {
        if (index + HEADER_IE_HDR_SIZE + footer_len > len)
        {
            return -1;
        }

        uint16_t hdr         = sys_get_le16(&psdu[index]);
        uint8_t  content_len = hdr & HEADER_IE_LEN_MASK;
        uint8_t  element_id  = (hdr >> HEADER_IE_ID_SHIFT) & 0xff;

        index += HEADER_IE_HDR_SIZE;

        if (index + content_len + footer_len > len)
        {
            return -1;
        }
        index += content_len;

        if (element_id == HEADER_IE_ID_TERMINATION_2 || index + footer_len >= len)
        {
            break;
        }
    }

    return index;
}

static int mic_len_for_level(uint8_t security_level)
{
    static const uint8_t mic_len[4] = {0, 4, 8, 16};

    return mic_len[security_level & 0x03];
}

static void patch_tx_csl_ie(uint8_t *psdu, uint16_t len, int ie_start, int footer_len, bool ie_present)
{
    int off = ie_start;

    if (data.csl_period == 0 || !ie_present)
    {
        return;
    }

    while (off + HEADER_IE_HDR_SIZE + footer_len <= len)
    {
        uint16_t hdr         = sys_get_le16(&psdu[off]);
        uint8_t  content_len = hdr & HEADER_IE_LEN_MASK;
        uint8_t  element_id  = (hdr >> HEADER_IE_ID_SHIFT) & 0xff;

        if (off + HEADER_IE_HDR_SIZE + content_len + footer_len > len)
        {
            return;
        }
        if (element_id == HEADER_IE_ID_CSL && content_len >= ACK_IE_CSL_CONTENT_SIZE)
        {
            uint8_t *content = &psdu[off + HEADER_IE_HDR_SIZE];

            sys_put_le16(csl_phase(), &content[0]);
            sys_put_le16((uint16_t)data.csl_period, &content[2]);
            return;
        }
        if (element_id == HEADER_IE_ID_TERMINATION_2)
        {
            return;
        }
        off += HEADER_IE_HDR_SIZE + content_len;
    }
}

static void build_ccm_nonce(uint32_t frame_counter, uint8_t security_level, uint8_t nonce[CCM_NONCE_SIZE])
{
    for (int i = 0; i < OT_EXT_ADDRESS_SIZE; i++)
    {
        nonce[i] = data.ext_addr[OT_EXT_ADDRESS_SIZE - 1 - i];
    }
    sys_put_be32(frame_counter, &nonce[OT_EXT_ADDRESS_SIZE]);
    nonce[OT_EXT_ADDRESS_SIZE + FRAME_COUNTER_FIELD_SIZE] = security_level;
}

static int frame_ie_start(const struct frame_addr_info *info)
{
    if (info->src_mode != ADDR_MODE_NONE)
    {
        return info->src_off + ((info->src_mode == ADDR_MODE_EXT) ? OT_EXT_ADDRESS_SIZE : SHORT_ADDR_SIZE);
    }
    if (info->dst_mode != ADDR_MODE_NONE)
    {
        return info->dst_off + ((info->dst_mode == ADDR_MODE_EXT) ? OT_EXT_ADDRESS_SIZE : SHORT_ADDR_SIZE);
    }
    return FCF_SIZE + SEQ_NUM_SIZE;
}

static int frame_header_len(const uint8_t *psdu,
                            uint16_t       len,
                            int            ie_start,
                            int            footer_len,
                            bool           ie_present,
                            uint16_t       fcf,
                            int            version)
{
    int header_len = find_payload_index(psdu, len, ie_start, footer_len, ie_present);

    if (header_len >= 0 && (fcf & FCF_FRAME_TYPE_MASK) == FCF_FRAME_TYPE_CMD && version != IEEE802154_VERSION_2015)
    {
        header_len += CMD_FRAME_ID_SIZE;
    }

    return header_len;
}

static int aead_encrypt_frame(uint8_t       *psdu,
                              int            header_len,
                              int            payload_len,
                              uint8_t        security_level,
                              int            mic_len,
                              uint32_t       frame_counter,
                              const uint8_t *key)
{
    bool    encrypt = (security_level & SEC_LEVEL_ENC_BIT) != 0;
    size_t  aad_len = encrypt ? (size_t)header_len : (size_t)(header_len + payload_len);
    size_t  pt_len  = encrypt ? (size_t)payload_len : 0;
    uint8_t nonce[CCM_NONCE_SIZE];
    uint8_t out[OT_RADIO_FRAME_MAX_SIZE];
    size_t  out_len = 0;

    build_ccm_nonce(frame_counter, security_level, nonce);

    psa_key_attributes_t attrs = PSA_KEY_ATTRIBUTES_INIT;
    psa_algorithm_t      alg   = PSA_ALG_AEAD_WITH_SHORTENED_TAG(PSA_ALG_CCM, mic_len);
    psa_key_id_t         key_id_psa;

    psa_set_key_type(&attrs, PSA_KEY_TYPE_AES);
    psa_set_key_bits(&attrs, MAC_KEY_SIZE * 8);
    psa_set_key_usage_flags(&attrs, PSA_KEY_USAGE_ENCRYPT);
    psa_set_key_algorithm(&attrs, alg);

    psa_status_t status = psa_import_key(&attrs, key, MAC_KEY_SIZE, &key_id_psa);

    if (status != PSA_SUCCESS)
    {
        LOG_ERR("encrypt_tx_frame: psa_import_key failed: %d", status);
        return -EIO;
    }

    status = psa_aead_encrypt(key_id_psa, alg, nonce, sizeof(nonce), psdu, aad_len, &psdu[aad_len], pt_len, out,
                              sizeof(out), &out_len);
    psa_destroy_key(key_id_psa);

    if (status != PSA_SUCCESS || out_len != pt_len + (size_t)mic_len)
    {
        LOG_ERR("encrypt_tx_frame: psa_aead_encrypt failed: status=%d out_len=%zu", status, out_len);
        return -EIO;
    }

    memcpy(&psdu[aad_len], out, out_len);
    return 0;
}

int encrypt_tx_frame(uint8_t *psdu, uint16_t len)
{
    struct frame_addr_info info;
    int                    footer_len = 0;

    if (parse_frame(psdu, len, &info) < 0)
    {
        LOG_ERR("encrypt_tx_frame: parse_frame failed len=%u", len);
        return -EINVAL;
    }

    uint16_t fcf        = info.fcf;
    bool     secured    = (fcf & FCF_SECURITY_ENABLED_BIT) != 0;
    bool     ie_present = (fcf & FCF_IE_PRESENT_BIT) != 0;
    int      ie_start   = frame_ie_start(&info);

    if (!secured)
    {
        patch_tx_csl_ie(psdu, len, ie_start, footer_len, ie_present);
        return 0;
    }

    if (ie_start >= len)
    {
        LOG_ERR("encrypt_tx_frame: ie_start>=len ie_start=%d len=%u", ie_start, len);
        return -EINVAL;
    }

    int     sec_ctrl_off   = ie_start;
    uint8_t security_level = psdu[sec_ctrl_off] & SEC_CTRL_LEVEL_MASK;
    uint8_t key_id_mode    = (psdu[sec_ctrl_off] >> SEC_CTRL_KEY_ID_MODE_SHIFT) & SEC_CTRL_KEY_ID_MODE_MASK;

    if (key_id_mode != 1)
    {
        return 0;
    }

    int mic_len = mic_len_for_level(security_level);

    footer_len = mic_len;

    ie_start += aux_sec_header_len(key_id_mode);
    if (ie_start > len)
    {
        LOG_ERR("encrypt_tx_frame: aux header overruns ie_start=%d len=%u", ie_start, len);
        return -EINVAL;
    }

    uint8_t        key_id = (key_id_mode != 0) ? psdu[ie_start - 1] : 0;
    const uint8_t *key    = find_mac_key(key_id_mode, key_id);

    if (key == NULL)
    {
        LOG_ERR("encrypt_tx_frame: no MAC key for key_id_mode=%u key_id=%u", key_id_mode, key_id);
        return -ENOKEY;
    }

    patch_tx_csl_ie(psdu, len, ie_start, footer_len, ie_present);

    int header_len = frame_header_len(psdu, len, ie_start, footer_len, ie_present, fcf, info.version);

    if (header_len < 0 || header_len + footer_len > len)
    {
        LOG_ERR("encrypt_tx_frame: bad header_len=%d footer_len=%d len=%u ie_present=%d", header_len, footer_len, len,
                ie_present);
        return -EINVAL;
    }

    int      payload_len   = len - header_len - footer_len;
    uint32_t frame_counter = data.mac_frame_counter++;

    sys_put_le32(frame_counter, &psdu[sec_ctrl_off + SEC_CTRL_FIELD_SIZE]);

    return aead_encrypt_frame(psdu, header_len, payload_len, security_level, mic_len, frame_counter, key);
}
