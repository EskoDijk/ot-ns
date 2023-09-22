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

package logger

import (
	"fmt"
	"os"
	"time"

	. "github.com/openthread/ot-ns/types"
)

type NodeLogger struct {
	Id           NodeId
	CurrentLevel WatchLogLevel

	logFile       *os.File
	logFileName   string
	isFileEnabled bool
	entries       chan logEntry
	timestampUs   uint64
}

var (
	nodeLogs = make(map[NodeId]*NodeLogger, 10)
)

func GetNodeLogger(simulationId int, cfg *NodeConfig) *NodeLogger {
	var log *NodeLogger
	nodeid := cfg.ID
	if _, ok := nodeLogs[nodeid]; !ok {
		log = &NodeLogger{
			Id:            nodeid,
			CurrentLevel:  ErrorLevel,
			entries:       make(chan logEntry, 1000),
			logFileName:   getLogFileName(simulationId, nodeid),
			isFileEnabled: cfg.NodeLogFile,
		}
		nodeLogs[nodeid] = log
		if log.isFileEnabled {
			log.createLogFile()
		}
	}
	return nodeLogs[nodeid]
}

func getLogFileName(simId int, nodeId NodeId) string {
	return fmt.Sprintf("tmp/%d_%d.log", simId, nodeId)
}

func (nl *NodeLogger) createLogFile() {
	var err error
	if err = os.RemoveAll(nl.logFileName); err != nil {
		nl.Errorf("remove existing node log file %s failed, file logging disabled (%+v)", nl.logFileName, err)
		nl.isFileEnabled = false
		return
	}

	nl.logFile, err = os.OpenFile(nl.logFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
	if err != nil {
		nl.Errorf("opening node log file %s failed: %+v", nl.logFileName, err)
		nl.isFileEnabled = false
		return
	}

	header := fmt.Sprintf("#\n# OpenThread node log for %s Created %s\n", GetNodeName(nl.Id),
		time.Now().Format(time.RFC3339)) +
		"# SimTimeUs NodeTime     Lev LogModule       Message"
	_ = nl.writeToLogFile(header)
	nl.Debugf("Node log file '%s' opened.", nl.logFileName)
}

func NodeLogf(nodeid NodeId, level WatchLogLevel, format string, args ...interface{}) {
	log := nodeLogs[nodeid]
	if level > log.CurrentLevel && !log.isFileEnabled {
		return
	}
	msg := getMessage(format, args)
	entry := logEntry{
		NodeId: nodeid,
		Level:  level,
		Msg:    msg,
	}
	select {
	case log.entries <- entry:
		break
	default:
		log.DisplayPendingLogEntries(log.timestampUs)
		log.entries <- entry
	}
}

func (nl *NodeLogger) Log(level WatchLogLevel, msg string) {
	if level > nl.CurrentLevel && !nl.isFileEnabled {
		return
	}
	NodeLogf(nl.Id, level, msg)
}

func (nl *NodeLogger) Logf(level WatchLogLevel, format string, args []interface{}) {
	if level > nl.CurrentLevel && !nl.isFileEnabled {
		return
	}
	NodeLogf(nl.Id, level, format, args)
}

func (nl *NodeLogger) Trace(args ...interface{}) {
	if TraceLevel > nl.CurrentLevel {
		return
	}
	NodeLogf(nl.Id, TraceLevel, "", args...)
}

func (nl *NodeLogger) Tracef(format string, args ...interface{}) {
	if TraceLevel > nl.CurrentLevel {
		return
	}
	NodeLogf(nl.Id, TraceLevel, format, args...)
}

func (nl *NodeLogger) Debugf(format string, args ...interface{}) {
	NodeLogf(nl.Id, DebugLevel, format, args...)
}

func (nl *NodeLogger) Infof(format string, args ...interface{}) {
	NodeLogf(nl.Id, InfoLevel, format, args...)
}

func (nl *NodeLogger) Info(format string) {
	NodeLogf(nl.Id, InfoLevel, format)
}

func (nl *NodeLogger) Warnf(format string, args ...interface{}) {
	NodeLogf(nl.Id, WarnLevel, format, args...)
}

func (nl *NodeLogger) Warn(format string) {
	NodeLogf(nl.Id, WarnLevel, format)
}

func (nl *NodeLogger) Errorf(format string, args ...interface{}) {
	NodeLogf(nl.Id, ErrorLevel, format, args...)
}

func (nl *NodeLogger) Error(err error) {
	if err == nil {
		return
	}
	NodeLogf(nl.Id, ErrorLevel, "Error: %v", err)
}

func (nl *NodeLogger) Panicf(format string, args ...interface{}) {
	NodeLogf(nl.Id, PanicLevel, format, args...)
}

func (nl *NodeLogger) writeToLogFile(line string) error {
	if !nl.isFileEnabled {
		return nil
	}
	_, err := nl.logFile.WriteString(line + "\n")
	if err != nil {
		_ = nl.logFile.Close()
		nl.logFile = nil
		nl.isFileEnabled = false
		nl.Errorf("couldn't write to node log file (%s), closing it", nl.logFileName)
	}
	return err
}

func (nl *NodeLogger) DisplayPendingLogEntries(ts uint64) {
	nl.timestampUs = ts
	tsStr := fmt.Sprintf("%11d ", ts)
	nodeStr := GetNodeName(nl.Id)
	for {
		select {
		case ent := <-nl.entries:
			logStr := tsStr + ent.Msg
			isDisplayEntry := nl.CurrentLevel >= ent.Level
			if ent.Level <= DebugLevel || isDisplayEntry {
				_ = nl.writeToLogFile(logStr)
			}
			if isDisplayEntry {
				logAlways(ent.Level, nodeStr+logStr)
			}
			break
		default:
			return
		}
	}
}
