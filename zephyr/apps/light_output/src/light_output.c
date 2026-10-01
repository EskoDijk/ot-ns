/*
 * SPDX-License-Identifier: Apache-2.0
 */

#include <zephyr/drivers/gpio.h>
#include <zephyr/logging/log.h>
LOG_MODULE_DECLARE(coap_server);

#include "light_output.h"

static const struct gpio_dt_spec led = GPIO_DT_SPEC_GET(DT_ALIAS(led0), gpios);
static int state;

void light_output_init(void)
{
	if (!gpio_is_ready_dt(&led)) {
		LOG_ERR("LED device not ready");
		return;
	}

	gpio_pin_configure_dt(&led, GPIO_OUTPUT_INACTIVE);
}

void light_output_set(enum light_cmd cmd)
{
	switch (cmd) {
	case LIGHT_CMD_ON:
		state = 1;
		break;
	case LIGHT_CMD_OFF:
		state = 0;
		break;
	case LIGHT_CMD_TOGGLE:
		state = !state;
		break;
	default:
		LOG_WRN("Unknown light command: %d", cmd);
		return;
	}

	gpio_pin_set_dt(&led, state);
	LOG_INF("Light: %s", state ? "ON" : "OFF");
}
