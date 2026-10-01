/*
 * SPDX-License-Identifier: Apache-2.0
 */

#ifndef LIGHT_OUTPUT_H_
#define LIGHT_OUTPUT_H_

#include "switch_light_protocol.h"

void light_output_init(void);
void light_output_set(enum light_cmd cmd);

#endif /* LIGHT_OUTPUT_H_ */
