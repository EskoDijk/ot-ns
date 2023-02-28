// Copyright (c) 2023, The OTNS Authors.
// All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
// 1. Redistributions of source code must retain the above copyright
//    notice, this list of conditions and the following disclaimer.
// 2. Redistributions in binary form must reproduce the above copyright
//    notice, this list of conditions and the following disclaimer in the
//    documentation and/or other materials provided with the distribution.
// 3. Neither the name of the copyright holder nor the
//    names of its contributors may be used to endorse or promote products
//    derived from this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
// AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
// IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
// ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
// LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
// CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
// SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
// INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
// CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
// ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
// POSSIBILITY OF SUCH DAMAGE.

import { Terminal } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import LocalEchoController from './console/LocalEchoController';

const {
    VisualizeRequest, VisualizeEvent, CommandRequest
} = require('./proto/visualize_grpc_pb.js');
const {VisualizeGrpcServiceClient} = require('./proto/visualize_grpc_grpc_web_pb.js');

const cliCommands = [
    "add",
    "coaps",
    "counters",
    "cv",
    "del",
    "energy",
    "exit",
    "go",
    "help",
    "joins",
    "log",
    "move",
    "netinfo",
    "node",
    "nodes",
    "partitions",
    "ping",
    "pings",
    "plr",
    "pts",
    "radio",
    "radiomodel",
    "scan",
    "speed",
    "time",
    "title",
    "unwatch",
    "watch",
    "web",
];

let grpcServiceClient = null;

// create the console Xterm and make it autofit.
const xterm = new Terminal();
const fitAddon = new FitAddon();
xterm.loadAddon(fitAddon);
xterm.open(document.getElementById('console'));
fitAddon.fit();
const webConsole = new LocalEchoController(null, {historySize:100} );
xterm.loadAddon(webConsole);
webConsole.addAutocompleteHandler(autoCompleteHandlerCb);
webConsole.println('OTNS Web Console - type "help" for command list.');

function loadOk() {
    console.log('connecting to server ' + server);
    grpcServiceClient = new VisualizeGrpcServiceClient(server);

    let visualizeRequest = new VisualizeRequest();
    let metadata = {'custom-header-1': 'value1'};
    let stream = grpcServiceClient.visualize(visualizeRequest, metadata);
    stream.on('data', function (resp) {
        let e = null;
        switch (resp.getTypeCase()) {
            case VisualizeEvent.TypeCase.CLI_WRITE:
                e = resp.getCliWrite();
                //term.write(e.getMsg());
                webConsole.print(e.getMsg());
                break;
            default:
                break
        }
    });

    stream.on('status', function (status) {
    });
    stream.on('end', function (end) {
        // stream end signal
    });
}

window.addEventListener("resize", function () {
    fitAddon.fit();
});

function autoCompleteHandlerCb(index, tokens) {
    let res = [];
    const cmd = tokens[index];
    if (index==0 || (index==1 && tokens[0] == 'help') ){
        let cliCommandsLen = cliCommands.length;
        for(let i = 0; i < cliCommandsLen; i++){
            if (cliCommands[i].startsWith(cmd)) {
                res.push(cliCommands[i]);
            }
        }
    }
    return res;
}

function runCommand(cmd, callback) {
    let req = new CommandRequest();
    req.setCommand(cmd);
    console.log(`> ${cmd}`);

    grpcServiceClient.command(req, {}, (err, resp) => {
            if (err !== null) {
                webConsole.println("Error: " + err.toLocaleString());
                console.error("Error: " + err.toLocaleString());
                if (callback) {
                    callback(err, [])
                }
            }

            let output = resp.getOutputList();
            for (let i in output) {
                webConsole.println(output[i]);
                console.log(output[i]);
            }

            if (callback) {
                let errmsg = output.pop();

                if (errmsg !== "Done") {
                    callback(new Error(errmsg), output)
                } else {
                    callback(null, output)
                }
            }
            readNextCommand();
        }
    )
}

function readNextCommand() {
    webConsole.read("> ")
        .then(input => {
            if (input.length > 0) {
                runCommand(input);
            }else {
                readNextCommand();
            }
        })
        .catch(error => {
            console.log(error);
            readNextCommand();
        });
}

loadOk();
readNextCommand();