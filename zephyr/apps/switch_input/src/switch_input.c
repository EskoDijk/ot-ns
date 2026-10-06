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
 * GPIO button wiring for the switch_input app: identical devicetree-driven
 * code on every board. On native_sim the sw0/sw1/sw2 aliases are bound to
 * the board's gpio_emul controller (see boards/native_sim.overlay); a
 * small shell helper is exposed in that case to pulse the emulated inputs,
 * exercising the exact same GPIO interrupt path as real hardware.
 */

#include <errno.h>

#include <zephyr/drivers/gpio.h>
#include <zephyr/kernel.h>
#include <zephyr/logging/log.h>
LOG_MODULE_DECLARE(coap_client_app);

#include "coap_client.h"
#include "switch_input.h"

struct switch_button
{
    const struct gpio_dt_spec gpio;
    struct gpio_callback      cb;
    enum light_cmd            cmd;
};

static void button_pressed(const struct device *dev, struct gpio_callback *cb, uint32_t pins);

static struct switch_button buttons[] = {
    {.gpio = GPIO_DT_SPEC_GET(DT_ALIAS(sw0), gpios), .cmd = LIGHT_CMD_ON},
    {.gpio = GPIO_DT_SPEC_GET(DT_ALIAS(sw1), gpios), .cmd = LIGHT_CMD_OFF},
    {.gpio = GPIO_DT_SPEC_GET(DT_ALIAS(sw2), gpios), .cmd = LIGHT_CMD_TOGGLE},
};

static void button_pressed(const struct device *dev, struct gpio_callback *cb, uint32_t pins)
{
    struct switch_button *button = CONTAINER_OF(cb, struct switch_button, cb);

    ARG_UNUSED(dev);
    ARG_UNUSED(pins);

    switch_input_send_command(button->cmd);
}

void switch_input_init(void)
{
    for (size_t i = 0; i < ARRAY_SIZE(buttons); i++)
    {
        struct switch_button *button = &buttons[i];

        if (!gpio_is_ready_dt(&button->gpio))
        {
            LOG_ERR("Button device not ready");
            continue;
        }

        int ret = gpio_pin_configure_dt(&button->gpio, GPIO_INPUT);

        if (ret)
        {
            LOG_ERR("Failed to configure button (%d)", ret);
            continue;
        }

        ret = gpio_pin_interrupt_configure_dt(&button->gpio, GPIO_INT_EDGE_TO_ACTIVE);
        if (ret)
        {
            LOG_ERR("Failed to configure button interrupt (%d)", ret);
            continue;
        }

        gpio_init_callback(&button->cb, button_pressed, BIT(button->gpio.pin));
        gpio_add_callback_dt(&button->gpio, &button->cb);
    }
}

#ifdef CONFIG_GPIO_EMUL
#include <string.h>
#include <zephyr/drivers/gpio/gpio_emul.h>
#include <zephyr/shell/shell.h>

static int cmd_switch_press(const struct shell *sh, size_t argc, char **argv)
{
    enum light_cmd cmd;

    if (!strcmp(argv[1], "on"))
    {
        cmd = LIGHT_CMD_ON;
    }
    else if (!strcmp(argv[1], "off"))
    {
        cmd = LIGHT_CMD_OFF;
    }
    else if (!strcmp(argv[1], "toggle"))
    {
        cmd = LIGHT_CMD_TOGGLE;
    }
    else
    {
        shell_error(sh, "Usage: switch press <on|off|toggle>");
        return -EINVAL;
    }

    for (size_t i = 0; i < ARRAY_SIZE(buttons); i++)
    {
        if (buttons[i].cmd == cmd)
        {
            /* Pulse the emulated line active then back to idle. */
            gpio_emul_input_set_dt(&buttons[i].gpio, 0);
            gpio_emul_input_set_dt(&buttons[i].gpio, 1);
            return 0;
        }
    }

    return -ENODEV;
}

SHELL_STATIC_SUBCMD_SET_CREATE(
    switch_subcmd,
    SHELL_CMD_ARG(press, NULL, "Simulate a button press: on, off or toggle", cmd_switch_press, 2, 0),
    SHELL_SUBCMD_SET_END);

SHELL_CMD_REGISTER(switch, &switch_subcmd, "Simulated switch input commands", NULL);
#endif /* CONFIG_GPIO_EMUL */
