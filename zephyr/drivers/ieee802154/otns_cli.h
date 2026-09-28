/*
 * Copyright (c) 2026
 * SPDX-License-Identifier: Apache-2.0
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
