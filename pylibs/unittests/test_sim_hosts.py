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
#

import aiocoap
import aiocoap.resource as resource
import asyncio
import unittest

from OTNSTestCase import OTNSTestCase
from otns.cli import errors, OTNS


class SimHostsTests(OTNSTestCase):
    def testConfigureSimHosts(self):
        ns = self.ns
        ns.cmd('host add "myserver.example.com" "fc00::1234" 5683 5683')
        with self.assertRaises(errors.OTNSCliError): # too long IPv6 address
            ns.cmd('host add "myserver.example.com" "fd12:1234:5678:abcd:1234:5678:abcd:2020:3030" 5684 65300')
        with self.assertRaises(errors.OTNSCliError): # missing port-mapped
            ns.cmd('host add "myserver.example.com" "fd12:1234:5678:abcd::5678:abcd:2020" 5684')
        ns.cmd('host add "myserver.example.com" "fd12:1234:5678:abcd:1234:5678:abcd:2020" 5684 65300')
        ns.cmd('host add "bad.example.com" "910b::f00d" 3 4')

        hosts_list = ns.cmd('host list')
        self.assertEqual(3+1, len(hosts_list)) # includes one header line

        ns.cmd('host del "myserver.example.com"')
        hosts_list = ns.cmd('host list')
        self.assertEqual(1+1, len(hosts_list))

        ns.cmd('host del "910b::f00d"')
        hosts_list = ns.cmd('host list')
        self.assertEqual(0+1, len(hosts_list))

    def testSendToSimHost(self):
        ns = self.ns
        ns.cmd('host add "myserver.example.com" "fc00::1234" 5683 5683')
        n1=ns.add('br')
        ns.go(10)
        n2=ns.add('router')
        ns.go(10)

        # n2 sends a coap message to AIL, to test AIL connectivity
        ns.node_cmd(n2, "coap start")
        ns.node_cmd(n2, "coap get fc00::1234 info") # dest addr must match an external route of the BR
        self.go(10)

        hosts_list = ns.cmd('host list')
        self.assertEqual(1+1, len(hosts_list))
        self.assertEqual("11        0", hosts_list[1][-11:]) # number of Rx bytes == 11

    def testResponseFromSimHost(self):
        asyncio.run(self.asyncResponseFromSimHost())

    async def asyncResponseFromSimHost(self):
        await coap_server_main()

        ns = self.ns
        ns.cmd('host add "myserver.example.com" "fc00::1234" 5683 5683')
        n1=ns.add('br')
        ns.go(10)
        n2=ns.add('router')
        ns.go(10)

        # n2 sends a coap message to AIL, to test AIL connectivity
        ns.node_cmd(n2, "coap start")
        ns.node_cmd(n2, "coap get fc00::1234 hello") # dest addr must match an external route of the BR
        self.go(10)

        #await asyncio.get_running_loop().create_future()
        await asyncio.sleep(10)

        hosts_list = ns.cmd('host list')
        self.assertEqual(1+1, len(hosts_list))
        self.assertEqual("12       11", hosts_list[1][-11:]) # number of Rx bytes == 11

        #await asynciocoap_server_main()






class HelloResource(resource.Resource):
    async def render_get(self, request):
        return aiocoap.Message(content_format=0, payload="Hello World".encode('utf8'))


async def coap_server_main():
    root = resource.Site()
    root.add_resource(['hello'], HelloResource())
    await aiocoap.Context.create_server_context(root)

if __name__ == '__main__':
    unittest.main()
