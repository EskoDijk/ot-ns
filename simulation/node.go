// Copyright (c) 2020-2023, The OTNS Authors.
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
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"github.com/simonlingoogle/go-simplelogger"

	"github.com/openthread/ot-ns/dispatcher"
	"github.com/openthread/ot-ns/otoutfilter"
	. "github.com/openthread/ot-ns/types"
)

const (
	DefaultCommandTimeout = time.Second * 5
)

var (
	DoneOrErrorRegexp = regexp.MustCompile(`(Done|Error \d+: .*)`)
)

type NodeUartType int

const (
	NodeUartTypeUndefined   NodeUartType = iota
	NodeUartTypeRealTime    NodeUartType = iota
	NodeUartTypeVirtualTime NodeUartType = iota
)

type Node struct {
	S         *Simulation
	Id        int
	cfg       *NodeConfig
	cmd       *exec.Cmd
	uartType  NodeUartType
	timestamp uint64      // timestamp for logging
	logger    chan string // node log messages are written here
	ncpNode   *OtbrNode   // optional NCP node associated to this radio-node (only for RCP).

	pendingCliLines   chan string        // lines with command output (non-log) from node, pending processing
	pipeStdIn         io.WriteCloser     // pipe to write data to StdIn of Node
	pipeStdOut        io.Reader          // pipe to read StdOut data from node
	pipeStdErr        io.ReadCloser      // pipe to read StdErr data/errors/warns from node
	virtualUartReader *io.PipeReader     // to read UART data from node (events transport UART data) with pipe interface
	virtualUartPipe   *io.PipeWriter     // to fill UART data from node into the virtual pipe
	ptyChan           chan []byte        // buffers data to be written to PTY (for RCP only)
	ptyFile           io.ReadWriteCloser // for RCP only, to communicate with NCP via PTY device
	ptyPath           string             // abs file path for ptyFile
}

