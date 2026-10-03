/*
 * CoAP client wiring for the switch_input app, using Zephyr's own CoAP
 * client API (not OpenThread's otCoap API) over a plain UDP socket.
 *
 * SPDX-License-Identifier: Apache-2.0
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
    int ret;

    sock = socket(AF_INET6, SOCK_DGRAM, IPPROTO_UDP);
    if (sock < 0)
    {
        LOG_ERR("Failed to create CoAP client socket: %d", errno);
        return;
    }

    ret = coap_client_init(&client, NULL);
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
    int ret;

    if (sock < 0)
    {
        LOG_ERR("CoAP client socket not ready");
        return;
    }

    payload = (uint8_t)cmd;
    inet_pton(AF_INET6, LIGHT_COAP_MULTICAST_ADDR, &addr.sin6_addr);

    ret = coap_client_req(&client, sock, (struct sockaddr *)&addr, &req, NULL);
    if (ret)
    {
        LOG_ERR("Failed to send CoAP request: %d", ret);
    }
}
