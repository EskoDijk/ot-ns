/*
 * CoAP server resource for the "light" command, using Zephyr's own CoAP
 * service API (not OpenThread's otCoap API).
 *
 * SPDX-License-Identifier: Apache-2.0
 */

#include <errno.h>

#include <zephyr/logging/log.h>
LOG_MODULE_REGISTER(coap_server, LOG_LEVEL_INF);

#include <zephyr/net/coap_service.h>

#include "light_output.h"
#include "switch_light_protocol.h"

static uint16_t coap_port = LIGHT_COAP_PORT;

COAP_SERVICE_DEFINE(light_coap_service, NULL, &coap_port, COAP_SERVICE_AUTOSTART);

static int light_put(struct coap_resource *resource, struct coap_packet *request,
		     struct sockaddr *addr, socklen_t addr_len)
{
	const uint8_t *payload;
	uint16_t payload_len;

	ARG_UNUSED(resource);
	ARG_UNUSED(addr);
	ARG_UNUSED(addr_len);

	payload = coap_packet_get_payload(request, &payload_len);
	if (!payload || payload_len != 1) {
		LOG_ERR("Light handler: missing or malformed command payload");
		return -EINVAL;
	}

	light_output_set((enum light_cmd)payload[0]);

	/* Light outputs never reply: a return of 0 suppresses any auto-ACK. */
	return 0;
}

static const char *const light_path[] = { LIGHT_COAP_URI_PATH, NULL };

COAP_RESOURCE_DEFINE(light_resource, light_coap_service, {
	.put = light_put,
	.path = light_path,
});
