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
 * Runner ("bottom") side of the OTNS virtual IEEE 802.15.4 radio driver.
 *
 * This file is compiled as part of the native simulator runner (host code). It
 * owns the real Unix domain socket connection to OTNS, implements the OTNS
 * simulation-event wire protocol, and — crucially — synchronizes the native
 * simulator virtual clock with the OTNS virtual time using the native simulator
 * hardware-event scheduler (a "pacer" NSI_HW_EVENT).
 */

#include <errno.h>
#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include <sys/socket.h>
#include <sys/un.h>

#include "irq_ctrl.h"
#include "nsi_cmdline.h"
#include "nsi_hw_scheduler.h"
#include "nsi_hws_models_if.h"
#include "nsi_tasks.h"
#include "nsi_tracing.h"

#include "ieee802154_priv.h"

#define PHY_BITRATE_DEFAULT 250000U
#define RADIO_MSG_HDR sizeof(((struct RadioMessage *)0)->mChannel)
#define RADIO_COMM_EVENT_DATA_SIZE sizeof(struct RadioCommEventData)
#define MAX_SLEEP_US 3600000000ULL
#define DRIFT_ACCUM_PER_US 1000000
#define RFSIM_PARAM_RSP_DATA_SIZE (sizeof(uint8_t) + sizeof(int32_t))

/* ------------------------------------------------------------------------- */
/* State                                                                     */
/* ------------------------------------------------------------------------- */

static uint32_t    cmd_node_id;
static const char *cmd_socket;
static int32_t     cmd_seed;

static int      fd = -1;
static bool     connected;
static bool     disabled;
static uint64_t last_msg_id;

static uint64_t pacer_time;

static struct otns_radio_event pending_ev;
static bool                    have_pending;

static struct otns_radio_event deliver_ev;
static bool                    deliver_ready;

static uint64_t ack_time = NSI_NEVER;
static uint8_t  ack_channel;
static int8_t   ack_power;
static uint8_t  ack_psdu[OT_RADIO_FRAME_MAX_SIZE];
static uint16_t ack_psdu_len;

static uint8_t radio_state      = OT_RADIO_STATE_DISABLED;
static uint8_t radio_channel    = OT_RADIO_2P4GHZ_OQPSK_CHANNEL_MIN;
static uint8_t reported_state   = OT_RADIO_STATE_INVALID;
static uint8_t reported_channel = OT_RADIO_STATE_INVALID;

static uint8_t ext_addr[OT_EXT_ADDRESS_SIZE];
static bool    ext_addr_valid;

static int8_t   s_rx_sensitivity  = RFSIM_RX_SENSITIVITY_DEFAULT_DBM;
static uint64_t s_phy_bitrate     = PHY_BITRATE_DEFAULT;
static int16_t  s_clock_drift_ppm = 0;
static uint8_t  s_csl_accuracy    = RFSIM_CSL_ACCURACY_DEFAULT_PPM;
static uint8_t  s_csl_uncertainty = RFSIM_CSL_UNCERTAINTY_DEFAULT_10US;

static int64_t  s_drift_ps                = 0;
static int64_t  s_drift_us_total          = 0;
static uint64_t s_drift_last_time         = 0;
static int32_t  s_drift_offset_ms_applied = 0;

/* ------------------------------------------------------------------------- */
/* Little-endian (de)serialization helpers                                   */
/* ------------------------------------------------------------------------- */

static inline void put_le64(uint8_t *p, uint64_t v)
{
    for (int i = 0; i < 8; i++)
    {
        p[i] = (v >> (8 * i)) & 0xff;
    }
}

static inline int32_t get_le32(const uint8_t *p)
{
    return (int32_t)((uint32_t)p[0] | ((uint32_t)p[1] << 8) | ((uint32_t)p[2] << 16) | ((uint32_t)p[3] << 24));
}

