/*
 *  Copyright (c) 2018-2024, The OpenThread Authors.
 *  All rights reserved.
 *
 *  Redistribution and use in source and binary forms, with or without
 *  modification, are permitted provided that the following conditions are met:
 *  1. Redistributions of source code must retain the above copyright
 *     notice, this list of conditions and the following disclaimer.
 *  2. Redistributions in binary form must reproduce the above copyright
 *     notice, this list of conditions and the following disclaimer in the
 *     documentation and/or other materials provided with the distribution.
 *  3. Neither the name of the copyright holder nor the
 *     names of its contributors may be used to endorse or promote products
 *     derived from this software without specific prior written permission.
 *
 *  THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
 *  AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
 *  IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
 *  ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
 *  LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
 *  CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
 *  SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
 *  INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
 *  CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
 *  ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
 *  POSSIBILITY OF SUCH DAMAGE.
 */

/**
 * @file
 * @brief
 *   This file includes the platform-specific initializers and processing functions
 *   to let the simulated OT node communicate with the simulator.
 */

#include "platform-rfsim.h"

#include <assert.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>

#include <openthread/tasklet.h>
#include <openthread/udp.h>
#include <openthread/logging.h>

#include "common/debug.hpp"
#include "utils/code_utils.h"
#include "utils/uart.h"

#include "event-sim.h"

#define VERIFY_EVENT_SIZE(X) OT_ASSERT( (payloadLen >= sizeof(X)) && "received event payload too small" );

extern int gSockFd;

uint64_t gLastMsgId = 0;
struct Event gLastRecvEvent;
static otIp6Address unspecifiedIp6Address;

void platformRfsimInit(void) {
    if(otIp6AddressFromString("::", &unspecifiedIp6Address) != OT_ERROR_NONE) {
        platformExit(EXIT_FAILURE);
    }
}

void platformExit(int exitCode) {
    gTerminate = true;
    otPlatLog(OT_LOG_LEVEL_NOTE,OT_LOG_REGION_PLATFORM,
              "Exiting with exit code %d.", exitCode);
    exit(exitCode);
}

void platformReceiveEvent(otInstance *aInstance)
{
    struct Event event;
    ssize_t      rval = recvfrom(gSockFd, (char *)&event, sizeof(struct EventHeader), 0, NULL, NULL);
    const uint8_t *evData = event.mData;
    otError error;

    if (rval < 0)
    {
        perror("recvfrom");
        platformExit(EXIT_FAILURE);
    }
    OT_ASSERT(rval >= sizeof(struct EventHeader));

    // read the rest of data (payload data - optional).
    uint16_t payloadLen = event.mDataLength;
    if (payloadLen > 0) {
        OT_ASSERT(payloadLen <= OT_EVENT_DATA_MAX_SIZE);

        rval = recvfrom(gSockFd, (char *)&event.mData, payloadLen, 0, NULL, NULL);
        if (rval < 0)
        {
            perror("recvfrom");
            platformExit(EXIT_FAILURE);
        }
        OT_ASSERT(rval == (ssize_t) payloadLen);
    }

    gLastRecvEvent = event;
    gLastMsgId = event.mMsgId;

    platformAlarmAdvanceNow(event.mDelay);

    switch (event.mEvent)
    {
    case OT_SIM_EVENT_ALARM_FIRED:
        // Alarm events may be used to wake the node again when some simulated time has passed.
        break;

    case OT_SIM_EVENT_UART_WRITE:
        otPlatUartReceived(event.mData, event.mDataLength);
        break;

    case OT_SIM_EVENT_RADIO_COMM_START:
        VERIFY_EVENT_SIZE(struct RadioCommEventData)
        platformRadioRxStart(aInstance, (struct RadioCommEventData *)evData);
        break;

    case OT_SIM_EVENT_RADIO_RX_DONE:
        VERIFY_EVENT_SIZE(struct RadioCommEventData)
        const size_t sz = sizeof(struct RadioCommEventData);
        platformRadioRxDone(aInstance, evData + sz,
                       event.mDataLength - sz, (struct RadioCommEventData *)evData);
        break;

    case OT_SIM_EVENT_RADIO_TX_DONE:
        VERIFY_EVENT_SIZE(struct RadioCommEventData)
        platformRadioTxDone(aInstance, (struct RadioCommEventData *)evData);
        break;

    case OT_SIM_EVENT_RADIO_CHAN_SAMPLE:
        VERIFY_EVENT_SIZE(struct RadioCommEventData)
        // TODO consider also energy-detect case. This only does CCA now.
        platformRadioCcaDone(aInstance, (struct RadioCommEventData *)evData);
        break;

    case OT_SIM_EVENT_RFSIM_PARAM_GET:
        VERIFY_EVENT_SIZE(struct RfSimParamEventData)
        platformRadioRfSimParamGet(aInstance, (struct RfSimParamEventData *)evData);
        break;

    case OT_SIM_EVENT_RFSIM_PARAM_SET:
        VERIFY_EVENT_SIZE(struct RfSimParamEventData)
        platformRadioRfSimParamSet(aInstance, (struct RfSimParamEventData *)evData);
        platformRadioReportStateToSimulator(true);
        break;

    case OT_SIM_EVENT_UDP_FROM_HOST:
        otLogWarnPlat("FIXME start platformUdpFromHost");
        VERIFY_EVENT_SIZE(struct MsgToHostEventData)
        error = platformUdpFromHost(aInstance, (struct MsgToHostEventData *)evData,
                event.mData + sizeof(struct MsgToHostEventData), payloadLen - sizeof(struct MsgToHostEventData));
        if (error != OT_ERROR_NONE) {
            otLogCritPlat("Error handling UDP from host event: %s", otThreadErrorToString(error));
        }
        otLogWarnPlat("FIXME done platformUdpFromHost");
        break;

    default:
        OT_ASSERT(false && "Unrecognized event type received");
    }
}

