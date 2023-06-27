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
	"sync"
	"syscall"
	"time"

	"bufio"
	"github.com/openthread/ot-ns/dispatcher"
	"github.com/openthread/ot-ns/otoutfilter"
	. "github.com/openthread/ot-ns/types"
	"github.com/pkg/errors"
	"github.com/simonlingoogle/go-simplelogger"
	"strings"
)

type OtbrNode struct {
	Node

	dockerContainerName string
	httpPort            int

	waitGroup sync.WaitGroup // waits on the external cmd processes started by this node.
	cmdSocat  *exec.Cmd
	cmdDocker *exec.Cmd
	loggerNcp chan string
}

func newNcpNode(s *Simulation, nodeid NodeId, cfg NodeConfig) (*OtbrNode, error) {
	simplelogger.AssertTrue(cfg.IsBorderRouter && cfg.IsNcp)
	var err error

	if cfg.Restore {
		return nil, errors.New("cfg.Restore == true not implemented for OTBR NCP")
	}

	simplelogger.Debugf("newNcpNode() NCP CLI exe path: %s", cfg.ExecutablePath)
	ptyPath := getPtyFilePath(s.cfg.Id, nodeid)
	cmd := exec.CommandContext(context.Background(), cfg.ExecutablePath, strconv.Itoa(nodeid), s.d.GetUnixSocketName(),
		strconv.Itoa(s.cfg.Id), ptyPath)

	node := &OtbrNode{
		Node: Node{
			S:               s,
			Id:              nodeid,
			cfg:             &cfg,
			cmd:             cmd,
			pendingCliLines: make(chan string, 10000),
			logger:          make(chan string, 10),
			uartType:        NodeUartTypeRealTime,
			ptyPath:         ptyPath,
		},
		loggerNcp:           make(chan string, 10),
		dockerContainerName: fmt.Sprintf("otbr_%d_%d", s.cfg.Id, nodeid),
		httpPort:            8080 + nodeid,
	}

	// pipes for the NCP CLI only
	if node.pipeStdIn, err = cmd.StdinPipe(); err != nil {
		return nil, err
	}
	if node.pipeStdOut, err = cmd.StdoutPipe(); err != nil {
		return nil, err
	}
	if node.pipeStdErr, err = cmd.StderrPipe(); err != nil {
		return nil, err
	}

	err = node.runNcpProcesses()

	return node, err
}

// runNcpProcesses runs all the OTBR NCP processes and configures input/output for these.
func (node *OtbrNode) runNcpProcesses() error {
	simplelogger.AssertTrue(node.cfg.IsNcp)
	var pipeStdOut io.Reader
	var pipeStdErr io.Reader
	var err error

	// start loggers
	go node.processLogger(1, node.logger)
	go node.processLogger(2, node.loggerNcp)
	node.logger <- fmt.Sprintf("[D] %v ot-ctl CLI logfile created", node)
	node.loggerNcp <- fmt.Sprintf("[D] %v NCP logfile created", node)
	simplelogger.Debugf("Node CLI log file '%s' created.", node.getLogfileName(1))
	simplelogger.Debugf("Node NCP log file '%s' created.", node.getLogfileName(2))

	// start reader processes
	go node.processCliReader(node.pipeStdOut)
	go node.processErrorReader(node.pipeStdErr, node.logger)

	// start socat
	// socat -d pty,raw,echo=0,link=$PTY_FILE pty,raw,echo=0,link=$PTY_FILE2 >> ${LOG_FILE} 2>&1 &
	node.cmdSocat = exec.CommandContext(context.Background(), "socat", "-d",
		fmt.Sprintf("pty,raw,echo=0,link=%s", node.ptyPath),
		fmt.Sprintf("pty,raw,echo=0,link=%s", node.ptyPath+"d"))

	if pipeStdErr, err = node.cmdSocat.StderrPipe(); err != nil {
		return err
	}
	go node.processErrorReader(pipeStdErr, node.loggerNcp)
	if err = node.addProcess(node.cmdSocat, 100*time.Millisecond); err != nil {
		return err
	}

	// start docker OTBR
	// # https://docs.docker.com/engine/reference/run/
	//# -d flag to run main docker detached in the background.
	//# -t flag must not be used when stdinput is piped to this script. So -it becomes -i
	//# --rm flag to remove container after exit to avoid pollution of Docker data. Remove this for post mortem debug.
	//# --entrypoint overrides the default otbr docker startup script - non-trivial to use see docs.
	//# -c provides cmd arguments for the 'entrypoint' executable.
	//# sed pipe prepends a log string to each line coming from docker.
	//docker run --name ${CONTAINER_NAME} \
	//    --sysctl "net.ipv6.conf.all.disable_ipv6=0 net.ipv4.conf.all.forwarding=1 net.ipv6.conf.all.forwarding=1" \
	//    -p ${WEB_PORT}:80 --dns=127.0.0.1 --rm --volume $PTY_FILE2:/dev/ttyUSB0 --privileged \
	//    --entrypoint /bin/bash \
	//    openthread/otbr \
	//    -c "/app/etc/docker/docker_entrypoint.sh" \
	//     2>&1 | sed -E 's/^/[L] /' &
	node.cmdDocker = exec.CommandContext(context.Background(), "docker", "run",
		"--name", node.dockerContainerName, "--rm",
		"--sysctl", "net.ipv6.conf.all.disable_ipv6=0 net.ipv4.conf.all.forwarding=1 net.ipv6.conf.all.forwarding=1",
		"-p", fmt.Sprintf("%d:80", node.httpPort),
		"--entrypoint", "/bin/bash",
		"openthread/otbr",
		"-c", "/app/etc/docker/docker_entrypoint.sh")

	if pipeStdOut, err = node.cmdDocker.StdoutPipe(); err != nil {
		return err
	}
	go node.processLogReader(pipeStdOut, node.loggerNcp)

	if pipeStdErr, err = node.cmdDocker.StderrPipe(); err != nil {
		return err
	}
	go node.processLogReader(pipeStdErr, node.loggerNcp)

	if err = node.addProcess(node.cmdDocker, 2*time.Second); err != nil {
		return err
	}

	// start ot-ctl CLI script
	err = node.cmd.Start()

	return err
}

