#!/usr/bin/env python3
# Copyright (c) 2020-2026, The OTNS Authors.
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

import logging
import re
import tracemalloc
import unittest

from pathlib import Path
from typing import Optional, Tuple

from otns.cli import OTNS

# Unit tests always run with a base OTNS instance with a fixed SimId and output path. These must
# stay in sync with the OTNS() arguments used in setUp() below.
OTNS_OUTPUT_PATH = 'tmp'
OTNS_SIM_ID = 0

# Layout of one log line in the OTNS log file: '<timestamp>\t<level>\t<message>'. This must match
# the encoder config and 'dateTimeFormat' in logger/logger.go.
_OTNS_LOG_LINE_RE = re.compile(
    r'\d{4}-\d\d-\d\d \d\d:\d\d:\d\d\.\d{3}\t(debug|info|warn|error|panic|fatal)(?:\t(.*))?')


def parse_otns_log_line(line: str) -> Optional[Tuple[str, str]]:
    """
    Parse one line of the OTNS log file.

    :param line: the line, with or without its line ending.
    :return: (level, message) tuple if the line is a log line, or None for any other line in the
             file: a CLI command or its output, a continuation line of a multi-line log message
             (e.g. a stack trace), or the 'Log level changed' notice.
    """
    m = _OTNS_LOG_LINE_RE.fullmatch(line.rstrip('\r\n'))
    if m is None:
        return None
    return m.group(1), m.group(2) or ''