void otPlatOtnsStatus(const char *aStatus)
{
    uint16_t     statusLength = (uint16_t)strlen(aStatus);
    if (statusLength > OT_EVENT_DATA_MAX_SIZE){
        statusLength = OT_EVENT_DATA_MAX_SIZE;
        OT_ASSERT(statusLength <= OT_EVENT_DATA_MAX_SIZE);
    }
    otSimSendOtnsStatusPushEvent(aStatus, statusLength);
}

otError platformUdpFromHost(otInstance *aInstance, const struct MsgToHostEventData *evData, uint8_t *msg, size_t msgLen) {
    otMessage        *message  = NULL;
    otError           error    = OT_ERROR_NONE;

    message = otIp6NewMessage(aInstance, NULL);
    otEXPECT_ACTION(message != NULL, error = OT_ERROR_NO_BUFS);

    // TODO set IPv6 header

    //otMessageSetOrigin(message, OT_MESSAGE_ORIGIN_HOST_UNTRUSTED);

    // TODO append UDP header

    error = otMessageAppend(message, msg, msgLen - 1);
    otEXPECT(error == OT_ERROR_NONE);

    error = otIp6Send(aInstance, message);
    otLogWarnPlat("FIXME error = %u", error);

exit:
    return error;
}

#if OPENTHREAD_CONFIG_UDP_FORWARD_ENABLE
void platformUdpForwarder(otMessage *aMessage,
                          uint16_t aPeerPort,
                          otIp6Address *aPeerAddr,
                          uint16_t aSockPort,
                          void *aContext)
{
    OT_UNUSED_VARIABLE(aContext);

    struct MsgToHostEventData evData;
    uint8_t buf[OPENTHREAD_CONFIG_IP6_MAX_DATAGRAM_LENGTH];
    size_t msgLen = otMessageGetLength(aMessage);

    OT_ASSERT(msgLen <= sizeof(buf));

    evData.mSrcPort = aSockPort;
    evData.mDstPort = aPeerPort;
    memcpy(evData.mSrcIp6, &unspecifiedIp6Address, OT_IP6_ADDRESS_SIZE);
    memcpy(evData.mDstIp6, aPeerAddr, OT_IP6_ADDRESS_SIZE);
    otMessageRead(aMessage, 0, buf, msgLen);

    otSimSendMsgToHostEvent(OT_SIM_EVENT_UDP_TO_HOST, &evData, &buf[0], msgLen);
}
#endif

