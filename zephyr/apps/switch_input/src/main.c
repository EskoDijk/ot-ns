/*
 * SPDX-License-Identifier: Apache-2.0
 */

#include "coap_client.h"
#include "switch_input.h"

int main(void)
{
	switch_coap_init();
	switch_input_init();

	return 0;
}