static inline void put_le32(uint8_t *p, int32_t v)
{
    p[0] = (uint8_t)(v & 0xff);
    p[1] = (uint8_t)((v >> 8) & 0xff);
    p[2] = (uint8_t)((v >> 16) & 0xff);
    p[3] = (uint8_t)((v >> 24) & 0xff);
}

static void update_alarm_drift_offset(uint64_t now)
{
    uint64_t elapsed = now - s_drift_last_time;

    s_drift_last_time = now;

    if (s_clock_drift_ppm == 0 || elapsed == 0)
    {
        return;
    }

    s_drift_ps += (int64_t)s_clock_drift_ppm * (int64_t)elapsed;
    if (s_drift_ps >= DRIFT_ACCUM_PER_US || s_drift_ps <= -DRIFT_ACCUM_PER_US)
    {
        int64_t whole_us = s_drift_ps / DRIFT_ACCUM_PER_US;

        s_drift_ps -= whole_us * DRIFT_ACCUM_PER_US;
        s_drift_us_total += whole_us;

        s_drift_offset_ms_applied = (int32_t)(-(s_drift_us_total / 1000));
    }
}

int32_t nsi_otns_bottom_get_drift_offset_ms(void) { return s_drift_offset_ms_applied; }

uint8_t nsi_otns_bottom_get_csl_accuracy(void) { return s_csl_accuracy; }

/* ------------------------------------------------------------------------- */
/* Socket I/O                                                                */
/* ------------------------------------------------------------------------- */

static int read_full(void *buf, size_t n)
{
    size_t got = 0;

    while (got < n)
    {
        ssize_t r = read(fd, (char *)buf + got, n - got);

        if (r > 0)
        {
            got += (size_t)r;
            continue;
        }
        if (r == 0)
        {
            fprintf(stderr, "[otns] socket closed by OTNS (EOF) after %zu/%zu bytes\n", got, n);
            return -1;
        }
        if (errno == EINTR)
        {
            continue;
        }
        fprintf(stderr, "[otns] socket read error: %s\n", strerror(errno));
        return -1;
    }
    return 0;
}

static int read_event(struct Event *ev)
{
    if (read_full(ev, sizeof(struct EventHeader)) < 0)
    {
        return -1;
    }

    if (ev->mDataLength > 0)
    {
        if (ev->mDataLength > sizeof(ev->mData))
        {
            fprintf(stderr, "[otns] event datalen %u too large (ev=%u)\n", ev->mDataLength, ev->mEvent);
            return -1;
        }
        if (read_full(ev->mData, ev->mDataLength) < 0)
        {
            return -1;
        }
    }
    return 0;
}

static int write_event(uint8_t type, uint64_t delay, const uint8_t *data, uint16_t datalen)
{
    if (fd < 0)
    {
        return -1;
    }
    if (datalen > OT_EVENT_DATA_MAX_SIZE)
    {
        return -1;
    }

    struct Event event;
    event.mDelay      = delay;
    event.mEvent      = type;
    event.mMsgId      = last_msg_id;
    event.mDataLength = datalen;
    if (datalen > 0)
    {
        memcpy(event.mData, data, datalen);
    }

    size_t total = sizeof(struct EventHeader) + datalen;
    size_t sent  = 0;
    while (sent < total)
    {
        ssize_t w = write(fd, (uint8_t *)&event + sent, total - sent);

        if (w > 0)
        {
            sent += (size_t)w;
            continue;
        }
        if (w < 0 && errno == EINTR)
        {
            continue;
        }
        return -1;
    }
    return 0;
}

/* ------------------------------------------------------------------------- */
/* Connection / handshake                                                    */
/* ------------------------------------------------------------------------- */

static void disconnect(void)
{
    if (fd >= 0)
    {
        close(fd);
        fd = -1;
    }
    connected  = false;
    disabled   = true;
    pacer_time = NSI_NEVER;
    nsi_print_warning("ieee802154_otns: disconnected from OTNS\n");
}

