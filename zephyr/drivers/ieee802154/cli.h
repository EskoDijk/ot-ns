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
 * Boundary definitions for the OTNS OpenThread CLI bridge.
 *
 * OTNS configures each simulated node through its OpenThread CLI. For a standard
 * (non-RCP) node it delivers commands over the simulation socket as
 * OT_SIM_EVENT_UART_WRITE events (the "virtual-time UART") and reads the replies
 * back the same way. This bridge feeds those command bytes to the raw
 * OpenThread CLI and sends its output back as UART_WRITE events.
 *
 * This header is shared by the embedded (Zephyr CPU) side and the runner
 * (host) side; it must use only standard C types.
 */

#ifndef OTNS_CLI_H__
#define OTNS_CLI_H__

#include <stdbool.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

/*
 * Interrupt line used by the runner side to notify the embedded side that CLI
 * input bytes (from stdin) are available. IRQs 0..4 are already used
 * (timer/offload/counter/nsos/OTNS-radio), so line 5 is free.
 */
#define IEEE802154_OTNS_CLI_IRQ 5

/*
 * ---------------------------------------------------------------------------
 * Runner-side functions, called from the embedded side.
 * ---------------------------------------------------------------------------
 */

/* Returns true if the OTNS CLI bridge is enabled for this run. */
bool nsi_otns_cli_is_enabled(void);

/*
 * Feed CLI command bytes received from OTNS (as OT_SIM_EVENT_UART_WRITE over the
 * simulation socket) to the embedded OpenThread CLI. Called from the radio
 * runner side. Defined weakly in the radio runner so the driver still links
 * when the CLI bridge is not compiled in.
 */
void nsi_otns_cli_feed_input(const uint8_t *buf, int len);

/*
 * Copy up to @p max pending CLI input bytes into @p buf. Returns the number of
 * bytes copied (0 if none pending). Pulled by the embedded CLI ISR.
 */
int nsi_otns_cli_get_input(uint8_t *buf, int max);

/*
 * Send @p len bytes of CLI output back to OTNS (as an OT_SIM_EVENT_UART_WRITE
 * event on the simulation socket).
 */
void nsi_otns_cli_output(const uint8_t *buf, int len);

#ifdef __cplusplus
}
#endif

#endif /* OTNS_CLI_H__ */
