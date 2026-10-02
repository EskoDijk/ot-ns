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
 * See ieee802154_isr.h.
 */

#include <errno.h>
#include <string.h>

#include <zephyr/kernel.h>
#include <zephyr/net/net_if.h>
#include <zephyr/net/net_pkt.h>
#include <zephyr/sys/byteorder.h>

#include "ieee802154_data.h"
#include "ieee802154_frame.h"
#include "ieee802154_isr.h"
#include "ieee802154_priv.h"
#include "radio.h"

#define LOG_MODULE_NAME ieee802154_otns
#if defined(CONFIG_IEEE802154_OTNS_LOG_LEVEL)
#define LOG_LEVEL CONFIG_IEEE802154_OTNS_LOG_LEVEL
#else
#define LOG_LEVEL LOG_LEVEL_INF
#endif

#include <zephyr/logging/log.h>
LOG_MODULE_DECLARE(LOG_MODULE_NAME);

/* ------------------------------------------------------------------------- */
/* RX frame delivery to the net stack                                        */
/* ------------------------------------------------------------------------- */

static void deliver_rx(const struct otns_radio_event *ev, bool acked_with_fpb)
{
    struct net_pkt *pkt;
    uint16_t        mac_len;

    if (ev->psdu_len < FCS_SIZE)
    {
        return;
    }

    if (IS_ENABLED(CONFIG_IEEE802154_L2_PKT_INCL_FCS))
    {
        mac_len = ev->psdu_len;
    }
    else
    {
        mac_len = ev->psdu_len - FCS_SIZE;
    }

    pkt = net_pkt_rx_alloc_with_buffer(data.iface, mac_len, NET_AF_UNSPEC, 0, K_NO_WAIT);
    if (pkt == NULL)
    {
        LOG_WRN("No RX pkt available, dropping frame");
        return;
    }

    if (net_pkt_write(pkt, ev->psdu, mac_len) < 0)
    {
        LOG_WRN("Failed to write RX frame");
        net_pkt_unref(pkt);
        return;
    }

    net_pkt_set_ieee802154_lqi(pkt, LQI_PERFECT);
    net_pkt_set_ieee802154_rssi_dbm(pkt, ev->data.mPower);
    /* Tell OT whether the ACK we (auto-)sent for this frame had Frame Pending set,
     * so DataPollHandler::HandleDataPoll() actually triggers the indirect Tx. */
    net_pkt_set_ieee802154_ack_fpb(pkt, acked_with_fpb);

    if (net_recv_data(data.iface, pkt) < 0)
    {
        LOG_DBG("RX frame dropped by net stack");
        net_pkt_unref(pkt);
    }
}

static void handle_rx(const struct otns_radio_event *ev)
{
    struct frame_addr_info info;
    uint16_t               fcf;
    bool                   for_me;

    if (ev->psdu_len < MIN_FRAME_SIZE)
    {
        return;
    }

    fcf = sys_get_le16(ev->psdu);

    if ((fcf & FCF_FRAME_TYPE_MASK) == FCF_FRAME_TYPE_ACK)
    {
        if (data.tx_wants_ack && ev->psdu[2] == data.tx_seq)
        {
            uint16_t l = ev->psdu_len;

            if (l > sizeof(data.ack_psdu))
            {
                l = sizeof(data.ack_psdu);
            }
            memcpy(data.ack_psdu, ev->psdu, l);
            data.ack_len = l;
            k_sem_give(&data.tx_wait);
        }
        return;
    }

    for_me = frame_is_for_me(ev->psdu, ev->psdu_len, &info);

    if (for_me && (info.fcf & FCF_ACK_REQ_BIT))
    {
        bool fpb = frame_pending_for(ev->psdu, ev->psdu_len, &info);

        deliver_rx(ev, fpb);
        schedule_ack(ev->psdu, ev->psdu_len, ev->data.mPower, &info);
    }
    else
    {
        deliver_rx(ev, false);
    }
}

/* ------------------------------------------------------------------------- */
/* ISR / event handling                                                      */
/* ------------------------------------------------------------------------- */

/*
 * Applies the runner side's clock-drift-derived alarm offset (see
 * ieee802154_otns_bottom.c's update_alarm_drift_offset()) to OpenThread's own
 * notion of elapsed time. alarm_milli_set_time_offset_ms() lives in Zephyr's
 * OpenThread platform layer (zephyr/modules/openthread/platform/alarm_milli.c)
 * and can only be called from the embedded side: it is otherwise unreferenced
 * in our build (none of our Kconfig options enable its other callers in
 * alarm_micro.c/alarm_counter.c), so the embedded image's own link would
 * discard it before the runner side ever gets a chance to call it.
 */
extern void alarm_milli_set_time_offset_ms(int32_t offset_ms);

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

static int err_to_errno(uint8_t error)
{
    switch (error)
    {
    case OT_ERROR_NONE:
        return 0;
    case OT_ERROR_CHANNEL_ACCESS_FAILURE:
        return -EBUSY;
    case OT_ERROR_ABORT:
        return -EIO;
    default:
        return -EIO;
    }
}

static void handle_tx_done(const struct otns_radio_event *ev)
{
    if (data.ack_tx_pending)
    {
        data.ack_tx_pending = false;
        if (data.sleep_pending)
        {
            data.sleep_pending = false;
            nsi_otns_bottom_set_state(OT_RADIO_STATE_SLEEP, data.channel);
        }
        k_sem_give(&data.ack_tx_done);
        return;
    }
    data.tx_result = err_to_errno(ev->data.mError);
    if (!data.tx_wants_ack || data.tx_result != 0)
    {
        k_sem_give(&data.tx_wait);
    }
}

static void handle_chan_sample(const struct otns_radio_event *ev)
{
    if (data.ed_scan_pending)
    {
        energy_scan_done_cb_t cb = data.ed_done_cb;

        data.ed_scan_pending = false;
        data.ed_done_cb      = NULL;
        if (cb != NULL)
        {
            cb(radio_dev, ev->data.mPower);
        }
        return;
    }
    data.cca_channel_free = (ev->data.mPower == (int8_t)OT_RADIO_RSSI_INVALID) ||
                            (ev->data.mPower < RFSIM_CCA_ED_THRESHOLD_DEFAULT_DBM);
    k_sem_give(&data.cca_wait);
}

void isr(const void *arg)
{
    struct otns_radio_event ev;

    ARG_UNUSED(arg);

    apply_pending_drift_offset();

    while (nsi_otns_bottom_get_event(&ev))
    {
        switch (ev.type)
        {
        case OT_SIM_EVENT_RADIO_RX_DONE:
            handle_rx(&ev);
            break;

        case OT_SIM_EVENT_RADIO_TX_DONE:
            handle_tx_done(&ev);
            break;

        case OT_SIM_EVENT_RADIO_CHAN_SAMPLE:
            handle_chan_sample(&ev);
            break;

        default:
            break;
        }
    }
}
