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
 * Embedded (Zephyr CPU) side of the OTNS OpenThread CLI bridge.
 */

#define LOG_MODULE_NAME otns_cli
#include <zephyr/logging/log.h>
LOG_MODULE_REGISTER(LOG_MODULE_NAME, LOG_LEVEL_INF);

#include <stdarg.h>
#include <stdio.h>
#include <string.h>

#include <zephyr/init.h>
#include <zephyr/irq.h>
#include <zephyr/kernel.h>

#include <openthread.h>
#include <openthread/cli.h>
#include <openthread/instance.h>

#include <nsi_main.h>

#include "cli.h"

#define CLI_LINE_MAX 384
#define MSGQ_DEPTH 8
#define MSGQ_ALIGN 4
#define STACK_SIZE 3072
#define THREAD_PRIO 8
#define INIT_PRIORITY 99
#define OUTPUT_BUF_MAX 512
#define INPUT_CHUNK_MAX 256

struct line
{
    char buf[CLI_LINE_MAX];
};

K_MSGQ_DEFINE(msgq, sizeof(struct line), MSGQ_DEPTH, MSGQ_ALIGN);

K_THREAD_STACK_DEFINE(stack, STACK_SIZE);
static struct k_thread thread;

static char acc[CLI_LINE_MAX];
static int  acc_len;

static int cli_output_cb(void *context, const char *format, va_list arg)
{
    char buf[OUTPUT_BUF_MAX];

    ARG_UNUSED(context);

    int len = vsnprintf(buf, sizeof(buf), format, arg);

    if (len <= 0)
    {
        return 0;
    }
    if (len > (int)sizeof(buf))
    {
        len = sizeof(buf);
    }

    nsi_otns_cli_output((const uint8_t *)buf, len);
    return len;
}

extern int32_t nsi_otns_bottom_get_drift_offset_ms(void);
extern void    alarm_milli_set_time_offset_ms(int32_t offset_ms);

static void apply_pending_drift_offset(void)
{
    static int32_t last_offset_ms;
    int32_t        offset_ms = nsi_otns_bottom_get_drift_offset_ms();

    if (offset_ms != last_offset_ms)
    {
        last_offset_ms = offset_ms;
        alarm_milli_set_time_offset_ms(offset_ms);
    }
}

static void isr(const void *arg)
{
    ARG_UNUSED(arg);

    uint8_t buf[INPUT_CHUNK_MAX];
    size_t  n;

    apply_pending_drift_offset();

    while ((n = nsi_otns_cli_get_input(buf, sizeof(buf))) > 0)
    {
        for (size_t i = 0; i < n; i++)
        {
            char c = (char)buf[i];

            if (c == '\r')
            {
                continue;
            }
            if (c == '\n')
            {
                struct line line;

                if (acc_len == 0)
                {
                    continue;
                }
                acc[acc_len] = '\0';
                memcpy(line.buf, acc, acc_len + 1);
                acc_len = 0;
                (void)k_msgq_put(&msgq, &line, K_NO_WAIT);
            }
            else if (acc_len < CLI_LINE_MAX - 1)
            {
                acc[acc_len++] = c;
            }
        }
    }
}

static void thread_fn(void *a, void *b, void *c)
{
    otInstance *instance;

    ARG_UNUSED(a);
    ARG_UNUSED(b);
    ARG_UNUSED(c);

    while ((instance = openthread_get_default_instance()) == NULL)
    {
        k_yield();
    }

    openthread_mutex_lock();
    otCliInit(instance, cli_output_cb, NULL);
    openthread_mutex_unlock();

    LOG_INF("OTNS OpenThread CLI bridge ready");

    for (;;)
    {
        struct line line;

        k_msgq_get(&msgq, &line, K_FOREVER);

        if (strcmp(line.buf, "exit") == 0)
        {
            nsi_exit(0);
        }

        openthread_mutex_lock();
        otCliInputLine(line.buf);
        openthread_mutex_unlock();
    }
}

static int init(void)
{
    if (!nsi_otns_cli_is_enabled())
    {
        return 0;
    }

    IRQ_CONNECT(IEEE802154_OTNS_CLI_IRQ, 0, isr, NULL, 0);
    irq_enable(IEEE802154_OTNS_CLI_IRQ);

    k_thread_create(&thread, stack, K_THREAD_STACK_SIZEOF(stack), thread_fn, NULL, NULL, NULL,
                    K_PRIO_PREEMPT(THREAD_PRIO), 0, K_NO_WAIT);
    k_thread_name_set(&thread, "otns_cli");

    return 0;
}

SYS_INIT(init, APPLICATION, INIT_PRIORITY);