func newNode(s *Simulation, nodeid NodeId, cfg NodeConfig) (*Node, error) {
	simplelogger.AssertTrue(!cfg.IsNcp)
	var err error

	logFiles := fmt.Sprintf("%s/%d_%d*.log", GetTmpDir(), s.cfg.Id, nodeid)
	if !cfg.Restore {
		flashFile := fmt.Sprintf("%s/%d_%d.flash", GetTmpDir(), s.cfg.Id, nodeid)
		if err = os.RemoveAll(flashFile); err != nil {
			return nil, fmt.Errorf("remove flash file failed: %w", err)
		}
		if err = os.RemoveAll(logFiles); err != nil {
			return nil, fmt.Errorf("remove node log files failed: %w", err)
		}
	}

	simplelogger.Debugf("newNode() exe path: %s", cfg.ExecutablePath)
	cmd := exec.CommandContext(context.Background(), cfg.ExecutablePath, strconv.Itoa(nodeid), s.d.GetUnixSocketName())

	node := &Node{
		S:               s,
		Id:              nodeid,
		cfg:             &cfg,
		cmd:             cmd,
		pendingCliLines: make(chan string, 10000),
		logger:          make(chan string, 10),
		uartType:        NodeUartTypeVirtualTime,
	}

	if !cfg.IsRcp {
		node.virtualUartReader, node.virtualUartPipe = io.Pipe()
	} else {
		node.ptyChan = make(chan []byte, 1024)
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

	// open main log file for node's OT output
	go node.processLogger(0, node.logger)
	node.logger <- fmt.Sprintf("[D] %v logfile created", node)
	simplelogger.Debugf("Node log file '%s' created.", node.getLogfileName(0))

	if err = cmd.Start(); err != nil {
		close(node.logger)
		return nil, err
	}

	go node.processErrorReader(node.pipeStdErr, node.logger)

	if cfg.IsRcp {
		node.logger <- "[D] This is the log of the OT RCP executable. For CLI/NCP logs of this node, see the other logs."
		node.ptyPath = getPtyFilePath(s.cfg.Id, nodeid)
		simplelogger.Debugf("%v - starting processPtyPiper for PTY %s", node, node.ptyPath)
		go node.processPtyPiper()
	} else {
		go node.processCliReader(node.virtualUartReader)
	}

	return node, err
}

func (node *Node) String() string {
	return fmt.Sprintf("Node<%d>", node.Id)
}

func (node *Node) RunInitScript(cfg []string) error {
	simplelogger.AssertNotNil(cfg)
	for _, cmd := range cfg {
		node.Command(cmd, DefaultCommandTimeout)
	}
	return nil
}

// GetInfo returns an info string with node properties and its configured network parameters
func (node *Node) GetInfo() string {
	return fmt.Sprintf("%v - panid=0x%04x, chan=%d, eui64=%#v, extaddr=%#v, state=%s, key=%#v, mode=%v", node,
		node.GetPanid(), node.GetChannel(), node.GetEui64(), node.GetExtAddr(), node.GetState(),
		node.GetNetworkKey(), node.GetMode())
}

func (node *Node) Start() {
	node.IfconfigUp()
	node.ThreadStart()
	simplelogger.Debugf(node.GetInfo())
}

func (node *Node) IsFED() bool {
	return !node.cfg.IsMtd
}

func (node *Node) Stop() {
	node.ThreadStop()
	node.IfconfigDown()
	simplelogger.Debugf("%v - stopped, state = %s", node, node.GetState())
}

// FIXME prevent that Exit() can be called twice e.g. when exiting and failure at same time.
func (node *Node) Exit() error {
	var errNcp error
	var err error

	if node.ncpNode != nil {
		errNcp = node.ncpNode.exitNcp() // try to stop the associated NCP node, if any.
		if errNcp != nil {
			simplelogger.Errorf("Problem exiting node.ncpNode: %+v", errNcp)
		}
	}

	if node.cmd.Process != nil {
		_ = node.cmd.Process.Signal(syscall.SIGTERM)

		err = node.cmd.Wait()
		node.S.Dispatcher().RecvEvents() // ensure to receive any remaining events of exited node.
	}

	// no more events or processCliReader lines should come, so we can close virtual-UART.
	if node.virtualUartReader != nil {
		_ = node.virtualUartReader.Close()
	}

	return err
}

func (node *Node) AssurePrompt() {
	node.inputCommand("")
	if found, _ := node.TryExpectLine("", time.Second); found {
		return
	}

	node.inputCommand("")
	if found, _ := node.TryExpectLine("", time.Second); found {
		return
	}

	node.inputCommand("")
	_, _ = node.expectLine("", DefaultCommandTimeout)
}

// inputCommand sends the CLI cmd to the node. Returns success flag.
func (node *Node) inputCommand(cmd string) bool {
	// If this node has associated NCP, that gets the CLI command.
	targetNode := node
	if node.ncpNode != nil {
		targetNode = &node.ncpNode.Node
	}

	simplelogger.AssertTrue(targetNode.uartType != NodeUartTypeUndefined)
	if targetNode.uartType == NodeUartTypeRealTime {
		_, err := targetNode.pipeStdIn.Write([]byte(cmd + "\n"))
		if err != nil {
			err = fmt.Errorf("%v - ncpNode.pipeStdIn write error for cmd '%s': %v", targetNode, cmd, err)
			simplelogger.Debugf("%v", err)
			return false
		}
	} else {
		return targetNode.S.Dispatcher().SendToUART(targetNode.Id, []byte(cmd+"\n"))
	}
	return true
}

func (node *Node) CommandExpectNone(cmd string, timeout time.Duration) {
	node.inputCommand(cmd)
	_, _ = node.expectLine(cmd, timeout)
}

func (node *Node) Command(cmd string, timeout time.Duration) []string {
	var err error
	var output []string

	node.inputCommand(cmd)
	_, err = node.expectLine(cmd, timeout) // cmd itself is echoed back
	if err != nil {
		err = fmt.Errorf("%v - did not echo cmd '%s'", node, cmd)
		simplelogger.Error(err)
		return []string{}
	}
	output, err = node.expectLine(DoneOrErrorRegexp, timeout)
	if err != nil || len(output) == 0 {
		err = fmt.Errorf("%v - did not find Done or Error for cmd '%s'", node, cmd)
		simplelogger.Error(err)
		return []string{}
	}

	var result string
	var outputCl []string
	outputCl, result = output[:len(output)-1], output[len(output)-1]
	// case where a CLI command gave an error. This is not a failure of the node.
	if result != "Done" {
		err = fmt.Errorf("%v - Unexpected result for cmd '%s': %s", node, cmd, result)
		simplelogger.Debugf("%v", err)
		return output
	}

	return outputCl
}

func (node *Node) CommandExpectString(cmd string, timeout time.Duration) string {
	output := node.Command(cmd, timeout)
	if len(output) != 1 {
		err := fmt.Errorf("%v - expected 1 line, but received %d: %#v", node, len(output), output)
		simplelogger.Error(err)
		return ""
	}

	return output[0]
}

func (node *Node) CommandExpectInt(cmd string, timeout time.Duration) int {
	s := node.CommandExpectString(cmd, timeout)
	var iv int64
	var err error

	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		iv, err = strconv.ParseInt(s[2:], 16, 0)
	} else {
		iv, err = strconv.ParseInt(s, 10, 0)
	}

	if err != nil {
		err := fmt.Errorf("%v - expected Int, but received '%s'", node, s)
		simplelogger.Error(err)
		return math.MaxInt
	}
	return int(iv)
}

