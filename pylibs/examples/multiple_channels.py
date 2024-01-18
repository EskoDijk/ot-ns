#!/usr/bin/env python3
# Copyright (c) 2023, The OTNS Authors.
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
# Thread Networks on multiple channels example. Each network has its own
# distinct active dataset. The PCAP type 'wpan-tap' is selected, which stores
# metadata for each frame including the 802.15.4 channel.

from otns.cli import OTNS
from otns.cli.errors import OTNSCliError, OTNSExitedError


class MultipleChannelsExample:

    def __init__(self):
        self.ns = OTNS(otns_args=["-raw", "-pcap", "wpan-tap"])
        self.ns.set_title("Multiple Channels Example")
        self.ns.web()

    # executes a startup script on each node, params depending on each group (ngrp)
    def setup_node_for_group(self, nid, ngrp):
        chan = ngrp-1 + 11
        self.ns.set_network_name(nid,f"Netw{ngrp}_Chan{chan}")
        self.ns.set_panid(nid,ngrp)
        self.ns.set_extpanid(nid,ngrp)
        self.ns.set_networkkey(nid, "00112233445566778899aabbccddeef" + str(ngrp)) # each grp own network-key
        self.ns.set_channel(nid,chan)
        self.ns.ifconfig_up(nid)
        self.ns.thread_start(nid)

    def create_topology(self):
        n_netw_group = [2, 3] # number of different network-groups [rows,cols]
        n_node_group = [4, 4] # nodes per Thread Network (i.e. channel) [rows,cols]
        gdx = 500
        gdy = 400
        ndx = 70
        ndy = 70
        ofs_x = 100
        ofs_y = 100
        ng = 1 # number of group (network) a node is in.
        for rg in range(0,n_netw_group[0]):
            for cg in range(0,n_netw_group[1]):
                for rn in range(0,n_node_group[0]):
                    for cn in range(0,n_node_group[1]):
                        nid = self.ns.add('router', x=ofs_x+cg*gdx+cn*ndx, y=ofs_y+rg*gdy+rn*ndy)
                        self.setup_node_for_group(nid, ng)
                ng += 1

    def go(self):
        self.ns.go(200)


def main():
    ex = MultipleChannelsExample()

    ex.ns.loglevel = 'info'
    ex.ns.watch_default('warn') # show errors+warnings from all OT nodes

    ex.create_topology()
    ex.go()


if __name__ == '__main__':
    try:
        main()
    except OTNSExitedError as err:
        if err.exit_code != 0:
            raise
