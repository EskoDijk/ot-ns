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
        for args in (['-output', 'results'], ['--output', 'results'], ['-output=results'],
                     ['--output=results']):
            self.assertEqual('results', OTNS._output_dir_from_args(args), args)

    def test_empty_value_falls_back_to_default(self):
        self.assertEqual('tmp', OTNS._output_dir_from_args(['-output=']))


if __name__ == '__main__':
    unittest.main()