static int try_connect(void)
{
    if (cmd_socket == NULL || cmd_socket[0] == '\0')
    {
        return -1;
    }

    size_t             path_len = strlen(cmd_socket);
    struct sockaddr_un addr;
    if (path_len >= sizeof(addr.sun_path))
    {
        nsi_print_warning("ieee802154_otns: socket path too long\n");
        return -1;
    }

    fd = socket(AF_UNIX, SOCK_STREAM, 0);
    if (fd < 0)
    {
        nsi_print_warning("ieee802154_otns: socket() failed\n");
        return -1;
    }

    memset(&addr, 0, sizeof(addr));
    addr.sun_family = AF_UNIX;
    memcpy(addr.sun_path, cmd_socket, path_len);

    if (connect(fd, (struct sockaddr *)&addr, sizeof(addr)) < 0)
    {
        nsi_print_warning("ieee802154_otns: cannot connect to OTNS at %s\n", cmd_socket);
        close(fd);
        fd = -1;
        return -1;
    }

    connected = true;

    {
        uint8_t node[4];

        node[0] = cmd_node_id & 0xff;
        node[1] = (cmd_node_id >> 8) & 0xff;
        node[2] = (cmd_node_id >> 16) & 0xff;
        node[3] = (cmd_node_id >> 24) & 0xff;
        (void)write_event(OT_SIM_EVENT_NODE_INFO, 0, node, sizeof(node));
    }

    if (ext_addr_valid)
    {
        (void)write_event(OT_SIM_EVENT_EXT_ADDR, 0, ext_addr, OT_EXT_ADDRESS_SIZE);
    }

    nsi_print_trace("ieee802154_otns: connected to OTNS (node %u) at %s\n", cmd_node_id, cmd_socket);
    return 0;
}

/* ------------------------------------------------------------------------- */
/* Pacer (virtual-time lock-step)                                            */
/* ------------------------------------------------------------------------- */

static uint64_t next_other_event(void)
{
    uint64_t saved = pacer_time;

    pacer_time = NSI_NEVER;
    nsi_hws_find_next_event();
    uint64_t next = nsi_hws_get_next_event_time();
    pacer_time    = saved;

    return next;
}

static void deliver_to_embedded(const struct otns_radio_event *ev)
{
    deliver_ev    = *ev;
    deliver_ready = true;

    hw_irq_ctrl_raise_im(IEEE802154_OTNS_IRQ);
}

static void parse_radio_event(struct otns_radio_event *out, const struct Event *raw)
{
    out->type          = raw->mEvent;
    out->data.mChannel = raw->mData[0];
    out->data.mPower   = (int8_t)raw->mData[1];
    out->data.mError   = raw->mData[2];
    out->psdu_len      = 0;

    if (raw->mEvent == OT_SIM_EVENT_RADIO_RX_DONE || raw->mEvent == OT_SIM_EVENT_RADIO_COMM_START)
    {
        if (raw->mDataLength > RADIO_COMM_EVENT_DATA_SIZE + RADIO_MSG_HDR)
        {
            uint16_t len = raw->mDataLength - RADIO_COMM_EVENT_DATA_SIZE - RADIO_MSG_HDR;

            if (len > sizeof(out->psdu))
            {
                len = sizeof(out->psdu);
            }
            memcpy(out->psdu, &raw->mData[RADIO_COMM_EVENT_DATA_SIZE + RADIO_MSG_HDR], len);
            out->psdu_len = len;
        }
    }
}

static int event_is_for_embedded(uint8_t type)
{
    return type == OT_SIM_EVENT_RADIO_RX_DONE || type == OT_SIM_EVENT_RADIO_TX_DONE ||
           type == OT_SIM_EVENT_RADIO_CHAN_SAMPLE;
}

