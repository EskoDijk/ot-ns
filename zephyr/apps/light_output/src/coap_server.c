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
 * CoAP server resource for the "light" command, using Zephyr's own CoAP
 * service API (not OpenThread's otCoap API).
 */

#include <errno.h>

#include <zephyr/logging/log.h>
LOG_MODULE_REGISTER(coap_server, LOG_LEVEL_INF);

#include <zephyr/net/coap_service.h>

#include "light_output.h"
#include "switch_light_protocol.h"

static uint16_t coap_port = LIGHT_COAP_PORT;

COAP_SERVICE_DEFINE(light_coap_service, NULL, &coap_port, COAP_SERVICE_AUTOSTART);

static int light_put(struct coap_resource *resource,
                     struct coap_packet   *request,
                     struct sockaddr      *addr,
                     socklen_t             addr_len)
{
    uint16_t payload_len;

    ARG_UNUSED(resource);
    ARG_UNUSED(addr);
    ARG_UNUSED(addr_len);

    const uint8_t *payload = coap_packet_get_payload(request, &payload_len);

    if (!payload || payload_len != 1)
    {
        LOG_ERR("Light handler: missing or malformed command payload");
        return -EINVAL;
    }

    light_output_set((enum light_cmd)payload[0]);

    /* Light outputs never reply: a return of 0 suppresses any auto-ACK. */
    return 0;
}

static const char *const light_path[] = {LIGHT_COAP_URI_PATH, NULL};

COAP_RESOURCE_DEFINE(light_resource,
                     light_coap_service,
                     {
                         .put  = light_put,
                         .path = light_path,
                     });
