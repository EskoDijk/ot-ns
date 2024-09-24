#!/usr/bin/env python3
# Copyright (c) 2020-2023, The OTNS Authors.
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

import logging
import unittest
from typing import Dict

from OTNSTestCase import OTNSTestCase
from otns.cli import errors, OTNS


class BasicTests(OTNSTestCase):
    def testGetSetSpeed(self):
        ns = self.ns
        self.assertEqual(ns.speed, OTNS.DEFAULT_SIMULATE_SPEED)
        ns.speed = 2
        self.assertEqual(ns.speed, 2)
        ns.speed = float('inf')
        self.assertEqual(ns.speed, OTNS.MAX_SIMULATE_SPEED)

    def testGetSetMDR(self):
        ns = self.ns
        assert ns.packet_loss_ratio == 0
        ns.packet_loss_ratio = 0.5
        assert ns.packet_loss_ratio == 0.5
        ns.packet_loss_ratio = 1
        assert ns.packet_loss_ratio == 1
        ns.packet_loss_ratio = 2
        assert ns.packet_loss_ratio == 1

    def testOneNodex100(self):
        for i in range(100):
            logging.info("testOneNodex100 round %d", i + 1)
            ns = self.ns
            ns.add("router")
            ns.go(10)
            self.assertFormPartitions(1)
            self.tearDown()
            self.setUp()

    def testAddNode(self):
        ns = self.ns
        ns.add("router")
        self.go(10)
        self.assertFormPartitions(1)

        ns.add("router")
        ns.add("fed")
        ns.add("med")
        ns.add("sed")
        self.go(33)
        self.assertFormPartitions(1)

    def testAddNodeWithID(self):
        ns = self.ns
        for new_id in [50, 55, 60]:
            nid = ns.add("router", id=new_id)
            self.assertEqual(nid, new_id)
            self.go(1)
        self.go(130)
        self.assertFormPartitions(1)

    def testAddNodeWithExistingID(self):
        ns = self.ns
        new_id = 50
        nid = ns.add("router", id=new_id)
        self.assertEqual(nid, new_id)
        self.go(1)
        self.assertRaises(errors.OTNSCliError, lambda: ns.add("router", id=new_id))

    def testRestoreNode(self):
        ns = self.ns
        ns.add("router", x=0, y=0)

        self.go(10)
        self.assertEqual(ns.get_state(1), "leader")

        n=0
        for type in ("router", "fed", "med", "sed"):
            nodeid = ns.add(type, x=n*10, y=10)
            self.go(10)
            self.assertFormPartitions(1)
            rloc16 = ns.get_rloc16(nodeid)
            print('rloc16', rloc16)

            ns.delete(nodeid)
            ns.go(10)

            self.assertEqual(nodeid, ns.add(type, x=n*10, y=10, restore=True))
            self.assertEqual(rloc16, ns.get_rloc16(nodeid))
            self.go(0.1)
            while len(ns.partitions()) > 1 and ns.time < 100e6:
                self.go(0.1)
            self.assertFormPartitions(1)
            n += 1

    def testDelNode(self):
        ns = self.ns
        ns.add("router")
        ns.add("router")
        self.go(25)
        self.assertFormPartitions(1)
        ns.delete(1)
        self.go(10)
        self.assertTrue(len(ns.nodes()) == 1 and 1 not in ns.nodes())

    def testDelManyNodes(self):
        for j in range(4):
            ns = self.ns
            many = 32

            for i in range(many):
                ns.add("router", x=(i % 6) * 100, y=(i // 6) * 150)

            ns.go(10)
            for i in range(1, many + 1):
                ns.delete(i)
                ns.go(5)

            self.assertTrue(ns.nodes() == {})
            self.tearDown()
            self.setUp()

    def testDelNodeAndImmediatelyRecreate(self):
        # repeat multiple times to catch some goroutine race conditions that only happen sometimes.
        for i in range(100):
            logging.info("testDelNodeAndImmediatelyRecreate round %d", i + 1)

            ns = self.ns
            ns.loglevel = 'debug'
            ns.watch_default('debug') # add extra detail in all node's logs
            id = ns.add("router")
            self.assertTrue(len(ns.nodes()) == 1 and 1 in ns.nodes() and id == 1)
            self.go(i/100)
            self.assertTrue(len(ns.nodes()) == 1 and 1 in ns.nodes())

            ns.delete(1)
            self.assertTrue(len(ns.nodes()) == 0)
            id = ns.add("router")
            self.assertTrue(len(ns.nodes()) == 1 and 1 in ns.nodes() and id == 1)

            ns.add("router")
            ns.add("router")
            id = ns.add("router")
            self.assertTrue(len(ns.nodes()) == 4 and id == 4)

            ns.delete(1, 2, 3, 4)
            self.assertTrue(len(ns.nodes()) == 0)
            if i>90:
                ns.go(0)

            ns.add("router")
            id = ns.add("router")
            self.assertTrue(len(ns.nodes()) == 2 and id == 2)

            self.tearDown()
            self.setUp()

    def testDelNonExistingNodes(self):
        ns = self.ns
        ns.add("router")
        ns.add("router")
        self.go(25)
        self.assertFormPartitions(1)
        ns.delete(1,3,4,5)
        self.go(10)
        self.assertTrue(len(ns.nodes()) == 1 and 1 not in ns.nodes())

    def testMDREffective(self):
        ns = self.ns
        ns.packet_loss_ratio = 1
        self.assertTrue(ns.packet_loss_ratio, 1)
        ns.add("router")
        ns.add("router")
        ns.add("router")
        self.go(100)
        self.assertFormPartitions(3)

    def testRadioInRange(self):
        ns = self.ns
        ns.radiomodel = 'Ideal'
        radio_range = 100
        ns.add("router", 0, 0, radio_range=radio_range)
        ns.add("router", 0, radio_range - 1, radio_range=radio_range)
        self.go(15)
        self.assertFormPartitions(1)

    def testRadioNotInRange(self):
        ns = self.ns
        ns.radiomodel = 'Ideal'
        radio_range = 100
        ns.add("router", 0, 0, radio_range=radio_range)
        ns.add("router", 0, radio_range + 1, radio_range=radio_range)
        self.go(10)
        self.assertFormPartitions(2)

    def testNodeFailRecover(self):
        ns = self.ns
        ns.add("router")
        fid = ns.add("router")
        self.go(20)
        self.assertFormPartitions(1)

        ns.radio_off(fid)
        self.go(240)
        print(ns.partitions())
        self.assertFormPartitions(2)

        ns.radio_on(fid)
        self.go(100)
        self.assertFormPartitions(1)

    def testFailTime(self):
        ns = self.ns
        id = ns.add("router")
        ns.radio_set_fail_time(id, fail_time=(2, 10))
        total_count = 0
        failed_count = 0
        for i in range(1000):
            ns.go(1)
            nodes = ns.nodes()
            failed = nodes[id]['failed']
            total_count += 1
            failed_count += failed

        self.assertAlmostEqual(failed_count / total_count, 0.2, delta=0.1)

        # fail-interval param must be greater than fail-duration.
        with self.assertRaises(errors.OTNSCliError):
            ns.radio_set_fail_time(id, fail_time=(18,16))

    def testCliCmd(self):
        ns = self.ns
        id = ns.add("router")
        self.go(10)
        self.assertTrue(ns.get_state(id), 'leader')

    def testCounters(self):
        ns = self.ns

        def assert_increasing(c0: Dict[str, int], c1: Dict[str, int]):
            for k0, v0 in c0.items():
                self.assertGreaterEqual(c1.get(k0, 0), v0)
            for k1, v1 in c1.items():
                self.assertGreaterEqual(v1, c0.get(k1, 0))

        c0 = counters = ns.counters()
        self.assertTrue(counters)
        self.assertTrue(all(x == 0 for x in counters.values()))
        ns.add("router")
        ns.add("router")
        ns.add("router")

        self.go(10)
        c10 = ns.counters()
        assert_increasing(c0, c10)

        self.go(10)
        c20 = ns.counters()
        assert_increasing(c10, c20)

    def testConfigVisualization(self):
        ns = self.ns
        vopts = ns.config_visualization()
        print('vopts', vopts)
        for opt in ('broadcast_message', 'unicast_message', 'ack_message', 'router_table', 'child_table'):
            self.assertTrue(opt in vopts)

            set_vals = (False, True) if vopts[opt] else (True, False)
            for v in set_vals:
                vopts[opt] = v
                self.assertTrue(ns.config_visualization(**{opt: v}) == vopts)

        vopts = ns.config_visualization(broadcast_message=True, unicast_message=True, ack_message=True,
                                        router_table=True,
                                        child_table=True)

        for opt in ('broadcast_message', 'unicast_message', 'ack_message', 'router_table', 'child_table'):
            self.assertTrue(vopts[opt])

        vopts = ns.config_visualization(broadcast_message=False, unicast_message=False, ack_message=False,
                                        router_table=False,
                                        child_table=False)

        for opt in ('broadcast_message', 'unicast_message', 'ack_message', 'router_table', 'child_table'):
            self.assertFalse(vopts[opt])

    def testWithOTNS(self):
        """
        make sure OTNS works in with-statement
        """
        self.tearDown()

        with OTNS(otns_args=['-log', 'debug']) as ns:
            self.assertEqual(OTNS.DEFAULT_SIMULATE_SPEED, ns.speed)
            ns.speed = 19999
            nid = ns.add("router")
            self.assertEqual(1, nid)
            ns.go(10)
            self.assertEqual(10e6,ns.time)

        # run a second time to make sure the previous simulation is properly terminated
        with OTNS(otns_args=['-log', 'warn', '-speed', '18123']) as ns:
            self.assertEqual(18123, ns.speed)
            nid = ns.add("router")
            self.assertEqual(1, nid)
            ns.go(10)
            self.assertEqual(10e6,ns.time)

        with OTNS() as ns:
            ns.add('router')
            ns.add('router')
            self.assertEqual(OTNS.DEFAULT_SIMULATE_SPEED, ns.speed)
            self.assertEqual(0,ns.time)

    def testSetRouterUpgradeThreshold(self):
        ns: OTNS = self.ns
        nid = ns.add("router")
        self.assertEqual(16, ns.get_router_upgrade_threshold(nid))
        for val in range(0, 33):
            ns.set_router_upgrade_threshold(nid, val)
            self.assertEqual(val, ns.get_router_upgrade_threshold(nid))

    def testSetRouterUpgradeThresholdEffective(self):
        ns: OTNS = self.ns
        nid = ns.add("router")
        ns.go(10)
        self.assertNodeState(nid, 'leader')

        reed = ns.add("router")
        ns.set_router_upgrade_threshold(reed, 1)
        ns.go(130)
        self.assertNodeState(reed, 'child')

        ns.set_router_upgrade_threshold(reed, 2)
        ns.go(150)
        self.assertNodeState(reed, 'router')

    def testSetRouterDowngradeThreshold(self):
        ns: OTNS = self.ns
        nid = ns.add("router")
        self.assertEqual(23, ns.get_router_downgrade_threshold(nid))
        for val in range(0, 33):
            ns.set_router_downgrade_threshold(nid, val)
            self.assertEqual(val, ns.get_router_downgrade_threshold(nid))

    def testCoaps(self):
        ns: OTNS = self.ns
        ns.coaps_enable()
        for i in range(10):
            id = ns.add('router', x=i*10, y=0)
            ns.node_cmd(id, 'routerselectionjitter 1')
            ns.go(5)

        ns.go(10)
        msgs = ns.coaps()
        routers = {}
        for msg in msgs:
            if msg.get('uri') == 'a/as':
                routers[msg['src']] = msg['id']

        # Node 2 ~ 10 should become Routers by sending `a/as`
        self.assertEqual(set(routers), set(range(2, 11)))

    def testMultiRadioChannel(self):
        ns = self.ns
        radio_range = 100
        ns.add("router", 0, 0, radio_range=radio_range)
        ns.add("router", 0, 50, radio_range=radio_range)
        ns.add("router", 50, 0, radio_range=radio_range)
        ns.add("router", 50, 50, radio_range=radio_range)
        self.go(130)
        self.assertFormPartitions(1)

        for n in [1,2]:
            ns.node_cmd(n, "ifconfig down")
            ns.node_cmd(n, "channel 20")
            ns.node_cmd(n, "ifconfig up")
            ns.node_cmd(n, "thread start")
        self.go(300)
        self.assertFormPartitions(2)

    def testLoglevel(self):
        ns: OTNS = self.ns
        ns.loglevel = "warn"
        id = ns.add("router")
        self.go(10)
        self.assertEqual(ns.loglevel, "warn")
        ns.loglevel = "debug"
        id = ns.add("router")
        self.go(10)
        self.assertEqual(ns.loglevel, "debug")
        with self.assertRaises(errors.OTNSCliError):
            ns.loglevel = "invalid_log_level"
        self.assertEqual(ns.loglevel, "debug")
        ns.loglevel = "info"
        ns.loglevel = "error"

    def testWatch(self):
        ns: OTNS = self.ns
        for i in range(10):
            ns.add('router')
            ns.go(2)
        ns.watch(3, 4, 5, 6, 8)
        ns.go(5)
        ns.unwatch(5, 6)
        self.assertEqual([3, 4, 8], ns.watched())
        ns.unwatch_all()
        self.assertEqual([], ns.watched())
        ns.watch_all('debug')
        self.assertEqual([1,2,3,4,5,6,7,8,9,10], ns.watched())
        ns.watch_all('warn')
        self.assertEqual([1,2,3,4,5,6,7,8,9,10], ns.watched())

    def testWatchDefault(self):
        ns: OTNS = self.ns
        ns.add('router')
        ns.watch_default('trace')
        for i in range(9):
            ns.add('router')
            ns.go(2)
        self.assertEqual([2,3,4,5,6,7,8,9,10], ns.watched())
        ns.watch_default('off')
        ns.add('router')
        self.assertEqual([2,3,4,5,6,7,8,9,10], ns.watched())
        ns.watch_default('info')
        ns.add('router')
        self.assertEqual([2,3,4,5,6,7,8,9,10, 12], ns.watched())

    def testWatchNonExistingNodes(self):
        ns: OTNS = self.ns
        for i in range(10):
            ns.add('router')
            ns.go(2)
        with self.assertRaises(errors.OTNSCliError):
            ns.watch(3, 4, 11) # node 11 does not exist
        ns.go(5)
        self.assertEqual([3, 4], ns.watched())
        ns.unwatch(5, 6) # nodes not being watched, but no error.
        self.assertEqual([3, 4], ns.watched())
        ns.unwatch(3, 5, 66) # 3 is watched but 5 not, 66 non-existing, but no error.
        self.assertEqual([4], ns.watched())

    def testHelp(self):
        ns: OTNS = self.ns
        ns._do_command("help")
        ns._do_command("help plr")
        ns._do_command("help radiomodel")
        ns._do_command("help add")
        ns._do_command("help exe")

    def testGoUnits(self):
        ns: OTNS = self.ns
        ns.add('router')
        ns.add('router')

        ns.go(10)
        self.assertEqual(10e6, ns.time) # ns.time returns microseconds
        ns.go(0.001)
        self.assertEqual(10001e3, ns.time)
        ns.go(1e-3)
        self.assertEqual(10002e3, ns.time)
        ns.go(1e-6)
        self.assertEqual(10002001, ns.time)
        ns.go(3e-5)
        self.assertEqual(10002031, ns.time)
        ns.go(1e-7)
        self.assertEqual(10002031, ns.time) # no time advance: rounded to nearest microsecond.
        ns.go(0.000999) # almost 1 ms
        self.assertEqual(10003030, ns.time)
        ns.go(4.0000004)
        self.assertEqual(14003030, ns.time) # rounded to nearest microsecond.

    def testScan(self):
        self.tearDown()
        with OTNS(otns_args=['-log', 'trace', '-autogo=true']) as ns:
            ns.radiomodel = 'MutualInterference'
            ns.add('router')

            with self.assertRaises(errors.OTNSCliError):
                ns._do_command("scan 2")

            ns.add('router')
            ns.add('router')
            ns.add('router', x=100, y=200)
            ns.add('router')
            ns.add('router')

            ns.go(50)
            ns._do_command("scan 1")
            ns.go(5)
            ns._do_command("scan 2")
            ns.go(1)
            ns._do_command("scan 3")
            ns.go(15)
            ns.speed = 1
            ns._do_command("scan 6")
            # no go() period at the end to test starting scan and immediately stopping simulation.

    def testInvalidNodeCmd(self):
        ns: OTNS = self.ns
        with self.assertRaises(errors.OTNSCliError):
            ns.node_cmd(1,'state')
        ns.add('router')
        ns.node_cmd(1,'state')
        with self.assertRaises(errors.OTNSCliError):
            ns.node_cmd(1,'sdfklsjflksj')
        with self.assertRaises(errors.OTNSCliError):
            ns.node_cmd(1,'dns config nonexistoption')
        ns.node_cmd(1,'dns config 2001::1234 1234 5000 2 0 srv_txt_sep udp')

        ns.node_cmd(1,'dns resolve nonexistent.example.com')
        ns.go(11) # response comes during the go period.

        with self.assertRaises(errors.OTNSCliError):
            ns.node_cmd(1,'dns resolvea b c d e f')

    def testExitCmd(self):
        ns: OTNS = self.ns
        # Tested here that the exit command itself does not raise errors.OTNSExitedError.
        # That error is only raised for unexpected exits of OTNS, or when performing an
        # action while OTNS has already exited.
        ns._do_command('exit')

        with self.assertRaises(errors.OTNSExitedError):
            ns.go(1)
        with self.assertRaises(errors.OTNSExitedError):
            ns.add('router')
        with self.assertRaises(errors.OTNSExitedError):
            ns._do_command('exit')


if __name__ == '__main__':
    unittest.main()