static void handle_rfsim_param_event(uint8_t event_type, const uint8_t *data, uint16_t datalen)
{
    uint8_t data_out[RFSIM_PARAM_RSP_DATA_SIZE];

    if (datalen < sizeof(data_out))
    {
        return;
    }

    uint8_t param = data[0];
    int32_t value = get_le32(&data[1]);

    if (event_type == OT_SIM_EVENT_RFSIM_PARAM_SET)
    {
        switch (param)
        {
        case RFSIM_PARAM_RX_SENSITIVITY:
            s_rx_sensitivity = (int8_t)value;
            break;
        case RFSIM_PARAM_PHY_BITRATE:
            s_phy_bitrate = (value < 1) ? 1 : (uint64_t)value;
            break;
        case RFSIM_PARAM_CLOCK_DRIFT:
            s_clock_drift_ppm = (int16_t)value;
            s_drift_last_time = nsi_hws_get_time();
            break;
        case RFSIM_PARAM_CSL_ACCURACY:
            s_csl_accuracy = (uint8_t)value;
            break;
        case RFSIM_PARAM_CSL_UNCERTAINTY:
            s_csl_uncertainty = (uint8_t)value;
            break;
        default:
            break;
        }
    }

    switch (param)
    {
    case RFSIM_PARAM_RX_SENSITIVITY:
        value = (int32_t)s_rx_sensitivity;
        break;
    case RFSIM_PARAM_PHY_BITRATE:
        value = (int32_t)s_phy_bitrate;
        break;
    case RFSIM_PARAM_CLOCK_DRIFT:
        value = (int32_t)s_clock_drift_ppm;
        break;
    case RFSIM_PARAM_CSL_ACCURACY:
        value = (int32_t)s_csl_accuracy;
        break;
    case RFSIM_PARAM_CSL_UNCERTAINTY:
        value = (int32_t)s_csl_uncertainty;
        break;
    default:
        param = RFSIM_PARAM_UNKNOWN;
        value = 0;
        break;
    }

    data_out[0] = param;
    put_le32(&data_out[1], value);
    (void)write_event(OT_SIM_EVENT_RFSIM_PARAM_RSP, 0, data_out, sizeof(data_out));
}

__attribute__((weak)) void nsi_otns_cli_feed_input(const uint8_t *buf, int len)
{
    (void)buf;
    (void)len;
}

static void report_state(void)
{
    uint8_t data[sizeof(struct RadioStateEventData)];

    if (radio_state == reported_state && radio_channel == reported_channel)
    {
        return;
    }

    if (radio_state == OT_RADIO_STATE_DISABLED)
    {
        return;
    }

    data[0] = radio_channel;
    data[1] = 0;
    data[2] = (uint8_t)s_rx_sensitivity;
    data[3] = radio_state;
    data[4] = 0;
    data[5] = radio_state;
    put_le64(&data[6], nsi_hws_get_time());

    (void)write_event(OT_SIM_EVENT_RADIO_STATE, 0, data, sizeof(data));

    reported_state   = radio_state;
    reported_channel = radio_channel;
}