func (node *Node) CommandExpectHex(cmd string, timeout time.Duration) int {
	s := node.CommandExpectString(cmd, timeout)
	var iv int64
	var err error

	iv, err = strconv.ParseInt(s[2:], 16, 0)

	if err != nil {
		err := fmt.Errorf("%v - expected Hex string, but received '%s'", node, s)
		simplelogger.Error(err)
		return math.MaxInt
	}
	return int(iv)
}

func (node *Node) SetChannel(ch int) {
	simplelogger.AssertTrue(11 <= ch && ch <= 26)
	node.Command(fmt.Sprintf("channel %d", ch), DefaultCommandTimeout)
}

func (node *Node) GetChannel() int {
	return node.CommandExpectInt("channel", DefaultCommandTimeout)
}

func (node *Node) GetChildList() (childlist []int) {
	s := node.CommandExpectString("child list", DefaultCommandTimeout)
	ss := strings.Split(s, " ")

	for _, ids := range ss {
		id, err := strconv.Atoi(ids)
		if err != nil {
			simplelogger.Panicf("unpexpected child list: %#v", s)
		}
		childlist = append(childlist, id)
	}
	return
}

func (node *Node) GetChildTable() {
	// todo: not implemented yet
}

func (node *Node) GetChildTimeout() int {
	return node.CommandExpectInt("childtimeout", DefaultCommandTimeout)
}

func (node *Node) SetChildTimeout(timeout int) {
	node.Command(fmt.Sprintf("childtimeout %d", timeout), DefaultCommandTimeout)
}

func (node *Node) GetContextReuseDelay() int {
	return node.CommandExpectInt("contextreusedelay", DefaultCommandTimeout)
}

func (node *Node) SetContextReuseDelay(delay int) {
	node.Command(fmt.Sprintf("contextreusedelay %d", delay), DefaultCommandTimeout)
}

func (node *Node) GetNetworkName() string {
	return node.CommandExpectString("networkname", DefaultCommandTimeout)
}

func (node *Node) SetNetworkName(name string) {
	node.Command(fmt.Sprintf("networkname %s", name), DefaultCommandTimeout)
}

func (node *Node) GetEui64() string {
	return node.CommandExpectString("eui64", DefaultCommandTimeout)
}

func (node *Node) SetEui64(eui64 string) {
	node.Command(fmt.Sprintf("eui64 %s", eui64), DefaultCommandTimeout)
}

func (node *Node) GetExtAddr() uint64 {
	s := node.CommandExpectString("extaddr", DefaultCommandTimeout)
	v, err := strconv.ParseUint(s, 16, 64)
	simplelogger.PanicIfError(err)
	return v
}

func (node *Node) SetExtAddr(extaddr uint64) {
	node.Command(fmt.Sprintf("extaddr %016x", extaddr), DefaultCommandTimeout)
}

func (node *Node) GetExtPanid() string {
	return node.CommandExpectString("extpanid", DefaultCommandTimeout)
}

func (node *Node) SetExtPanid(extpanid string) {
	node.Command(fmt.Sprintf("extpanid %s", extpanid), DefaultCommandTimeout)
}

func (node *Node) GetIfconfig() string {
	return node.CommandExpectString("ifconfig", DefaultCommandTimeout)
}

