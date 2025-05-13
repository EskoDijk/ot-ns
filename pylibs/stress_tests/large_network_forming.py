#!/usr/bin/env python3
#
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
import os
import time

from BaseStressTest import BaseStressTest

XGAP = 80
YGAP = 80
RADIO_RANGE = int(XGAP * 2.5)

LARGE_N = 11
PACKET_LOSS_RATIO = max((int(os.getenv('STRESS_LEVEL', '1')) - 1) / 50.0, 0.0)

SIMULATE_TIME_TOTAL = 600
SIMULATE_TIME_PERIOD = 30
REPEAT = max(int(os.getenv('STRESS_LEVEL', '1')) // 2, 1)


class StressTest(BaseStressTest):
    SUITE = 'network-forming'

    def __init__(self):
        super(StressTest, self).__init__("Large Network Formation Test",
                                         ["Rep", "Simulation Time", "Execution Time", "Partition Count"])

    def run(self):
        self.ns.packet_loss_ratio = PACKET_LOSS_RATIO
        self.ns.loglevel = 'info'
        self.ns.web('stats')

        durations = []
        partition_counts = []
        for nrep in range(1, REPEAT + 1):
            durations, partition_counts = self.test_n(LARGE_N, durations, partition_counts, nrep)

    def test_n(self, n, durations, partition_counts, nrep):
        self.reset()

        for r in range(n):
            for c in range(n):
                id = self.ns.add("router", 50 + XGAP * c, 50 + YGAP * r, radio_range=RADIO_RANGE)
                # self.ns.node_cmd(id, f'childtimeout {5}')  # prefer not to modify default childtimeout

        for _ in range(SIMULATE_TIME_TOTAL // SIMULATE_TIME_PERIOD):
            t0 = time.time()
            self.ns.go(SIMULATE_TIME_PERIOD)
            dt = time.time() - t0

            durations.append(dt)
            par_cnt = len(self.ns.partitions())
            partition_counts.append(par_cnt)
            sim_time = self.ns.time
            self.result.append_row('%d' % nrep, '%ds' % sim_time, '%ds' % sum(durations), '%d' % par_cnt)

        return durations, partition_counts


if __name__ == '__main__':
    StressTest().run()
