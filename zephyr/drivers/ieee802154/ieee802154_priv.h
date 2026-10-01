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

/*
 * Interrupt line used by the runner side to notify the embedded side that a
 * radio event is ready to be consumed. native_sim reserves IRQs 0..3
 * (timer/offload/counter/nsos), so line 4 is free.
 */
#define IEEE802154_OTNS_IRQ 4

/*
 * A radio event exchanged across the embedded/runner boundary. The PSDU here is
 * the raw MAC frame *including* the 2-byte FCS, without any PHY length prefix
 * and without the OTNS channel byte (both handled inside the runner side).
 */
struct otns_radio_event
{
    uint8_t                   type;     /* OT_SIM_EVENT_* */
    struct RadioCommEventData data;     /* channel/power/error (mDuration unused here) */
    uint16_t                  psdu_len; /* length of psdu[] (incl. FCS); 0 if none */
    uint8_t                   psdu[OT_RADIO_FRAME_MAX_SIZE];
};

/*
 * ---------------------------------------------------------------------------
 * Runner-side functions, called from the embedded side.
 * ---------------------------------------------------------------------------
 */

/* Returns the OTNS node id assigned to this process (>=1), or 0 if unknown. */
int nsi_otns_bottom_get_node_id(void);

/* Returns true once the driver is connected to OTNS, false otherwise. */
bool nsi_otns_bottom_is_connected(void);

/* Returns true if an OTNS socket path was configured on the command line. */
bool nsi_otns_bottom_is_configured(void);

/*
 * Queue a frame transmission towards the simulator (RADIO_COMM_START). The
 * frame is the MAC PSDU including FCS. Returns 0 on success, negative on error.
 */
int nsi_otns_bottom_tx(uint8_t channel, int8_t power, const uint8_t *psdu, uint16_t len);

/*
 * Like nsi_otns_bottom_tx(), but defer the transmission by @p delay_us of
 * virtual time, timed on the runner's microsecond-resolution HW clock. Used for
 * the auto-ACK, which must be sent exactly one AIFS turnaround (192 us) after
 * the acknowledged frame ends - an interval too short for the Zephyr kernel
 * tick to represent. Only one deferred frame may be pending at a time; a new
 * call replaces any still-pending one. Returns 0 on success, negative on error.
 */
int nsi_otns_bottom_tx_after(uint8_t channel, int8_t power, const uint8_t *psdu, uint16_t len, uint32_t delay_us);

/*
 * Request a channel sample / CCA from the simulator (RADIO_CHAN_SAMPLE).
 * The energy result is delivered back asynchronously as a radio event.
 */
int nsi_otns_bottom_cca(uint8_t channel);

/* Report the current radio energy state to the simulator (RADIO_STATE). */
void nsi_otns_bottom_set_state(uint8_t state, uint8_t channel);

/*
 * Send CLI/UART reply bytes back to the simulator as an OT_SIM_EVENT_UART_WRITE
 * event. Used by the OTNS CLI bridge.
 */
void nsi_otns_bottom_send_uart(const uint8_t *buf, uint16_t len);

/*
 * Report this node's IEEE 802.15.4 extended (MAC) address to the simulator as
 * an OT_SIM_EVENT_EXT_ADDR event. OTNS uses this to route unicast frames that
 * are addressed by extended address to the correct node; without it, such
 * frames (e.g. the MLE Parent Response) are never delivered and the link fails
 * with repeated NoAck errors. @p ext_addr_be must hold the 8 address bytes in
 * big-endian order (OTNS keys its node lookup on the big-endian value), i.e.
 * the reverse of the little-endian order they appear in on air.
 */
void nsi_otns_bottom_send_ext_addr(const uint8_t *ext_addr_be);

/*
 * Forward an OpenThread OTNS status string to the simulator as an
 * OT_SIM_EVENT_OTNS_STATUS_PUSH event. OTNS uses these to visualise node state
 * in its web UI (e.g. "role=2;rloc16=..."). Without them a node stays greyed
 * out ("disabled") even though it has actually attached. @p status is a raw,
 * non-NUL-terminated status string of @p len bytes.
 */
void nsi_otns_bottom_send_status(const char *status, uint16_t len);

/*
 * Fetch the radio event that the runner side has just delivered (pulled by the
 * embedded ISR). Returns true and fills *ev if an event was pending, false otherwise.
 */
bool nsi_otns_bottom_get_event(struct otns_radio_event *ev);

/*
 * Returns the millisecond offset currently implied by this node's configured
 * clock drift ('rfsim <id> clkdrift'), as last computed by the runner side's
 * drift accumulator. The embedded side is responsible for actually applying it
 * (via alarm_milli_set_time_offset_ms(), part of Zephyr's OpenThread platform
 * layer) since that symbol only exists in the embedded image; the runner side
 * cannot call it directly (it is link-time unreachable from the runner/native
 * simulator executable link, which discards it unless the embedded side itself
 * references it).
 */
int32_t nsi_otns_bottom_get_drift_offset_ms(void);

#ifdef __cplusplus
}
#endif

#endif /* IEEE802154_OTNS_PRIV_H__ */