func (node *Node) IfconfigUp() {
	node.Command("ifconfig up", DefaultCommandTimeout)
}

func (node *Node) IfconfigDown() {
	node.Command("ifconfig down", DefaultCommandTimeout)
}

func (node *Node) GetIpAddr() []string {
	// todo: parse IPv6 addresses
	addrs := node.Command("ipaddr", DefaultCommandTimeout)
	return addrs
}

func (node *Node) GetIpAddrLinkLocal() []string {
	// todo: parse IPv6 addresses
	addrs := node.Command("ipaddr linklocal", DefaultCommandTimeout)
	return addrs
}

func (node *Node) GetIpAddrMleid() []string {
	// todo: parse IPv6 addresses
	addrs := node.Command("ipaddr mleid", DefaultCommandTimeout)
	return addrs
}

func (node *Node) GetIpAddrRloc() []string {
	addrs := node.Command("ipaddr rloc", DefaultCommandTimeout)
	return addrs
}

func (node *Node) GetIpMaddr() []string {
	// todo: parse IPv6 addresses
	addrs := node.Command("ipmaddr", DefaultCommandTimeout)
	return addrs
}

func (node *Node) GetIpMaddrPromiscuous() bool {
	return node.CommandExpectEnabledOrDisabled("ipmaddr promiscuous", DefaultCommandTimeout)
}

func (node *Node) IpMaddrPromiscuousEnable() {
	node.Command("ipmaddr promiscuous enable", DefaultCommandTimeout)
}

func (node *Node) IpMaddrPromiscuousDisable() {
	node.Command("ipmaddr promiscuous disable", DefaultCommandTimeout)
}

func (node *Node) GetPromiscuous() bool {
	return node.CommandExpectEnabledOrDisabled("promiscuous", DefaultCommandTimeout)
}

func (node *Node) PromiscuousEnable() {
	node.Command("promiscuous enable", DefaultCommandTimeout)
}

func (node *Node) PromiscuousDisable() {
	node.Command("promiscuous disable", DefaultCommandTimeout)
}

func (node *Node) GetRouterEligible() bool {
	return node.CommandExpectEnabledOrDisabled("routereligible", DefaultCommandTimeout)
}

func (node *Node) RouterEligibleEnable() {
	node.Command("routereligible enable", DefaultCommandTimeout)
}

func (node *Node) RouterEligibleDisable() {
	node.Command("routereligible disable", DefaultCommandTimeout)
}

func (node *Node) GetJoinerPort() int {
	return node.CommandExpectInt("joinerport", DefaultCommandTimeout)
}

func (node *Node) SetJoinerPort(port int) {
	node.Command(fmt.Sprintf("joinerport %d", port), DefaultCommandTimeout)
}

func (node *Node) GetKeySequenceCounter() int {
	return node.CommandExpectInt("keysequence counter", DefaultCommandTimeout)
}

func (node *Node) SetKeySequenceCounter(counter int) {
	node.Command(fmt.Sprintf("keysequence counter %d", counter), DefaultCommandTimeout)
}

func (node *Node) GetKeySequenceGuardTime() int {
	return node.CommandExpectInt("keysequence guardtime", DefaultCommandTimeout)
}

func (node *Node) SetKeySequenceGuardTime(guardtime int) {
	node.Command(fmt.Sprintf("keysequence guardtime %d", guardtime), DefaultCommandTimeout)
}

type LeaderData struct {
	PartitionID       int
	Weighting         int
	DataVersion       int
	StableDataVersion int
	LeaderRouterID    int
}

func (node *Node) GetLeaderData() (leaderData LeaderData) {
	var err error
	output := node.Command("leaderdata", DefaultCommandTimeout)
	for _, line := range output {
		if strings.HasPrefix(line, "Partition ID:") {
			leaderData.PartitionID, err = strconv.Atoi(line[14:])
			simplelogger.PanicIfError(err)
		}

		if strings.HasPrefix(line, "Weighting:") {
			leaderData.Weighting, err = strconv.Atoi(line[11:])
			simplelogger.PanicIfError(err)
		}

		if strings.HasPrefix(line, "Data Version:") {
			leaderData.DataVersion, err = strconv.Atoi(line[14:])
			simplelogger.PanicIfError(err)
		}

		if strings.HasPrefix(line, "Stable Data Version:") {
			leaderData.StableDataVersion, err = strconv.Atoi(line[21:])
			simplelogger.PanicIfError(err)
		}

		if strings.HasPrefix(line, "Leader Router ID:") {
			leaderData.LeaderRouterID, err = strconv.Atoi(line[18:])
			simplelogger.PanicIfError(err)
		}
	}
	return
}

