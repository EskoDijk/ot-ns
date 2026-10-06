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
 * CoAP client wiring for the switch_input app, using Zephyr's own CoAP
 * client API (not OpenThread's otCoap API) over a plain UDP socket.
 */

#include <errno.h>

#include <zephyr/logging/log.h>
LOG_MODULE_REGISTER(coap_client_app, LOG_LEVEL_INF);

#include <zephyr/net/coap_client.h>
#include <zephyr/net/socket.h>
#include <zephyr/posix/arpa/inet.h>
#include <zephyr/posix/sys/socket.h>

#include "coap_client.h"
#include "switch_light_protocol.h"

/* RFC 7967 No-Response value suppressing replies for all response classes. */
#define NO_RESPONSE_SUPPRESS_ALL 0x1A

static struct coap_client client;
static int                sock = -1;

void switch_coap_init(void)
{
    sock = socket(AF_INET6, SOCK_DGRAM, IPPROTO_UDP);
    if (sock < 0)
    {
        LOG_ERR("Failed to create CoAP client socket: %d", errno);
        return;
    }

    int ret = coap_client_init(&client, NULL);

    if (ret)
    {
        LOG_ERR("Failed to init CoAP client: %d", ret);
    }
}

void switch_input_send_command(enum light_cmd cmd)
{
    static uint8_t      payload;
    struct sockaddr_in6 addr = {
        .sin6_family = AF_INET6,
        .sin6_port   = htons(LIGHT_COAP_PORT),
    };
    struct coap_client_request req = {
        .method      = COAP_METHOD_PUT,
        .confirmable = false,
        .path        = LIGHT_COAP_URI_PATH,
        .fmt         = COAP_CONTENT_FORMAT_TEXT_PLAIN,
        .payload     = &payload,
        .len         = sizeof(payload),
        .options =
            {
                {
                    .code  = COAP_OPTION_NO_RESPONSE,
                    .len   = 1,
                    .value = {NO_RESPONSE_SUPPRESS_ALL},
                },
            },
        .num_options = 1,
    };

    if (sock < 0)
    {
        LOG_ERR("CoAP client socket not ready");
        return;
    }

    payload = (uint8_t)cmd;
    inet_pton(AF_INET6, LIGHT_COAP_MULTICAST_ADDR, &addr.sin6_addr);

    int ret = coap_client_req(&client, sock, (struct sockaddr *)&addr, &req, NULL);

    if (ret)
    {
        LOG_ERR("Failed to send CoAP request: %d", ret);
    }
}