static void pacer(void)
{
    if (disabled)
    {
        pacer_time = NSI_NEVER;
        return;
    }

    if (!connected)
    {
        if (try_connect() < 0)
        {
            disabled   = true;
            pacer_time = NSI_NEVER;
            return;
        }
    }

    uint64_t now = nsi_hws_get_time();

    update_alarm_drift_offset(now);

    if (have_pending)
    {
        have_pending = false;
        deliver_to_embedded(&pending_ev);
    }

    for (;;)
    {
        uint64_t next = next_other_event();
#ifdef OTNS_TRACE
        fprintf(stderr, "[otns] pacer now=%llu next=%llu\n", (unsigned long long)now, (unsigned long long)next);
#endif

        if (next != NSI_NEVER && next <= now)
        {
            pacer_time = now;
            return;
        }

        uint64_t delay = (next == NSI_NEVER) ? MAX_SLEEP_US : (next - now);

        report_state();

        if (write_event(OT_SIM_EVENT_ALARM_FIRED, delay, NULL, 0) < 0)
        {
            fprintf(stderr, "[otns] write ALARM_FIRED failed\n");
            disconnect();
            return;
        }

        struct Event raw;

        if (read_event(&raw) < 0)
        {
            fprintf(stderr, "[otns] read event failed\n");
            disconnect();
            return;
        }
        last_msg_id = raw.mMsgId;
#ifdef OTNS_TRACE
        fprintf(stderr, "[otns] recv ev=%u delay=%llu\n", raw.mEvent, (unsigned long long)raw.mDelay);
#endif

        if (!event_is_for_embedded(raw.mEvent))
        {
            if (raw.mEvent == OT_SIM_EVENT_UART_WRITE)
            {
                nsi_otns_cli_feed_input(raw.mData, raw.mDataLength);
            }
            else if (raw.mEvent == OT_SIM_EVENT_RFSIM_PARAM_GET || raw.mEvent == OT_SIM_EVENT_RFSIM_PARAM_SET)
            {
                handle_rfsim_param_event(raw.mEvent, raw.mData, raw.mDataLength);
            }
            pacer_time = now + raw.mDelay;
            return;
        }

        if (raw.mDelay == 0)
        {
            struct otns_radio_event ev;

            parse_radio_event(&ev, &raw);
            deliver_to_embedded(&ev);
            continue;
        }

        parse_radio_event(&pending_ev, &raw);
        have_pending = true;
        pacer_time   = now + raw.mDelay;
        return;
    }
}

NSI_HW_EVENT(pacer_time, pacer, 950);

/* ------------------------------------------------------------------------- */
/* Command line options                                                      */
/* ------------------------------------------------------------------------- */

static void register_cmdline_opts(void)
{
    static struct args_struct_t cmdline_options[] = {
        {
            .option   = "otns-node-id",
            .name     = "id",
            .type     = 'u',
            .dest     = (void *)&cmd_node_id,
            .descript = "OTNS node id assigned to this node (>= 1)",
        },
        {
            .option   = "otns-socket",
            .name     = "path",
            .type     = 's',
            .dest     = (void *)&cmd_socket,
            .descript = "Path of the OTNS dispatcher Unix domain socket",
        },
        {
            .option   = "otns-seed",
            .name     = "seed",
            .type     = 'i',
            .dest     = (void *)&cmd_seed,
            .descript = "Optional OTNS random seed",
        },
        ARG_TABLE_ENDMARKER,
    };

    nsi_add_command_line_opts(cmdline_options);
}

NSI_TASK(register_cmdline_opts, PRE_BOOT_1, 200);

static void boot(void)
{
    if (cmd_socket != NULL && cmd_socket[0] != '\0')
    {
        pacer_time = 0;
    }
    else
    {
        disabled   = true;
        pacer_time = NSI_NEVER;
    }
}

NSI_TASK(boot, HW_INIT, 500);

static void cleanup(void)
{
    if (fd >= 0)
    {
        close(fd);
        fd = -1;
    }
}

NSI_TASK(cleanup, ON_EXIT_PRE, 100);

/* ------------------------------------------------------------------------- */
/* Boundary functions called from the embedded side                         */
/* ------------------------------------------------------------------------- */

int nsi_otns_bottom_get_node_id(void) { return (int)cmd_node_id; }

bool nsi_otns_bottom_is_connected(void) { return connected; }

bool nsi_otns_bottom_is_configured(void) { return cmd_socket != NULL && cmd_socket[0] != '\0'; }

static void wake_pacer_now(void)
{
    if (disabled || !connected)
    {
        return;
    }
    if (have_pending)
    {
        return;
    }
    pacer_time = nsi_hws_get_time();
    nsi_hws_find_next_event();
}

