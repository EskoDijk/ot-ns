#!/usr/bin/env python3
# Copyright (c) 2024, The OTNS Authors.
# All rights reserved.
#
# Redistribution and use in source and binary forms, with or without
# modification, are permitted provided that the following conditions are met:
# 1. Redistributions of source code must retain the above copyright
#    notice, this list of conditions and the following disclaimer.
# 2. Redistributions in binary form must reproduce the above copyright
#    notice, this list of conditions and the following disclaimer in the
#    documentation and/or other materials provided with the distribution.
# 3. Neither the name of the copyright holder nor the
#    names of its contributors may be used to endorse or promote products
#    derived from this software without specific prior written permission.
#
# THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
# AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
# IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
# ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
# LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
# CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
# SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
# INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
# CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
# ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
# POSSIBILITY OF SUCH DAMAGE.

import tracemalloc
import unittest

from OTNSTestCase import OTNSTestCase
from otns.cli import OTNS

tracemalloc.start()


class CommissioningTests(OTNSTestCase):

    def setUp(self) -> None:
        self.ns = OTNS(otns_args=['-ot-script', 'none', '-log', 'debug'])
        self.ns.speed = float('inf')

    def setFirstNodeDataset(self, n1) -> None:
        self.ns.node_cmd(n1, "dataset init new")
        self.ns.node_cmd(n1, "dataset networkkey 00112233445566778899aabbccddeeff") # allow easy Wireshark dissecting
        self.ns.node_cmd(n1, "dataset commit active")

    def testCommissioningOneHop(self):
        ns = self.ns
        ns.web()
        ns.radiomodel = 'MIDisc'

        n1 = ns.add("br", radio_range = 110)
        n2 = ns.add("router", radio_range = 110)

        self.setFirstNodeDataset(n1)
        ns.ifconfig_up(n1)
        ns.thread_start(n1)
        self.go(35)
        self.assertTrue(ns.get_state(n1) == "leader")
        ns.commissioner_start(n1)
        ns.commissioner_ccm_joiner_add(n1, "*")

        ns.ifconfig_up(n2)
        ns.ccm_joiner_start(n2)
        self.go(20)
        ns.thread_start(n2)
        self.go(100)
        c = ns.counters()
        print('counters', c)
        joins = ns.joins()
        print('joins', joins)

        ns.interactive_cli()
        self.assertFormPartitions(1)
        self.assertTrue(joins and joins[0][1] > 0)  # assert join success

if __name__ == '__main__':
    unittest.main()