class OTNSTestCase(unittest.TestCase):

    # After each test, tearDown scans the OTNS log file and fails the test for any unwanted log
    # entry. See assertNoUnwantedLogEntries(). A subclass may rebind the two attributes below. An
    # individual test may assign self.logFailureLevels or self.logFailureSignatures, e.g. to an
    # empty tuple for a test in which OTNS is expected to log such an entry.

    # Levels of the OTNS log entries that are always unwanted.
    logFailureLevels = ('panic', 'fatal')

    # Substrings of the OTNS log messages of level 'warn' or 'error' that are unwanted. Each must
    # match the format string of a log call in the Go code.
    logFailureSignatures = (
        'RfSimEvent with unexpected param',
        'received unexpected (longer) node output',
        'received from unknown Node',
        'gRPC server could not listen',
        'gRPC server quit with error',
        'RecvEvents timeout: alive nodes are',
        'processEvent() detects unknown node',
        'New node did not send a correct init event EventTypeNodeInfo',
        'sent incomplete event data, closing node connection',
        'received unknown status push: ',
        ' received from apparent duplicate Node ',
        'dispatcher handle task failed',
        'Kill existing grpcwebproxy process failed',
        'expectEvent: expected event type ',
    )

    # Max number of unwanted log entries that a test failure message lists.
    LOG_FAILURE_MAX_SHOWN = 20

    @classmethod
    def setUpClass(cls) -> None:
        tracemalloc.start()
        logging.basicConfig(level=logging.DEBUG, format='%(asctime)-15s - %(levelname)s - %(message)s')

    def name(self) -> str:
        return self.id().replace('__main__.', '')

    def setUp(self) -> None:
        logging.info("Setting up for test: %s", self.name())
        # Remove node flash files from earlier tests, so that no node state carries over.
        # Unit tests may be randomly ordered so never test flash-based node restore between
        # tests. Note this cleans the default output directory and sim ID only. See
        # OTNS_OUTPUT_PATH above. A subclass that runs OTNS with a different '-output' or
        # sim ID is responsible for cleaning that directory itself.
        for f in Path(OTNS_OUTPUT_PATH).glob(f'{OTNS_SIM_ID}_*.flash'):
            f.unlink(missing_ok=True)
        # Remove the OTNS log file of an earlier test. OTNS creates it anew, but not if OTNS exits
        # on an error at startup before log file creation: then the old file would mistakenly be used otherwise.
        Path(OTNS_OUTPUT_PATH, f'{OTNS_SIM_ID}_otns.log').unlink(missing_ok=True)
        self.ns = OTNS(otns_args=['-log', 'debug'])  # may add '-watch', 'trace' to see detailed OT node traces.

    def tearDown(self) -> None:
        self.ns.close()
        self.ns.save_pcap(f"{OTNS_OUTPUT_PATH}/unittest_pcap", self.name() + ".pcap")
        self.assertNoUnwantedLogEntries()

    def assertNoUnwantedLogEntries(self) -> None:
        """
        Fail the test if the log file of the OTNS instance self.ns has an unwanted log entry: one of
        a level in self.logFailureLevels, or one of level 'warn' or 'error' with a message that
        contains any of self.logFailureSignatures.

        Call this after self.ns.close(), when OTNS has exited and its log file is complete. OTNS
        creates the log file anew at each start: if a test invokes multiple OTNS instances in a sequence, with the
        same sim ID and output directory, only the log of the last instance can be checked here.
        """
        log_file = Path(self.ns.get_otns_log_file())
        if not log_file.is_file():
            self.fail(f"OTNS log file '{log_file}' not found: can't check it for unwanted log entries. "
                      "OTNS did not start up, or it uses a non-standard output directory or sim ID.")

        unwanted = []
        with log_file.open(errors='replace') as f:
            for line in f:
                entry = parse_otns_log_line(line)
                if entry is None:
                    continue
                level, message = entry
                if level in self.logFailureLevels:
                    unwanted.append(entry)
                elif level in ('warn', 'error') and any(sig in message for sig in self.logFailureSignatures):
                    unwanted.append(entry)

        if unwanted:
            msg = f"OTNS log file '{log_file}' has {len(unwanted)} unwanted log entries:"
            for level, message in unwanted[:self.LOG_FAILURE_MAX_SHOWN]:
                msg += f"\n  [{level}] {message}"
            if len(unwanted) > self.LOG_FAILURE_MAX_SHOWN:
                msg += f"\n  ... and {len(unwanted) - self.LOG_FAILURE_MAX_SHOWN} more"
            self.fail(msg)

    def go(self, duration: float) -> None:
        """
        Run the simulation for a given duration.

        :param duration: the duration to simulate
        """
        self.ns.go(duration)

    def assertFormPartitions(self, count: int):
        pars = self.ns.partitions()
        self.assertEqual(count, len(pars), f"Partitions count mismatch: expected {count}, but is {len(pars)}")
        self.assertTrue(0 not in pars, pars)

    def assertFormPartitionsIgnoreOrphans(self, count: int):
        pars = self.ns.partitions()
        parsNoOrphans = []
        parsNoOrphans[:] = (value for value in pars if value != 0)
        self.assertEqual(count, len(parsNoOrphans),
                         f"Partitions count mismatch: expected {count}, but is {len(parsNoOrphans)}")

    def assertNodeState(self, nodeid: int, state: str):
        cur_state = self.ns.get_state(nodeid)
        self.assertEqual(state, cur_state, f"Node {nodeid} state mismatch: expected {state}, but is {cur_state}")

    def assertPings(self, pings, n, max_delay=1000, max_fails=0):
        self.assertEqual(n, len(pings))
        n_fails = 0
        for srcid, dst, datasize, delay in pings:
            if delay == OTNS.MAX_PING_DELAY:
                n_fails += 1
            else:
                self.assertTrue(delay <= max_delay)
        self.assertTrue(n_fails <= max_fails)

    def assertCoapMessageSent(self,
                              src: int,
                              type: int,
                              code: int,
                              dst_port: int = 5683,
                              number_expected: int = 1,
                              coap_msgs: list = None):
        """
        Verify that a CoAP message is sent from the given source node.
        :param src: node id
        :param type: CoAP type
        :param code: CoAP code
        :param dst_port: destination port number (default 5683)
        :param number_expected: number of messages expected (default 1)
        :param coap_msgs: optional list of CoAP messages from ns.coaps()
        """
        coap_msgs = self.ns.coaps() if coap_msgs == None else coap_msgs
        number_seen = 0
        for c in coap_msgs:
            if c['src'] == src and c['dst_port'] == dst_port and c['code'] == code:
                self.assertEqual(type, c['type'])
                number_seen += 1
        self.assertEqual(
            number_expected, number_seen,
            f"Expected number of CoAP messages is not present: src={src} type={type} code={code} dst_port={dst_port}")