int nsi_otns_bottom_tx(uint8_t channel, int8_t power, const uint8_t *psdu, uint16_t len)
{
    if (!connected)
    {
        return -1;
    }
    if (len > OT_RADIO_FRAME_MAX_SIZE)
    {
        return -1;
    }

    uint64_t duration = (uint64_t)(OT_RADIO_SHR_PHR_LENGTH_BYTES + len) * 8U * 1000000U / s_phy_bitrate;

    uint8_t data[RADIO_COMM_EVENT_DATA_SIZE + RADIO_MSG_HDR + OT_RADIO_FRAME_MAX_SIZE];
    data[0] = channel;
    data[1] = (uint8_t)power;
    data[2] = OT_ERROR_NONE;
    put_le64(&data[3], duration);
    data[RADIO_COMM_EVENT_DATA_SIZE] = channel;

    uint16_t hdr_len = RADIO_COMM_EVENT_DATA_SIZE + RADIO_MSG_HDR;
    memcpy(&data[hdr_len], psdu, len);

    if (write_event(OT_SIM_EVENT_RADIO_COMM_START, 0, data, (uint16_t)(hdr_len + len)) < 0)
    {
        return -1;
    }

    wake_pacer_now();
    return 0;
}

static void ack_timer_fired(void)
{
    ack_time = NSI_NEVER;
    (void)nsi_otns_bottom_tx(ack_channel, ack_power, ack_psdu, ack_psdu_len);
}

NSI_HW_EVENT(ack_time, ack_timer_fired, 940);

int nsi_otns_bottom_tx_after(uint8_t channel, int8_t power, const uint8_t *psdu, uint16_t len, uint32_t delay_us)
{
    if (!connected)
    {
        return -1;
    }
    if (len > OT_RADIO_FRAME_MAX_SIZE)
    {
        return -1;
    }

    ack_channel = channel;
    ack_power   = power;
    memcpy(ack_psdu, psdu, len);
    ack_psdu_len = len;
    ack_time     = nsi_hws_get_time() + delay_us;

    nsi_hws_find_next_event();
    return 0;
}

int nsi_otns_bottom_cca(uint8_t channel)
{
    uint8_t data[RADIO_COMM_EVENT_DATA_SIZE];

    if (!connected)
    {
        return -1;
    }

    data[0] = channel;
    data[1] = 0;
    data[2] = OT_ERROR_NONE;
    put_le64(&data[3], OT_RADIO_CCA_TIME_US);

    if (write_event(OT_SIM_EVENT_RADIO_CHAN_SAMPLE, 0, data, sizeof(data)) < 0)
    {
        return -1;
    }

    wake_pacer_now();
    return 0;
}

void nsi_otns_bottom_set_state(uint8_t state, uint8_t channel)
{
    radio_state   = state;
    radio_channel = channel;
    wake_pacer_now();
}

void nsi_otns_bottom_send_uart(const uint8_t *buf, uint16_t len)
{
    if (!connected || buf == NULL || len == 0)
    {
        return;
    }

    (void)write_event(OT_SIM_EVENT_UART_WRITE, 0, buf, len);
    wake_pacer_now();
}

void nsi_otns_bottom_send_ext_addr(const uint8_t *ext_addr_be)
{
    if (ext_addr_be == NULL)
    {
        return;
    }

    memcpy(ext_addr, ext_addr_be, OT_EXT_ADDRESS_SIZE);
    ext_addr_valid = true;

    if (!connected)
    {
        return;
    }

    (void)write_event(OT_SIM_EVENT_EXT_ADDR, 0, ext_addr, OT_EXT_ADDRESS_SIZE);
    wake_pacer_now();
}

void nsi_otns_bottom_send_status(const char *status, uint16_t len)
{
    if (!connected || status == NULL || len == 0)
    {
        return;
    }

    (void)write_event(OT_SIM_EVENT_OTNS_STATUS_PUSH, 0, (const uint8_t *)status, len);
    wake_pacer_now();
}

bool nsi_otns_bottom_get_event(struct otns_radio_event *ev)
{
    if (!deliver_ready)
    {
        return false;
    }

    *ev           = deliver_ev;
    deliver_ready = false;
    return true;
}
