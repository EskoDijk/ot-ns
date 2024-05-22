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
 *   This file includes the C++ portions of the OT-RFSIM platform.
 */

#if OPENTHREAD_CONFIG_BORDER_ROUTING_ENABLE

#include "net/ip6.hpp"
#include "net/ip6_address.hpp"

extern "C" otError platformParseIp6( otMessage *aMessage, otMessageInfo *ip6Info);
extern "C" void validateOtMsg( otMessage *aMessage);

#include "platform-rfsim.h"
#include "utils/uart.h"

using namespace ot;

otError platformParseIp6( otMessage *aMessage, otMessageInfo *ip6Info) {
    Ip6::Headers headers;
    otError error = OT_ERROR_PARSE;
    Message msg = AsCoreType(aMessage);

    SuccessOrExit(error = headers.ParseFrom(msg));
    ip6Info->mSockAddr = headers.GetSourceAddress();
    ip6Info->mPeerAddr = headers.GetDestinationAddress();
    ip6Info->mSockPort = headers.GetSourcePort();
    ip6Info->mPeerPort = headers.GetDestinationPort();

exit:
    return error;
}

// FIXME delete
void validateOtMsg( otMessage *aMessage) {
    Message msg = AsCoreType(aMessage);
    msg.RemoveHeader(msg.GetOffset());
    //OT_ASSERT(msg.GetOffset() == 0);
}

#endif // OPENTHREAD_CONFIG_BORDER_ROUTING_ENABLE