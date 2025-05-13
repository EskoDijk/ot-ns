#!/usr/bin/env python3
# Copyright (c) 2023-2024, The OTNS Authors.
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
#
import unittest

from OTNSTestCase import OTNSTestCase
from otns.cli import errors, OTNS


class ExeVersionTests(OTNSTestCase):
    """
    Unit tests of the 'exe' command to select OT node executables; running nodes with manually specified
    executable; and the 'vxx' parameters for adding nodes of different (prebuilt) OT versions.
    """

    def testAddNodeWrongExecutable(self):
        ns: OTNS = self.ns
        ns.add('router')
        ns.go(5)
        self.assertEqual(1, len(ns.nodes()))
        with self.assertRaises(errors.OTNSCliError):
            ns.add('router', executable='ot-cli-nonexistent')
        ns.go(5)
        self.assertEqual(1, len(ns.nodes()))
        self.assertEqual(1, len(ns.partitions()))

    def testExe(self):
        ns: OTNS = self.ns
        nid = ns.add('router')
        self.assertTrue(ns.get_thread_version(nid) >= 4)
        ns.go(10)
        ns.cmd("exe ftd \"./path/to/non-existent-executable\"")
        with self.assertRaises(errors.OTNSCliError):
            ns.add('router')
        ns.cmd("exe")
        ns.cmd("exe default")
        ns.go(10)
        nid = ns.add('router')
        self.assertTrue(ns.get_thread_version(nid) >= 4)
        ns.go(30)
        nid = ns.add('fed')
        self.assertTrue(ns.get_thread_version(nid) >= 4)
        self.assertEqual(3, len(ns.nodes()))
        ns.go(140)
        self.assertEqual(3, len(ns.nodes()))
        self.assertEqual(1, len(ns.partitions()))

        ns.cmd("exe v11")
        nid = ns.add('router')
        ns.go(10)
        self.assertEqual(2, ns.get_thread_version(nid))

        ns.cmd("exe v12")
        nid = ns.add('router')
        ns.go(10)
        self.assertEqual(3, ns.get_thread_version(nid))

        ns.cmd("exe v13")
        nid = ns.add('router')
        self.assertEqual(4, ns.get_thread_version(nid))
        ns.go(10)

        ns.cmd("exe v14")
        nid = ns.add('router')
        self.assertEqual(5, ns.get_thread_version(nid))
        ns.go(10)
        self.assertEqual(7, len(ns.nodes()))
        ns.go(60)
        self.assertEqual(7, len(ns.nodes()))
        self.assertEqual(1, len(ns.partitions()))

    def testAddVersionNodes(self):
        ns: OTNS = self.ns
        ns.add('router', x=250, y=250)
        ns.go(10)
        nid = ns.add('router', version='v14')
        self.assertEqual(5, ns.get_thread_version(nid))
        ns.go(10)
        nid = ns.add('router', version='v13')
        self.assertEqual(4, ns.get_thread_version(nid))
        ns.go(10)
        nid = ns.add('router', version='v12')
        self.assertEqual(3, ns.get_thread_version(nid))
        ns.go(10)
        nid = ns.add('router', version='v11')
        self.assertEqual(2, ns.get_thread_version(nid))
        ns.go(10)
        self.assertEqual(5, len(ns.nodes()))
        self.assertEqual(1, len(ns.partitions()))

    def testSsedVersions(self):
        ns = self.ns

        ns.add("router", 100, 100)
        ns.go(10)
        ns.add("ssed", version="v12")
        ns.add("ssed", version="v13")
        ns.add("ssed", version="v14")
        ns.add("ssed")
        ns.go(10)
        self.assertFormPartitions(1)

        # SSED pings parent
        for n in range(2, 6):
            ns.ping(n, 1, datasize=n + 10)
            ns.go(5)
        self.assertPings(ns.pings(), 4, max_delay=3000, max_fails=1)

        # parent pings SSED
        for n in range(2, 6):
            ns.ping(1, n, datasize=n + 10)
            ns.go(5)
        self.assertPings(ns.pings(), 4, max_delay=3000, max_fails=1)

    def testWifiInterferers(self):
        ns: OTNS = self.ns
        ns.add('router')
        ns.add('router')
        ns.add('router')
        ns.add('wifi')
        ns.go(50)
        # the wifi node stays on partition 0 (Thread is disabled)
        self.assertEqual(2, len(ns.partitions()))


if __name__ == '__main__':
    unittest.main()