func (node *Node) GetLeaderPartitionId() int {
	return node.CommandExpectInt("leaderpartitionid", DefaultCommandTimeout)
}

func (node *Node) SetLeaderPartitionId(partitionid int) {
	node.Command(fmt.Sprintf("leaderpartitionid 0x%x", partitionid), DefaultCommandTimeout)
}

func (node *Node) GetLeaderWeight() int {
	return node.CommandExpectInt("leaderweight", DefaultCommandTimeout)
}

func (node *Node) SetLeaderWeight(weight int) {
	node.Command(fmt.Sprintf("leaderweight 0x%x", weight), DefaultCommandTimeout)
}

func (node *Node) FactoryReset() {
	simplelogger.Warnf("%v - factoryreset", node)
	node.inputCommand("factoryreset")
	node.AssurePrompt()
	simplelogger.Debugf("%v - ready", node)
}

func (node *Node) Reset() {
	simplelogger.Warnf("%v - reset", node)
	node.inputCommand("reset")
	node.AssurePrompt()
	simplelogger.Debugf("%v - ready", node)
}

func (node *Node) GetNetworkKey() string {
	return node.CommandExpectString("networkkey", DefaultCommandTimeout)
}

func (node *Node) SetNetworkKey(key string) {
	node.Command(fmt.Sprintf("networkkey %s", key), DefaultCommandTimeout)
}

func (node *Node) GetMode() string {
	// todo: return Mode type rather than just string
	return node.CommandExpectString("mode", DefaultCommandTimeout)
}

func (node *Node) SetMode(mode string) {
	node.Command(fmt.Sprintf("mode %s", mode), DefaultCommandTimeout)
}

func (node *Node) GetPanid() uint16 {
	// todo: return Mode type rather than just string
	return uint16(node.CommandExpectInt("panid", DefaultCommandTimeout))
}

func (node *Node) SetPanid(panid uint16) {
	node.Command(fmt.Sprintf("panid 0x%x", panid), DefaultCommandTimeout)
}

func (node *Node) GetRloc16() uint16 {
	return uint16(node.CommandExpectHex("rloc16", DefaultCommandTimeout))
}

func (node *Node) GetRouterSelectionJitter() int {
	return node.CommandExpectInt("routerselectionjitter", DefaultCommandTimeout)
}

func (node *Node) SetRouterSelectionJitter(timeout int) {
	node.Command(fmt.Sprintf("routerselectionjitter %d", timeout), DefaultCommandTimeout)
}

func (node *Node) GetRouterUpgradeThreshold() int {
	return node.CommandExpectInt("routerupgradethreshold", DefaultCommandTimeout)
}

func (node *Node) SetRouterUpgradeThreshold(timeout int) {
	node.Command(fmt.Sprintf("routerupgradethreshold %d", timeout), DefaultCommandTimeout)
}

func (node *Node) GetRouterDowngradeThreshold() int {
	return node.CommandExpectInt("routerdowngradethreshold", DefaultCommandTimeout)
}

func (node *Node) SetRouterDowngradeThreshold(timeout int) {
	node.Command(fmt.Sprintf("routerdowngradethreshold %d", timeout), DefaultCommandTimeout)
}

func (node *Node) GetState() string {
	return node.CommandExpectString("state", DefaultCommandTimeout)
}

func (node *Node) ThreadStart() {
	node.Command("thread start", DefaultCommandTimeout)
}

func (node *Node) ThreadStop() {
	node.Command("thread stop", DefaultCommandTimeout)
}

// GetVersion gets the version string of the OpenThread node.
func (node *Node) GetVersion() string {
	return node.CommandExpectString("version", DefaultCommandTimeout)
}

func (node *Node) GetExecutablePath() string {
	return node.cfg.ExecutablePath
}