// utility function to check IPv6 address for fe80::/10 or ffx2::/16 prefix -> link-local.
static bool isLinkLocal(otIp6Address *addr)
{
    return (addr->mFields.m8[0] == 0xfe && (addr->mFields.m8[1] & 0b11000000) == 0x80)
           || (addr->mFields.m8[0] == 0xff && (addr->mFields.m8[1] & 0b00001111) == 0x02);
}

// utility function that returns IPv6 address' multicast scope 0x0-0xf or 0xff for parse-error.
static uint8_t ip6McastScope(otIp6Address  *addr)
{
    if (addr->mFields.m8[0] != 0xff)
        return 0xff;
    return addr->mFields.m8[0] & 0x0f;
}

void platformIp6Receiver(otMessage *aMessage, void *aContext)
{
    OT_UNUSED_VARIABLE(aContext);

    struct MsgToHostEventData evData;
    uint8_t buf[OPENTHREAD_CONFIG_IP6_MAX_DATAGRAM_LENGTH];
    const uint8_t dstAddrZero[OT_IP6_ADDRESS_SIZE] = {0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0};
    size_t msgLen;
    otMessageInfo ip6Info;
    otError error = OT_ERROR_NONE;

    msgLen = otMessageGetLength(aMessage);
    OT_ASSERT(msgLen <= sizeof(buf));

    // parse IPv6 message
    error = platformParseIp6(aMessage, &ip6Info);
    OT_ASSERT(error == OT_ERROR_NONE);

    // determine if IPv6 datagram must go to AIL. This implements simulation-specific BR packet filtering.
    otEXPECT(otMessageIsLoopbackToHostAllowed(aMessage) &&
             !isLinkLocal(&ip6Info.mPeerAddr) &&
             !isLinkLocal(&ip6Info.mSockAddr) &&
             ip6McastScope(&ip6Info.mPeerAddr) >= 0x4);

    // create simulator event
    evData.mSrcPort = ip6Info.mSockPort;
    evData.mDstPort = ip6Info.mPeerPort;
    memcpy(evData.mSrcIp6, &ip6Info.mSockAddr, OT_IP6_ADDRESS_SIZE);
    memcpy(evData.mDstIp6, &ip6Info.mPeerAddr, OT_IP6_ADDRESS_SIZE);
    otMessageRead(aMessage, 0, buf, msgLen);

    otPlatLog(OT_LOG_LEVEL_INFO, OT_LOG_REGION_PLATFORM, "Delivering msg to host for AIL forwarding");
    otSimSendMsgToHostEvent(OT_SIM_EVENT_IP6_TO_HOST, &evData, &buf[0], msgLen);

exit:
    otMessageFree(aMessage);
}

void platformNetifSetUp(otInstance *aInstance)
{
    assert(aInstance != NULL);

    otIp6SetReceiveFilterEnabled(aInstance, true); // FIXME - needed?
    //otIcmp6SetEchoMode(gInstance, OT_ICMP6_ECHO_HANDLER_ALL); // TODO
    //otIcmp6SetEchoMode(gInstance, OT_ICMP6_ECHO_HANDLER_DISABLED);
    otIp6SetReceiveCallback(aInstance, platformIp6Receiver, aInstance);
#if OPENTHREAD_CONFIG_NAT64_TRANSLATOR_ENABLE
    // We can use the same function for IPv6 and translated IPv4 messages.
    // otNat64SetReceiveIp4Callback(gInstance, processReceive, gInstance);
#endif
    //otIp6SetAddressCallback(aInstance, processAddressChange, aInstance);
#if OPENTHREAD_POSIX_MULTICAST_PROMISCUOUS_REQUIRED
    //otIp6SetMulticastPromiscuousEnabled(aInstance, true);
#endif
#if OPENTHREAD_CONFIG_NAT64_TRANSLATOR_ENABLE
    //nat64Init();
#endif
#if OPENTHREAD_CONFIG_DNS_UPSTREAM_QUERY_ENABLE
    //gResolver.Init();
#endif
}
