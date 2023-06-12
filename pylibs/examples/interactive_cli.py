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

from otns.cli import OTNS
from otns.cli.errors import OTNSExitedError


def main():
    # the 'is_interactive' parameter will configure OTNS for interactive use.
    ns = OTNS(is_interactive=True)

    ns.radiomodel = 'MIDisc'
    ns.speed = 0.008
    ns.set_title("Interactive simulation with OTNS CLI example - switch to cmdline and e.g. type 'ping 1 5'")

    # add some nodes and let them form network
    ns.add("router")
    ns.go(10)
    ns.add("router")
    ns.go(10)
    ns.add("router")
    ns.go(10)
    ns.add("router")
    ns.go(10)
    ns.add("router")
    ns.go(10)

    # here we call the CLI for the user to type commands. Now the simulation can be manipulated as wanted,
    # using the CLI or GUI commands. Typing 'exit' will exit this call.
    ns.interactive_cli()

    # after the user exits, more scripted things could be done. But usually the script would also exit.
    ns.speed = 10.0
    ns.add("fed")
    ns.go(60)


if __name__ == '__main__':
    try:
        main()
    except OTNSExitedError as ex:
        if ex.exit_code != 0:
            raise