/*
 * Copyright (c) 2026
 * SPDX-License-Identifier: Apache-2.0
 *
 * Embedded (Zephyr CPU) side of the OTNS OpenThread CLI bridge.
 *
 * It runs the raw OpenThread CLI (otCliInputLine / otCli output callback) so
 * that replies use the exact "<output>\nDone" / "Error <n>: ..." format that
 * OTNS expects — unlike the Zephyr `ot` shell, which wraps commands and adds a
 * shell prompt. Command bytes arrive from the runner side (stdin) through
 * IEEE802154_OTNS_CLI_IRQ; CLI output is sent back to the runner side (stdout).
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

#include "otns_cli.h"

#define LINE_MAX 384
#define MSGQ_DEPTH 8
#define MSGQ_ALIGN 4
#define STACK_SIZE 3072
#define THREAD_PRIO 8
#define INIT_PRIORITY 99
#define OUTPUT_BUF_MAX 512
#define INPUT_CHUNK_MAX 256

struct line
{
    char buf[LINE_MAX];
};

K_MSGQ_DEFINE(msgq, sizeof(struct line), MSGQ_DEPTH, MSGQ_ALIGN);

K_THREAD_STACK_DEFINE(stack, STACK_SIZE);
static struct k_thread thread;

/* Line accumulator, only touched from the (serialized) CLI ISR. */
static char acc[LINE_MAX];
static int  acc_len;

/* OpenThread CLI output callback: forward formatted output to stdout. */
static int cli_output_cb(void *context, const char *format, va_list arg)
{
    char buf[OUTPUT_BUF_MAX];
    int  len;

    ARG_UNUSED(context);

    len = vsnprintf(buf, sizeof(buf), format, arg);
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

/*
 * Applies the runner side's clock-drift-derived alarm offset (see
 * ieee802154_otns_bottom.c's update_alarm_drift_offset()) right before any CLI
 * command (notably `uptime`) is handled, so replies reflect the most up to date
 * drift even if no radio activity happened to refresh it first. See
 * ieee802154_otns.c's apply_pending_drift_offset() for why this can only be
 * done from the embedded side.
 */
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

/* ISR: drain stdin bytes from the runner side and split into command lines. */
static void isr(const void *arg)
{
    uint8_t buf[INPUT_CHUNK_MAX];
    int     n;

    ARG_UNUSED(arg);

    apply_pending_drift_offset();

    while ((n = nsi_otns_cli_get_input(buf, sizeof(buf))) > 0)
    {
        for (int i = 0; i < n; i++)
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
                    continue; /* skip empty lines */
                }
                acc[acc_len] = '\0';
                memcpy(line.buf, acc, acc_len + 1);
                acc_len = 0;
                (void)k_msgq_put(&msgq, &line, K_NO_WAIT);
            }
            else if (acc_len < LINE_MAX - 1)
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

    /*
     * Wait for the OpenThread instance to be created (it is created at boot
     * by the L2 init). Use k_yield() rather than k_sleep(): during OTNS node
     * setup the simulator does not advance virtual time, so a timed sleep
     * would never elapse.
     */
    while ((instance = openthread_get_default_instance()) == NULL)
    {
        k_yield();
    }

    /* Take over the CLI output so replies go to OTNS via UART_WRITE. */
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
