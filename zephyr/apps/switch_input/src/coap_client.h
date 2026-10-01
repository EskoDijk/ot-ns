/*
 * SPDX-License-Identifier: Apache-2.0
 */

#ifndef COAP_CLIENT_H_
#define COAP_CLIENT_H_

#include "switch_light_protocol.h"

void switch_coap_init(void);
void switch_input_send_command(enum light_cmd cmd);

#endif /* COAP_CLIENT_H_ */
