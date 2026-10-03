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
 * 802.15.4 frame parsing, the auto-ACK frame-pending-bit (FPB) address list,
 * and Enhanced-ACK (CSL / Thread Link-Metrics) building. Implemented in
 * ieee802154_frame.c; shared by ieee802154.c and ieee802154_isr.c.
 */

#ifndef IEEE802154_OTNS_FRAME_H__
#define IEEE802154_OTNS_FRAME_H__

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "radio.h"

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
#define FCF_BYTE0_FRAME_PENDING_BIT (FCF_FRAME_PENDING_BIT & 0xff) /* FCF low byte, for byte-wise PSDU access */
#define FCF_BYTE0_ACK_REQ_BIT (FCF_ACK_REQ_BIT & 0xff)
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
#define FRAME_OFF_NONE (-1) /* sentinel for frame_addr_info.{dst,src}_off: address not present */

/* Header IE (Information Element), IEEE 802.15.4-2015 section 7.4.2.1. */
#define HEADER_IE_HDR_SIZE 2 /* 2-byte length+element-id header preceding the IE content */
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

#define MAX_FPB_ENTRIES 32

#define AIFS_TURNAROUND_US ((uint32_t)OT_RADIO_AIFS_TIME_US)

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

uint16_t crc16(const uint8_t *data, size_t len);

int  parse_frame(const uint8_t *psdu, uint16_t len, struct frame_addr_info *info);
bool frame_is_for_me(const uint8_t *psdu, uint16_t len, struct frame_addr_info *info);

bool frame_pending_for(const uint8_t *psdu, uint16_t len, const struct frame_addr_info *info);

int  fpb_short_add(uint16_t addr);
int  fpb_short_remove(uint16_t addr);
void fpb_short_clear(void);
int  fpb_ext_add(const uint8_t *addr);
int  fpb_ext_remove(const uint8_t *addr);
void fpb_ext_clear(void);

void schedule_ack(const uint8_t *rx_psdu, uint16_t rx_len, int8_t rssi, const struct frame_addr_info *info);

struct ieee802154_config;
int configure_enh_ack_ie(const struct ieee802154_config *config);

#endif /* IEEE802154_OTNS_FRAME_H__ */
