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
 * Shared driver state (struct otns_radio_data) for the embedded-side IEEE
 * 802.15.4 OTNS driver, split across ieee802154.c, ieee802154_frame.c and
 * ieee802154_isr.c. `data`/`radio_dev` are defined once in ieee802154.c.
 */

#ifndef IEEE802154_OTNS_DATA_H__
#define IEEE802154_OTNS_DATA_H__

#include <stdbool.h>
#include <stdint.h>

#include <zephyr/device.h>
#include <zephyr/kernel.h>
#include <zephyr/net/ieee802154_radio.h>
#include <zephyr/net/net_if.h>

#include "ieee802154_frame.h"
#include "radio.h"

#define LQI_PERFECT 255

struct otns_radio_data
{
    struct net_if *iface;
    uint8_t        mac_addr[8];

    /* Synchronous TX completion / CCA / deferred-ACK signaling. */
    struct k_sem tx_wait;
    struct k_sem cca_wait;
    struct k_sem ack_tx_done; /* given when a pending deferred auto-ACK transmission completes */

    volatile int  tx_result;
    volatile bool cca_channel_free;

    /* State of the frame currently being transmitted, and its received ACK (if any). */
    bool              tx_wants_ack;
    uint8_t           tx_seq;
    uint8_t           ack_psdu[OT_RADIO_FRAME_MAX_SIZE];
    volatile uint16_t ack_len;

    /* Address filters (ieee802154_radio_api.filter()). */
    uint8_t pan_id[2];
    uint8_t short_addr[2];
    uint8_t ext_addr[8];

    /* Auto-ACK frame-pending-bit (FPB) address list. */
    bool     auto_ack_fpb_enabled;
    uint16_t fpb_short[MAX_FPB_ENTRIES];
    uint8_t  fpb_short_count;
    uint8_t  fpb_ext[MAX_FPB_ENTRIES][OT_EXT_ADDRESS_SIZE];
    uint8_t  fpb_ext_count;

    /* Energy-detection scan (ed_scan()). */
    bool                  ed_scan_pending;
    energy_scan_done_cb_t ed_done_cb;

    /* Deferred auto-ACK transmission vs. a stop() requested while it's in flight. */
    bool ack_tx_pending;
    bool sleep_pending;

    /* Coordinated Sampled Listening (CSL) phase computation for Enhanced ACKs. */
    uint32_t csl_period;
    int64_t  csl_expected_rx_time_ns;

    struct enh_ack_ie ack_ies[MAX_ACK_IES];

    uint8_t channel;
    int8_t  txpower;
    bool    started;
};

/*
 * This driver only ever instantiates one device (NET_DEVICE_DT_INST_DEFINE(0, ...) in
 * ieee802154.c), so `data` is accessed directly everywhere (ISR included) rather than
 * via dev->data indirection.
 */
extern struct otns_radio_data data;

extern const struct device *radio_dev;

#endif /* IEEE802154_OTNS_DATA_H__ */
