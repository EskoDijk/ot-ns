import logging
import unittest
from typing import Dict

from OTNSTestCase import OTNSTestCase
from otns.cli import errors, OTNS

class EskoTests(OTNSTestCase):

    #override
    def setUp(self):
        super().setUp()
        self.ns.radiomodel = 'MutualInterference'

    def testRadioInRange(self):
        ns = self.ns
        radio_range = 100
        ns.add("router", 0, 0, radio_range=radio_range)
        ns.add("router", 0, radio_range - 1, radio_range=radio_range)
        self.go(15)
        self.assertFormPartitions(1)


if __name__ == '__main__':
    unittest.main()