// AddProcess adds a new concurrent process Cmd to the node, to be run and monitored.
func (node *OtbrNode) addProcess(cmd *exec.Cmd, startupDelay time.Duration) error {
	simplelogger.Debugf("Starting process: %v", cmd)
	if err := cmd.Start(); err != nil {
		return err
	}

	// monitor started process in the background, signal error if it exits with non-zero exit code.
	errChan := make(chan error, 1)
	node.waitGroup.Add(1)
	go func(cmd *exec.Cmd) {
		defer node.waitGroup.Done()
		_ = cmd.Wait()
		if cmd.ProcessState.Exited() {
			ec := cmd.ProcessState.ExitCode()
			if ec > 0 {
				errChan <- fmt.Errorf("process exited with error code %d, %s", ec, cmd.Path)
			}
		}
	}(cmd)

	// wait for either startupDelay to pass or process to exit with error.
	simplelogger.Debugf("Waiting for process startupDelay (%s)", startupDelay.String())
	deadline := time.After(startupDelay)
	var err error = nil
loop:
	for {
		select {
		case err = <-errChan:
			break loop
		case <-deadline:
			break loop
		}
	}
	return err // if node's process caused an error within the startupDelay, return it.
}

func (node *OtbrNode) exitNcp() error {
	simplelogger.AssertTrue(node.cfg.IsNcp)

	err := node.Exit()

	// stop all processes and wait for all threads to finish.
	_ = node.cmdDocker.Process.Signal(syscall.SIGTERM)
	_ = node.cmdSocat.Process.Signal(syscall.SIGTERM)
	node.waitGroup.Wait()

	// remove docker container
	rmCmd := exec.CommandContext(context.Background(), "docker", "rm", node.dockerContainerName)
	err2 := rmCmd.Run()

	close(node.loggerNcp)

	if err != nil {
		return err
	}
	return err2
}

// processPtyReader reads data from a PTY device and sends it directly to the UART of the node.
func (node *Node) processPtyReader(reader io.Reader) {
	buf := make([]byte, 1024) // FIXME size

loop:
	for reader != nil {
		n, err := reader.Read(buf)

		if n > 0 {
			node.S.Dispatcher().SendToUART(node.Id, buf[0:n])
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				simplelogger.Debugf("%v - processPtyReader was closed.", node)
				break loop
			}
			simplelogger.Errorf("%v - processPtyReader error: %v", node, err)
			break loop
		}
	}
	node.onProcessStops()
}

// processPtyPiper pipes data from node's ptyChan (where virtual UART data comes in) to node's associated
// PTY (for handling by an OT NCP)
func (node *Node) processPtyPiper() {
loop:
	for {
		// check when the PTY device becomes available.
		_, err := os.Stat(node.ptyPath)
		if node.ptyFile == nil && err == nil {
			node.ptyFile, err = os.OpenFile(node.ptyPath, os.O_RDWR, 0)
			if err != nil {
				err = fmt.Errorf("%v - processPtyPiper couldn't open ptyFile: %v", node, err)
				simplelogger.Error(err)
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

	go node.processPtyReader(node.ptyFile)

loop2:
	for {
		select {
		case b := <-node.ptyChan:
			_, err := node.ptyFile.Write(b)
			if err != nil {
				if errors.Is(err, io.EOF) {
					simplelogger.Debugf("%v - processPtyPiper output was closed.", node)
					break loop2
				}
				err = fmt.Errorf("%v - processPtyPiper error: %v", node, err)
				simplelogger.Errorf("%v", err)
				break loop2
			}
		case <-node.S.ctx.Done():
			break loop2
		}
	}

	node.onProcessStops()
}

// handles an incoming log message from otOutFilter and its detected loglevel (otLevel). It is written
// to the node's NCP log file. Display of the log message is done by the Dispatcher.
func (node *OtbrNode) handlerNcpLogMsg(otLevel string, msg string) {
	node.loggerNcp <- msg

	// create a node-specific log message that may be used by the Dispatcher's Watch function.
	lev := dispatcher.ParseWatchLogLevel(otLevel)
	node.S.PostAsync(false, func() {
		node.S.Dispatcher().WatchMessage(node.Id, lev, msg)
	})
}

// processLogReader reads lines from 'reader'. These lines are sent as Watch messages and written to a logger.
func (node *OtbrNode) processLogReader(reader io.Reader, logger chan string) {
	// Below filter takes out any OT node log-message lines and sends these to the handler.
	scanner := bufio.NewScanner(otoutfilter.NewOTOutFilter(bufio.NewReader(reader), "", node.handlerNcpLogMsg))
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		line := scanner.Text()
		logger <- line

		// detect specific OTBR docker container failure conditions
		if strings.Contains(line, "can't initialize ip6tables table") {
			simplelogger.Errorf("OTBR requires 'sudo modprobe ip6table_filter' run before starting OTNS.")
		}
	}
	node.onProcessStops()
}