func (node *Node) GetExecutableName() string {
	return filepath.Base(node.cfg.ExecutablePath)
}

func (node *Node) GetSingleton() bool {
	s := node.CommandExpectString("singleton", DefaultCommandTimeout)
	if s == "true" {
		return true
	} else if s == "false" {
		return false
	} else {
		simplelogger.Panicf("expect true/false, but read: %#v", s)
		return false
	}
}

func (node *Node) TryExpectLine(line interface{}, timeout time.Duration) (bool, []string) {
	var outputLines []string

	deadline := time.After(timeout)
	cliNode := node
	if node.ncpNode != nil {
		cliNode = &node.ncpNode.Node
	}

	for {
		select {
		case <-deadline:
			return false, outputLines
		case readLine, ok := <-cliNode.pendingCliLines:
			if !ok {
				simplelogger.Debugf("%v - cliNode.pendingLines channel was closed", node)
				return false, outputLines
			}

			if !strings.HasPrefix(readLine, "|") && !strings.HasPrefix(readLine, "+") {
				simplelogger.Debugf("%v %s", node, readLine)
			}

			outputLines = append(outputLines, readLine)
			if isLineMatch(readLine, line) {
				// found the exact line
				return true, outputLines
			} else {
				// TODO hack: output scan result here, should have better implementation
				//| J | Network Name     | Extended PAN     | PAN  | MAC Address      | Ch | dBm | LQI |
				if strings.HasPrefix(readLine, "|") || strings.HasPrefix(readLine, "+") {
					fmt.Printf("%s\n", readLine)
				}
			}
		default:
			node.S.Dispatcher().RecvEvents() // keep virtual-UART events coming.
		}
	}
}

func (node *Node) expectLine(line interface{}, timeout time.Duration) ([]string, error) {
	found, output := node.TryExpectLine(line, timeout)
	if !found {
		err := errors.Errorf("%v - expect line timeout: %#v", node, line)
		simplelogger.Debugf("%v", err)
		return []string{}, err
	}

	return output, nil
}

func (node *Node) CommandExpectEnabledOrDisabled(cmd string, timeout time.Duration) bool {
	output := node.CommandExpectString(cmd, timeout)
	if output == "Enabled" {
		return true
	} else if output == "Disabled" {
		return false
	} else {
		simplelogger.Panicf("expect Enabled/Disabled, but read: %#v", output)
	}
	return false
}

func (node *Node) Ping(addr string, payloadSize int, count int, interval int, hopLimit int) {
	cmd := fmt.Sprintf("ping async %s %d %d %d %d", addr, payloadSize, count, interval, hopLimit)
	node.inputCommand(cmd)
	_, _ = node.expectLine(cmd, DefaultCommandTimeout)
	node.AssurePrompt()
}

func isLineMatch(line string, _expectedLine interface{}) bool {
	switch expectedLine := _expectedLine.(type) {
	case string:
		return line == expectedLine
	case *regexp.Regexp:
		return expectedLine.MatchString(line)
	case []string:
		for _, s := range expectedLine {
			if s == line {
				return true
			}
		}
	default:
		simplelogger.Panic("unknown expected string")
	}
	return false
}

func (node *Node) DumpStat() string {
	return fmt.Sprintf("extaddr %016x, addr %04x, state %-6s", node.GetExtAddr(), node.GetRloc16(), node.GetState())
}

func (node *Node) setupMode() {
	if node.cfg.IsRouter {
		// routers should be full functional and rx always on
		simplelogger.AssertFalse(node.cfg.IsMtd)
		simplelogger.AssertFalse(node.cfg.RxOffWhenIdle)
	}

	// only MED can use RxOffWhenIdle
	simplelogger.AssertTrue(!node.cfg.RxOffWhenIdle || node.cfg.IsMtd)

	mode := ""
	if !node.cfg.RxOffWhenIdle {
		mode += "r"
	}
	if !node.cfg.IsMtd {
		mode += "d"
	}
	mode += "n"

	node.SetMode(mode)

	if !node.cfg.IsRouter && !node.cfg.IsMtd {
		node.RouterEligibleDisable()
	}
}

