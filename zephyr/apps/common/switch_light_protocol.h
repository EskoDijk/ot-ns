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
 * Shared CoAP protocol definitions for the light_output / switch_input apps.
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
enum light_cmd
{
    LIGHT_CMD_OFF    = 0,
    LIGHT_CMD_ON     = 1,
    LIGHT_CMD_TOGGLE = 2,
};

#endif /* SWITCH_LIGHT_PROTOCOL_H_ */
