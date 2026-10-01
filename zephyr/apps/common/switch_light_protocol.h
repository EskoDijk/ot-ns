/*
 * Shared CoAP protocol definitions for the light_output / switch_input apps.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

#ifndef SWITCH_LIGHT_PROTOCOL_H_
#define SWITCH_LIGHT_PROTOCOL_H_

/** CoAP resource path exposed by light_output and targeted by switch_input. */
#define LIGHT_COAP_URI_PATH "light"

/** CoAP port used by both apps (standard CoAP default port). */
#define LIGHT_COAP_PORT 5683

/** Realm-local all-Thread-nodes multicast address; no pairing/provisioning needed. */
#define LIGHT_COAP_MULTICAST_ADDR "ff03::1"

/** Single-byte command payload exchanged over the "light" CoAP resource. */
enum light_cmd {
	LIGHT_CMD_OFF = 0,
	LIGHT_CMD_ON = 1,
	LIGHT_CMD_TOGGLE = 2,
};

#endif /* SWITCH_LIGHT_PROTOCOL_H_ */