// onUartWrite is called when UART data is written by the node using an event message.
func (node *Node) onUartWrite(data []byte) {
	if node.ptyChan != nil {
		node.ptyChan <- data
	} else if node.virtualUartPipe != nil {
		_, _ = node.virtualUartPipe.Write(data)
	} else {
		simplelogger.Panicf("onUartWrite - %v misconfiguration", node)
	}
}

// onProcessStops is called when the node process exits/stops. It may be called multiple times.
func (node *Node) onProcessStops() {
	// TODO
}

func (node *Node) getLogfileName(logId int) string {
	idStr := ""
	if logId > 0 {
		idStr = fmt.Sprintf(".%d", logId)
	}
	return fmt.Sprintf("%s/%d_%d%s.log", GetTmpDir(), node.S.cfg.Id, node.Id, idStr)
}

// processLogger reads from the logger channel and writes lines to the log file 'lodId'.
func (node *Node) processLogger(logId int, logger chan string) {
	fn := node.getLogfileName(logId)
	f, err := os.OpenFile(fn, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
	if err != nil {
		simplelogger.Fatalf("create node log file id=%d failed: %w", logId, err)
		return
	}

	for {
		line, ok := <-logger
		if !ok {
			break
		}

		//logStr := fmt.Sprintf("%-10d ", node.timestamp) + line + "\n" // TODO use timestamp in log line
		logStr := line + "\n"
		_, err = f.WriteString(logStr)
		if err != nil {
			simplelogger.Warnf("[%d] %s", logId, line)
			simplelogger.Errorf("write to node log file id=%d failed: %w", logId, err)
			break
		}
	}
	_ = f.Close()
}

// handles an incoming log message from otOutFilter and its detected loglevel (otLevel). It is written
// to the node's log file. Display of the log message is done by the Dispatcher.
func (node *Node) handlerLogMsg(otLevel string, msg string) {
	node.logger <- msg

	// create a node-specific log message that may be used by the Dispatcher's Watch function.
	lev := dispatcher.ParseWatchLogLevel(otLevel)
	node.S.PostAsync(false, func() {
		node.S.Dispatcher().WatchMessage(node.Id, lev, msg)
	})
}

// processCliReader reads CLI lines, filters out any OT log messages, and sends the remaining CLI lines/output
// to the watch function and to the logger.
func (node *Node) processCliReader(reader io.Reader) {
	// Below filter takes out any OT node log-message lines and sends these to the handler.
	scanner := bufio.NewScanner(otoutfilter.NewOTOutFilter(bufio.NewReader(reader), "", node.handlerLogMsg))
	scanner.Split(bufio.ScanLines)

	// Below loop handles the remainder of OT node output that are not OT node log messages.
	for scanner.Scan() {
		line := scanner.Text()
		node.logger <- line

		select {
		case node.pendingCliLines <- line:
			break
		default: // panic - should not happen normally. If so, needs a fix.
			simplelogger.Panicf("%v - node.pendingCliLines exceeded length %v", node, len(node.pendingCliLines))
		}
	}
	node.onProcessStops()
	close(node.pendingCliLines)
}

// processErrorReader reads lines from a (StdErr) 'reader'. These lines are sent as Watch messages and written to a
// logger. It treats any read line as a 'node process error' and schedules to stop the node automatically.
func (node *Node) processErrorReader(reader io.Reader, logger chan string) {
	scanner := bufio.NewScanner(bufio.NewReader(reader)) // no filter applied.
	scanner.Split(bufio.ScanLines)
	wasProcessFailed := false

	for scanner.Scan() {
		line := scanner.Text()
		logger <- line

		node.S.PostAsync(false, func() {
			node.S.Dispatcher().WatchMessage(node.Id, dispatcher.WatchCritLevel, fmt.Sprintf("StdErr> %s", line))
		})

		if !wasProcessFailed {
			wasProcessFailed = true
			simplelogger.Debugf("%v process failed", node)
			simplelogger.Errorf("%v StdErr> %s", node, line)

			node.S.PostAsync(false, func() {
				if _, ok := node.S.nodes[node.Id]; ok {
					if node.S.ctx.Err() == nil {
						simplelogger.Warnf("Deleting node %v due to process failure.", node.Id)
						_ = node.S.DeleteNode(node.Id)
					}
				}
			})
		}
	}
	node.onProcessStops()
}
