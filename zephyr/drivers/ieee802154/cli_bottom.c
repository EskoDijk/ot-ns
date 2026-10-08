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
 * Runner ("bottom") side of the OTNS OpenThread CLI bridge.
 */

#include <errno.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include "irq_ctrl.h"
#include "nsi_cmdline.h"
#include "nsi_tasks.h"
#include "nsi_tracing.h"

#include "cli.h"

extern int  nsi_otns_bottom_is_configured(void);
extern void nsi_otns_bottom_send_uart(const uint8_t *buf, uint16_t len);

#define RING_SIZE 8192U
#define RING_MASK (RING_SIZE - 1U)

#define CMDLINE_TASK_PRIO 210
#define BOOT_TASK_PRIO 400

static bool flag;
static bool enabled;

static uint8_t           ring[RING_SIZE];
static volatile uint32_t ring_head;
static volatile uint32_t ring_tail;

static void register_cmdline_opts(void)
{
    static struct args_struct_t options[] = {
        {
            .is_switch = true,
            .option    = "otns-cli",
            .type      = 'b',
            .dest      = (void *)&flag,
            .descript  = "Serve the OpenThread CLI to OTNS over the simulation "
                         "socket (implied when --otns-socket is given)",
        },
        ARG_TABLE_ENDMARKER,
    };

    nsi_add_command_line_opts(options);
}

NSI_TASK(register_cmdline_opts, PRE_BOOT_1, CMDLINE_TASK_PRIO);

static void boot(void)
{
    enabled = flag || nsi_otns_bottom_is_configured();
    if (!enabled)
    {
        return;
    }

    if (dup2(STDERR_FILENO, STDOUT_FILENO) < 0)
    {
        nsi_print_warning("ieee802154_otns cli: dup2 failed\n");
    }
}

NSI_TASK(boot, HW_INIT, BOOT_TASK_PRIO);

bool nsi_otns_cli_is_enabled(void) { return enabled; }

void nsi_otns_cli_feed_input(const uint8_t *buf, size_t len)
{
    bool woke = false;

    if (!enabled || buf == NULL)
    {
        return;
    }

    for (size_t i = 0; i < len; i++)
    {
        uint32_t next = (ring_head + 1U) & RING_MASK;

        if (next == ring_tail)
        {
            break;
        }
        ring[ring_head] = buf[i];
        ring_head       = next;
        woke            = true;
    }

    if (woke)
    {
        hw_irq_ctrl_set_irq(IEEE802154_OTNS_CLI_IRQ);
    }
}

size_t nsi_otns_cli_get_input(uint8_t *buf, size_t max)
{
    size_t count = 0;

    while (count < max && ring_tail != ring_head)
    {
        buf[count++] = ring[ring_tail];
        ring_tail    = (ring_tail + 1U) & RING_MASK;
    }
    return count;
}

void nsi_otns_cli_output(const uint8_t *buf, size_t len)
{
    if (!enabled || len == 0)
    {
        return;
    }

    nsi_otns_bottom_send_uart(buf, (uint16_t)len);
}
