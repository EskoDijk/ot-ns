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

package cli

import (
	_ "embed"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/mitchellh/go-wordwrap"
	"github.com/simonlingoogle/go-simplelogger"
	"golang.org/x/term"
)

type Help struct {
	termWidth   uint
	maxCmdWidth uint
	commands    map[string]string
}

var (
	cmdHeaderPattern = regexp.MustCompile("###+ .+")
)

var commandHelp = map[string]string{
	"help":       "Show help for a specific command.",
	"add":        "Add a node to the simulation.",
	"coaps":      "Enable collecting info about CoAP messages.",
	"counters":   "Display runtime counters of the simulation.",
	"cv":         "Configure visualization options.",
	"del":        "Delete node(s) by node ID.",
	"energy":     "Save node energy use information to a file.",
	"exe":        "Display or set the OT executables used per node type.",
	"exit":       "Exit OTNS (if not in node context) or exit node context.",
	"go":         "Simulate for a specified time.",
	"joins":      "Connect finished joiner sessions.",
	"log":        "Inspect current log level or set a new log level.",
	"move":       "Move a node to a target position.",
	"netinfo":    "Set network info.",
	"node":       "Switch CLI to a specific node context, or send a command to a specific node.",
	"nodes":      "List all nodes.",
	"partitions": "List all Thread Partitions.",
	"pts":        "(synonym for: partitions)",
	"ping":       "Ping from a given source node to a destination.",
	"pings":      "Display finished 'ping' commands.",
	"plr":        "Get or set the global packet loss ratio.",
	"radio":      "Set a node's radio on/off or set fail-time parameters.",
	"radiomodel": "Get or set the current RF simulation radio model.",
	"scan":       "Let a node perform a network scan.",
	"speed":      "Get or set the curent simulation speed.",
	"time":       "Display current simulation time in us.",
	"title":      "Set simulation window title.",
	"watch":      "Enable additional detailed log messages for selected node(s).",
	"unwatch":    "Disable the additional detailed log messages set by 'watch'.",
	"web":        "Open a web browser for visualization.",
}

// Embed the CLI help file as a static resource.
//
//go:embed README.md
var cliHelpFile string

// Creates new Help object. It is used to display CLI commands help to the user.
func newHelp() Help {
	h := Help{
		termWidth:   80,
		maxCmdWidth: 10,
		commands:    make(map[string]string),
	}
	h.parseHelpFile()
	h.update()
	return h
}

// Updates the Help object to take into account current user's terminal size.
func (help *Help) update() {
	fdTerm := int(os.Stdout.Fd()) // Windows platform requires cast to int.
	if term.IsTerminal(fdTerm) {
		width, _, err := term.GetSize(fdTerm)
		simplelogger.PanicIfError(err, "Could not get terminal size.")
		help.termWidth = uint(width)
	}
}

// Output short help for all commands.
func (help *Help) outputGeneralHelp() string {
	cmdHelp := ""
	// get a sorted list of commands
	cmds := make([]string, 0, len(commandHelp))
	for k := range commandHelp {
		cmds = append(cmds, k)
	}
	sort.Strings(cmds)

	for _, c := range cmds {
		cmdHelp += fmt.Sprintf("%-15s %s\n", c, commandHelp[c])
	}
	return cmdHelp +
		wordwrap.WrapString("\nFor detailed help per command, use: 'help <command>'\n",
			help.termWidth) +
		wordwrap.WrapString("\nFor detailed one-page CLI command reference go to:\n"+
			"https://github.com/EskoDijk/ot-ns/blob/main/cli/README.md\n",
			help.termWidth)
}

// Output help for one specific command.
func (help *Help) outputCommandHelp(command string) string {
	return help.outputHelp([]string{command})
}

// Output help for one or more specific commands, in given order.
func (help *Help) outputHelp(commands []string) string {
	help.update()
	s := ""
	for _, cmd := range commands {
		explanation, ok := help.commands[cmd]
		if !ok {
			explanation = "(Non-existent command.)"
		}
		w := help.termWidth - help.maxCmdWidth - 1
		explWrapped := strings.Split(wordwrap.WrapString(explanation, w), "\n")
		for _, line := range explWrapped {
			if cmdHeaderPattern.MatchString(line) {
				s += line[strings.Index(line, " ")+1:] + "\n"
			} else {
				s += "  " + line + "\n"
			}
		}
	}
	return s
}

func (help *Help) parseHelpFile() {
	indentString := "    "
	lines := strings.Split(cliHelpFile, "\n")
	activeCmd := ""
	indent := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if len(line) == 0 {
			continue
		}

		if line == "```bash" {
			line = "\nExample:"
			indent = 2
		} else if line == "```" {
			line = ""
			indent = 0
		} else if cmdHeaderPattern.MatchString(line) {
			cmdline := markdownUnquote(strings.TrimSpace(line))
			cmdSingle := cmdline[strings.Index(cmdline, " ")+1:]
			idx := strings.Index(cmdSingle, " ")
			if idx > 0 {
				cmdSingle = cmdSingle[0:idx]
			}
			activeCmd = cmdSingle
			if _, ok := help.commands[activeCmd]; !ok {
				help.commands[activeCmd] = ""
			}
			line = cmdline
			indent = 0
		}

		if len(activeCmd) > 0 {
			help.commands[activeCmd] += indentString[0:indent] + line + "\n"
		}
	}
}

func markdownUnquote(md string) string {
	// TODO: consider that double backslash may be present in the future in the Markdown.
	// TODO: change MD links to text
	md = strings.ReplaceAll(md, "\\", "")
	return md
}
