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
 * Shared definitions for the OTNS (OpenThread Network Simulator) IEEE 802.15.4
 * driver. This header is included by BOTH:
 *
 *   - the embedded (Zephyr CPU) side  : ieee802154_otns.c
 *   - the runner  (host/native) side  : ieee802154_otns_bottom.c
 *
 * It must therefore only use standard C integer types and headers that are
 * valid in both compilation environments (plain C, or portable OpenThread
 * public API headers - no Zephyr- or host-specific ones).
 *
 * The two sides are linked together into a single native_simulator executable
 * and communicate through the plain C functions declared below. Data flowing
 * from the runner side towards the embedded side is signalled with a dedicated
 * interrupt (IEEE802154_OTNS_IRQ) and pulled by the embedded ISR.
 */

#ifndef IEEE802154_OTNS_PRIV_H__
#define IEEE802154_OTNS_PRIV_H__

#include <stdbool.h>
#include <stdint.h>

#include "ot-rfsim/src/event-sim.h"

#ifdef __cplusplus
extern "C" {
#endif

#define IEEE802154_OTNS_IRQ 4

struct otns_radio_event
{
    uint8_t                   type;
    struct RadioCommEventData data;
    uint16_t                  psdu_len;
    uint8_t                   psdu[OT_RADIO_FRAME_MAX_SIZE];
};

int nsi_otns_bottom_get_node_id(void);

bool nsi_otns_bottom_is_connected(void);

bool nsi_otns_bottom_is_configured(void);

int nsi_otns_bottom_tx(uint8_t channel, int8_t power, const uint8_t *psdu, uint16_t len);

int nsi_otns_bottom_tx_after(uint8_t channel, int8_t power, const uint8_t *psdu, uint16_t len, uint32_t delay_us);

int nsi_otns_bottom_cca(uint8_t channel);

void nsi_otns_bottom_set_state(uint8_t state, uint8_t channel);

void nsi_otns_bottom_send_uart(const uint8_t *buf, uint16_t len);

void nsi_otns_bottom_send_ext_addr(const uint8_t *ext_addr_be);

void nsi_otns_bottom_send_status(const char *status, uint16_t len);

bool nsi_otns_bottom_get_event(struct otns_radio_event *ev);

int32_t nsi_otns_bottom_get_drift_offset_ms(void);

uint8_t nsi_otns_bottom_get_csl_accuracy(void);

#ifdef __cplusplus
}
#endif

#endif /* IEEE802154_OTNS_PRIV_H__ */
