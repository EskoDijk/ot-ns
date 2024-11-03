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

# Thread Commercial Commissioning Mode (CCM) tests using cBRSKI. Nodes can onboard into a Thread Domain
# using a localhost server, OT-Registrar, which runs from a Java JAR file. The Registrar
# communicates with a MASA server that creates the Voucher that the node needs to approve the
# onboarding. The Registrar generates the final domain identity (LDevID certificate) for the Joiner to use.
# See IETF ANIMA WG cBRSKI draft for details. https://datatracker.ietf.org/doc/draft-ietf-anima-constrained-voucher/

import logging
import os
import time
import subprocess

from otns.cli import OTNS
from otns.cli.errors import OTNSExitedError

global registrar_log_file

thread_domain_name = "TestCcmDomain"


def setupNs():
    logging.info("Setting up for Thread CCM sandbox")
    ns = OTNS(
        otns_args=['-log', 'debug', '-pcap', 'wpan-tap', '-seed', '4', '-ot-script', './pylibs/case_studies/ccm.yaml'])
    ns.watch_default('debug')
    ns.web()
    ns.set_title('Thread CCM sandbox for IETF 121')
    # configure sim-host server that acts as BRSKI Registrar. TODO update IPv6 addr
    ns.cmd('host add "masa.example.com" "910b::1234" 5684 5684')
    ns.coaps_enable()
    #ns.radiomodel = 'MIDisc'  # enforce strict line topologies for testing
    return ns


def setActiveDataset(ns, n1) -> None:
    ns.node_cmd(n1, "dataset init new")
    ns.node_cmd(n1, "dataset networkname CcmTestNet")
    ns.node_cmd(n1, "dataset channel 22")
    ns.node_cmd(n1, "dataset activetimestamp 456789")
    ns.node_cmd(n1, "dataset panid 0x1234")
    ns.node_cmd(n1, "dataset extpanid 39758ec8144b07fb")
    ns.node_cmd(n1, "dataset pskc 3ca67c969efb0d0c74a4d8ee923b576c")
    ns.node_cmd(n1, "dataset meshlocalprefix fd00:777e:10ca::")
    ns.node_cmd(n1, "dataset networkkey 00112233445566778899aabbccddeeff")  # allow easy Wireshark dissecting
    ns.node_cmd(n1, "dataset securitypolicy 672 orcCR 3")  # enable CCM-commissioning flag in secpolicy
    ns.node_cmd(n1, "dataset commit active")


def startRegistrar(ns):
    logging.debug("starting OT Registrar")
    ns.registrar_log_file = open("tmp/ot-registrar.log", 'w')
    subprocess.Popen([
        'java', '-jar', './etc/ot-registrar/ot-registrar.jar', '-registrar', '-vvv', '-f',
        './etc/ot-registrar/credentials_registrar.p12', '-d', thread_domain_name
    ],
                     stdout=ns.registrar_log_file,
                     stderr=subprocess.STDOUT)


def verifyRegistrarStarted():
    for n in range(1, 20):
        time.sleep(0.5)
        if os.path.isfile("tmp/ot-registrar.log"):
            with open("tmp/ot-registrar.log", 'r') as file:
                rlog = file.read()
                if "Registrar listening (CoAPS)" in rlog:
                    process_list = subprocess.run(["ps", "a"], capture_output=True, text=True).stdout
                    if "ot-registrar.jar" in process_list:
                        return True
    return False


def enrollBr(ns, nid):
    ns.coaps()  # clear coaps

    # BR enrolls via AIL.
    ns.node_cmd(nid, "ipaddr add fd12::5")  # dummy address to allow sending to AIL. TODO resolve in stack.
    ns.go(1)
    ns.joiner_startccmbr(nid)
    ns.go(5)

    coap_events = ns.coaps()  # see emitted CoAP events
    if len(coap_events) != 4:  # messages are /rv, /vs, /sen, /es
        logging.error("BR may not have enrolled correctly.")
        #raise Exception("BR may not have enrolled correctly.")


def setupStartTopology(ns):
    n1 = ns.add("br", version="ccm")
    ns.go(1)
    ns.speed = 3
    enrollBr(ns, n1)
    setActiveDataset(ns, n1)
    ns.ifconfig_up(n1)
    ns.thread_start(n1)
    ns.go(10)

    # n1 starts commissioner on BR
    ns.commissioner_start(n1)
    ns.go(5)
    ns.commissioner_ccm_joiner_add(n1, "*")

    # this changes default node type to 'CCM' node
    ns.cmd('exe ftd "ot-cli-ftd_ccm"')
    ns.cmd('exe mtd "ot-cli-mtd_ccm"')

    # n2 added: not on network yet
    n2 = ns.add("router")
    ns.speed = 1
    ns.autogo = True


def main():
    ns = setupNs()
    if not verifyRegistrarStarted():
        startRegistrar(ns)
        if not verifyRegistrarStarted():
            logging.error("Couldn't start OT-Registrar")
            return
    setupStartTopology(ns)
    ns.interactive_cli()


if __name__ == '__main__':
    try:
        main()
    except OTNSExitedError as ex:
        if ex.exit_code != 0:
            raise
