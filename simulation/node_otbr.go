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

package simulation

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/simonlingoogle/go-simplelogger"

	. "github.com/openthread/ot-ns/types"
)

type OtbrNode struct {
	Node
}

func newNcpNode(s *Simulation, nodeid NodeId, cfg NodeConfig) (*OtbrNode, error) {
	simplelogger.AssertTrue(cfg.IsBorderRouter && cfg.IsNcp)
	var err error

	if cfg.Restore {
		return nil, errors.New("cfg.Restore == true not implemented for OTBR NCP")
	}

	logFileName := fmt.Sprintf("%s/%d_%d.ncp.log", GetTmpDir(), s.cfg.Id, nodeid)
	if err = os.RemoveAll(logFileName); err != nil {
		return nil, fmt.Errorf("remove node log file failed: %w", err)
	}

	simplelogger.Debugf("newNode() exe path: %s", cfg.ExecutablePath)
	ptyPath := getPtyFilePath(s.cfg.Id, nodeid)
	cmd := exec.CommandContext(context.Background(), cfg.ExecutablePath, strconv.Itoa(nodeid), s.d.GetUnixSocketName(),
		strconv.Itoa(s.cfg.Id), ptyPath)

	node := &OtbrNode{
		Node{
			S:            s,
			Id:           nodeid,
			cfg:          &cfg,
			cmd:          cmd,
			pendingLines: make(chan string, 10000),
			errors:       make(chan error, 100),
		},
	}

	if node.pipeStdIn, err = cmd.StdinPipe(); err != nil {
		return nil, err
	}

	if node.pipeStdOut, err = cmd.StdoutPipe(); err != nil {
		return nil, err
	}

	if node.pipeStdErr, err = cmd.StderrPipe(); err != nil {
		return nil, err
	}

	// open log file for node's OT output
	node.logFile, err = os.OpenFile(logFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
	if err != nil {
		return nil, fmt.Errorf("create node log file failed: %w", err)
	}
	simplelogger.Debugf("Node log file '%s' created.", logFileName)

	go node.processMonitor()

	node.uartType = NodeUartTypeRealTime
	go node.lineReader(node.pipeStdOut)
	go node.lineReaderStdErr(node.pipeStdErr, false) // on NCP, StdErr output is not fatal.
	if err = node.StartNcp(); err != nil {
		_ = node.logFile.Close()
		return nil, err
	}

	return node, err
}

func (node *OtbrNode) StartNcp() error {
	simplelogger.AssertTrue(node.cfg.IsNcp)

	if err := node.cmd.Start(); err != nil {
		return err
	}

	return nil
}

func (node *OtbrNode) Exit() error {
	simplelogger.AssertTrue(node.cfg.IsNcp)
	return nil
}

// ptyReader reads data from a PTY device and sends it directly to the UART of the node.
func (node *Node) ptyReader(reader io.Reader) {
	buf := make([]byte, 1024) // FIXME size

loop:
	for reader != nil {
		n, err := reader.Read(buf)

		if n > 0 {
			node.S.Dispatcher().SendToUART(node.Id, buf[0:n])
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				simplelogger.Debugf("%v - ptyReader was closed.", node)
				break loop
			}
			simplelogger.Errorf("%v - ptyReader error: %v", node, err)
			break loop
		}
	}
	node.onProcessStops()
}

// ptyPiper pipes data from node's ptyChan (where virtual UART dat comes in) to node's associated
// PTY (for handling by an OT NCP)
func (node *Node) ptyPiper(ptyPath string) {
loop:
	for {
		// check when the PTY device becomes available.
		_, err := os.Stat(ptyPath)
		if node.ptyFile == nil && err == nil {
			node.ptyFile, err = os.OpenFile(ptyPath, os.O_RDWR, 0)
			if err != nil {
				err = fmt.Errorf("%v - ptyPiper couldn't open ptyFile: %v", node, err)
				node.errors <- err
				return
			}
			break loop
		}

		select {
		case <-node.S.ctx.Done():
			break loop
		default:
			time.Sleep(20 * time.Millisecond) // TODO
		}
	}

	go node.ptyReader(node.ptyFile)

loop2:
	for {
		select {
		case b := <-node.ptyChan:
			_, err := node.ptyFile.Write(b)
			if err != nil {
				if errors.Is(err, io.EOF) {
					simplelogger.Debugf("%v - ptyPiper output was closed.", node)
					break loop2
				}
				err = fmt.Errorf("%v - ptyPiper error: %v", node, err)
				simplelogger.Debugf("%v", err)
				node.errors <- err
				break loop2
			}
		case <-node.S.ctx.Done():
			break loop2
		}
	}

	node.onProcessStops()
}
