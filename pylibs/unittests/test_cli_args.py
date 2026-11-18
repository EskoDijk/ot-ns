#!/usr/bin/env python3
#
# Copyright (c) 2026, The OTNS Authors.
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

from OTNSTestCase import parse_otns_log_line
from otns.cli import OTNS


class CliArgsTests(unittest.TestCase):
    """Tests for parsing the otns commandline arguments, without launching a simulation."""

    def parse(self, args, name='output'):
        return OTNS._get_flag_value(args, name)

    def test_absent(self):
        self.assertIsNone(self.parse([]))
        self.assertIsNone(self.parse(['-web=false', '-speed', '100']))

    def test_all_dash_and_value_forms(self):
        # Go's flag package accepts each of these equally.
        self.assertEqual('results', self.parse(['-output', 'results']))
        self.assertEqual('results', self.parse(['--output', 'results']))
        self.assertEqual('results', self.parse(['-output=results']))
        self.assertEqual('results', self.parse(['--output=results']))

    def test_surrounded_by_other_flags(self):
        args = ['-autogo=false', '-web=false', '-speed', '100', '--output', 'run17', '-log', 'warn']
        self.assertEqual('run17', self.parse(args))

    def test_last_occurrence_wins(self):
        self.assertEqual('second', self.parse(['-output', 'first', '-output=second']))

    def test_value_is_not_mistaken_for_a_flag(self):
        # '-output' here is the value of -listen, not a flag of its own.
        self.assertEqual('real', self.parse(['-speed', '100', '--output=real']))

    def test_other_flag_with_matching_prefix(self):
        self.assertIsNone(self.parse(['-output-format=json']))
        self.assertIsNone(self.parse(['-no-output', 'x']))

    def test_double_dash_ends_flags(self):
        self.assertIsNone(self.parse(['--', '-output', 'results']))
        self.assertEqual('before', self.parse(['-output=before', '--', '-output', 'after']))

    def test_missing_value_is_ignored(self):
        # otns itself reports the error; the Python side must not crash or index past the end.
        self.assertIsNone(self.parse(['-web=false', '-output']))

    def test_empty_value(self):
        self.assertEqual('', self.parse(['-output=']))


class OutputDirTests(unittest.TestCase):
    """Tests for the output directory that the Python side tracks for otns."""

    def test_default_when_not_given(self):
        # Must match the Go-side default, or save_pcap()/kpi_save() look in the wrong place.
        self.assertEqual('tmp', OTNS.DEFAULT_OUTPUT_DIR)
        self.assertEqual('tmp', OTNS._output_dir_from_args(['-autogo=false', '-web=false']))

    def test_override_in_every_form(self):
        for args in (['-output', 'results'], ['--output', 'results'], ['-output=results'], ['--output=results']):
            self.assertEqual('results', OTNS._output_dir_from_args(args), args)

    def test_empty_value_falls_back_to_default(self):
        self.assertEqual('tmp', OTNS._output_dir_from_args(['-output=']))


class LogLineTests(unittest.TestCase):
    """Tests for parsing the lines of the otns log file, which OTNSTestCase can check after each unit test."""

    def parse(self, line):
        return parse_otns_log_line(line)

    def test_all_levels(self):
        # Layout as written by logger/logger.go: timestamp, level and message, tab-separated.
        for level in ('debug', 'info', 'warn', 'error', 'panic', 'fatal'):
            line = f'2026-11-16 18:06:11.019\t{level}\tdispatcher started\n'
            self.assertEqual((level, 'dispatcher started'), self.parse(line), line)

    def test_node_log_line(self):
        line = '2026-11-16 18:06:11.019\terror\tNode<4>     15670937 [C] StdErr--------: assert failed at a.cpp:1\n'
        self.assertEqual(('error', 'Node<4>     15670937 [C] StdErr--------: assert failed at a.cpp:1'),
                         self.parse(line))

    def test_line_endings(self):
        for ending in ('', '\n', '\r\n'):
            self.assertEqual(('warn', 'msg'), self.parse('2026-11-16 18:06:11.019\twarn\tmsg' + ending))

    def test_message_keeps_its_own_whitespace(self):
        self.assertEqual(('info', 'a\tb  c '), self.parse('2026-11-16 18:06:11.019\tinfo\ta\tb  c \n'))

    def test_empty_message(self):
        self.assertEqual(('warn', ''), self.parse('2026-11-16 18:06:11.019\twarn\t\n'))
        self.assertEqual(('warn', ''), self.parse('2026-11-16 18:06:11.019\twarn\n'))

    def test_other_lines_are_not_log_entries(self):
        for line in (
                '',
                '\n',
                '#\n',  # log file header
                '# OTNS log for sim-ID 0 created 2026-11-17T18:27:49+01:00\n',
                '2026-11-16 18:06:11.019         Log level changed to debug\n',  # unformatted notice
                '> !add router x 100 y 100\n',  # CLI command
                'Done\n',  # CLI command output
                'Error: node 5 not found\n',
                'goroutine 1 [running]:\n',  # continuation line of a stack trace
                '\tmain.go:12 +0x1d\n',
                'error\t2026-11-16 18:06:11.019\tlevel first\n',
                '2026-11-16 18:06:11.019\tnote\tunknown level\n',
                '2026-11-16 18:06:11.019\tWARN\tuppercase level\n',
                '2026-11-16 18:06:11.019 warn spaces instead of tabs\n',
                'prefix 2026-11-16 18:06:11.019\twarn\tnot at line start\n',
        ):
            self.assertIsNone(self.parse(line), repr(line))


if __name__ == '__main__':
    unittest.main()
