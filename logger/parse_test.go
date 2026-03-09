// Copyright (c) 2026, The OTNS Authors.
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
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
)

// levelStrings are all level names ParseLevelString accepts.
var levelStrings = []string{
	"micro", "trace", "T", "debug", "D", "info", "I", "note", "N",
	"warn", "warning", "W", "crit", "critical", "error", "err", "C", "E",
	"off", "none", "default", "def",
}

// settableLevels are the levels a user can end up with, most to least severe.
var settableLevels = []Level{
	PanicLevel, ErrorLevel, WarnLevel, NoteLevel, InfoLevel, DebugLevel, TraceLevel, MicroLevel,
}

// zapLevels is indexed as zapLevels[level-MinLevel] with no bounds check, so the array, MinLevel
// and the Level constants must stay in sync. A desync panics on the first log call at that level.
func TestZapLevelMapping(t *testing.T) {
	assert.Equal(t, int(MicroLevel-MinLevel)+1, len(zapLevels),
		"zapLevels must have exactly one entry per level from MinLevel to MicroLevel")

	for _, lv := range append([]Level{FatalLevel}, settableLevels...) {
		idx := int(lv - MinLevel)
		assert.GreaterOrEqual(t, idx, 0, "level %d indexes before zapLevels", lv)
		assert.Less(t, idx, len(zapLevels), "level %d indexes past zapLevels", lv)
	}

	// Fatal and Panic must map to the zap levels that abort, or logger.Fatalf() stops terminating.
	assert.Equal(t, zapcore.FatalLevel, zapLevels[FatalLevel-MinLevel])
	assert.Equal(t, zapcore.PanicLevel, zapLevels[PanicLevel-MinLevel])
	assert.Equal(t, zapcore.ErrorLevel, zapLevels[ErrorLevel-MinLevel])
	assert.Equal(t, zapcore.WarnLevel, zapLevels[WarnLevel-MinLevel])
	assert.Equal(t, zapcore.DebugLevel, zapLevels[MicroLevel-MinLevel])
}

// GetLevelString panics on a level it does not know, and both SetLevel and the 'log' CLI command
// call it, so a gap crashes the simulator rather than printing a level name.
func TestGetLevelStringCoversSettableLevels(t *testing.T) {
	for _, lv := range settableLevels {
		assert.NotPanics(t, func() { _ = GetLevelString(lv) }, "no name for level %d", lv)
		assert.NotEmpty(t, GetLevelString(lv), "empty name for level %d", lv)
	}

	// "off" is reported for the Panic level: zap renders such a message as "panic", while the
	// user-facing level name (e.g. for node watching) stays the "off" that was asked for.
	assert.Equal(t, "off", GetLevelString(PanicLevel))
	assert.Equal(t, "off", GetLevelString(OffLevel))

	// Aliases deliberately do not round-trip; GetLevelString returns one canonical name per level.
	assert.Equal(t, "crit", GetLevelString(ErrorLevel))
}

// No level a user can select may suppress Panic or Fatal. Several call sites rely on
// logger.Fatalf() aborting the process.
func TestParsedLevelsNeverSuppressPanicOrFatal(t *testing.T) {
	for _, s := range levelStrings {
		lv, err := ParseLevelString(s)
		assert.NoError(t, err, "level string %q", s)
		assert.LessOrEqual(t, PanicLevel, lv, "level %q (%d) would suppress Panic", s, lv)
		assert.LessOrEqual(t, FatalLevel, lv, "level %q (%d) would suppress Fatal", s, lv)
	}

	off, err := ParseLevelString("off")
	assert.NoError(t, err)
	assert.Equal(t, PanicLevel, off)
	none, err := ParseLevelString("none")
	assert.NoError(t, err)
	assert.Equal(t, PanicLevel, none)

	_, err = ParseLevelString("not-a-level")
	assert.Error(t, err)
}

// The end-to-end property the above guards: with logging off, Fatalf still terminates the process.
func TestFatalTerminatesAtOffLevel(t *testing.T) {
	if os.Getenv("OTNS_TEST_FATAL_CHILD") == "1" {
		lv, _ := ParseLevelString("off")
		currentLevel = lv
		Fatalf("fatal message at 'off' level")
		os.Exit(99) // reached only if Fatalf failed to terminate
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestFatalTerminatesAtOffLevel")
	cmd.Env = append(os.Environ(), "OTNS_TEST_FATAL_CHILD=1")
	out, err := cmd.CombinedOutput()

	var exitErr *exec.ExitError
	assert.ErrorAs(t, err, &exitErr, "Fatalf must terminate the process; output: %s", out)
	if exitErr != nil {
		// zap exits with 1 on a fatal; 99 means Fatalf returned and the child ran on.
		assert.Equal(t, 1, exitErr.ExitCode(), "Fatalf returned instead of exiting; output: %s", out)
	}
	assert.Contains(t, string(out), "fatal message at 'off' level", "the fatal message must still be logged")
}

func TestParseSyslogPrefix(t *testing.T) {
	// Typical RCP syslog prefix with relative path and pid.
	line1 := "./ot-rfsim/ot-versions/ot-cli[119560]: Running OPENTHREAD/thread-reference"
	assert.Equal(t, "./ot-rfsim/ot-versions/ot-cli[119560]: ", ParseSyslogPrefix(line1))

	// Bare executable name.
	line2 := "ot-cli[1]: Thread version: 5"
	assert.Equal(t, "ot-cli[1]: ", ParseSyslogPrefix(line2))

	// Normal OT log line — must not match.
	line3 := "00:00:00.000 [D] Platform------: Clear ShortAddr entries"
	assert.Equal(t, "", ParseSyslogPrefix(line3))

	// Empty string — must not match.
	assert.Equal(t, "", ParseSyslogPrefix(""))
}

func TestParseOtLogLine(t *testing.T) {
	// Standard OT single-character level markers.
	ok, lvl := ParseOtLogLine("00:00:12.894 [D] SubMac--------: RadioState: Receive -> CsmaBackoff")
	assert.True(t, ok)
	assert.Equal(t, DebugLevel, lvl)

	ok, lvl = ParseOtLogLine("00:00:32.298 [I] Mle-----------: Send Advertisement (ff02:0:0:0:0:0:0:1)")
	assert.True(t, ok)
	assert.Equal(t, InfoLevel, lvl)

	ok, lvl = ParseOtLogLine("00:00:00.035 [N] Mle-----------: Attach attempt 1, AnyPartition reattaching with Active Dataset")
	assert.True(t, ok)
	assert.Equal(t, NoteLevel, lvl)

	ok, lvl = ParseOtLogLine("00:00:01.000 [W] Mac-----------: Frame tx attempt failed")
	assert.True(t, ok)
	assert.Equal(t, WarnLevel, lvl)

	ok, lvl = ParseOtLogLine("00:00:01.000 [C] Core----------: Assertion failed")
	assert.True(t, ok)
	assert.Equal(t, ErrorLevel, lvl)

	// The '-' marker (e.g. OTNS status push lines) maps to the default level.
	ok, lvl = ParseOtLogLine("00:00:00.035 [-] Otns----------: transmit=11,d841,179,ffff")
	assert.True(t, ok)
	assert.Equal(t, DefaultLevel, lvl)

	// Posix host multi-character level markers.
	ok, lvl = ParseOtLogLine("[NOTE]-AGENT---: Thread version: 1.4.0")
	assert.True(t, ok)
	assert.Equal(t, NoteLevel, lvl)

	ok, lvl = ParseOtLogLine("[CRIT]-Core----: terminate called after an exception")
	assert.True(t, ok)
	assert.Equal(t, ErrorLevel, lvl)

	ok, lvl = ParseOtLogLine("[WARN]-Mac-----: This is just a mockup test message")
	assert.True(t, ok)
	assert.Equal(t, WarnLevel, lvl)

	ok, lvl = ParseOtLogLine("[INFO]-Cli-----: command done; mockup test message")
	assert.True(t, ok)
	assert.Equal(t, InfoLevel, lvl)

	ok, lvl = ParseOtLogLine("[DEBG]-Platform: state updated; mockup test message")
	assert.True(t, ok)
	assert.Equal(t, DebugLevel, lvl)

	ok, lvl = ParseOtLogLine("(OTNS)       [T] RadioState----: EnergyState=Tx_ SubState=FrameTx RadioState=Tx_ Ch=11 RadioTime=147876808 NextStTime=+2433")
	assert.True(t, ok)
	assert.Equal(t, TraceLevel, lvl)

	// Lines without a recognizable level marker.
	ok, lvl = ParseOtLogLine("not a log line")
	assert.False(t, ok)
	assert.Equal(t, OffLevel, lvl)

	ok, lvl = ParseOtLogLine("")
	assert.False(t, ok)
	assert.Equal(t, OffLevel, lvl)
}

func TestParseOtnsStatusPush(t *testing.T) {
	// Typical OTNS status push line.
	ok, status := ParseOtnsStatusPush("00:00:04.248 [-] Otns----------: transmit=11,d841,17,ffff")
	assert.True(t, ok)
	assert.Equal(t, "transmit=11,d841,17,ffff", status)

	// A single dash after 'Otns' is enough to match.
	ok, status = ParseOtnsStatusPush("00:00:02.233 [-] Otns-: role=2")
	assert.True(t, ok)
	assert.Equal(t, "role=2", status)

	// Matches even when a syslog prefix precedes the log line.
	ok, status = ParseOtnsStatusPush("./my/path/ot-cli[42]: 00:00:02.233 [-] Otns----------: extaddr=0123456789abcdef")
	assert.True(t, ok)
	assert.Equal(t, "extaddr=0123456789abcdef", status)

	// An empty status after the marker still counts as a match.
	ok, status = ParseOtnsStatusPush("00:00:02.233 [-] Otns----------: ")
	assert.True(t, ok)
	assert.Equal(t, "", status)

	// A different OT module must not match.
	ok, status = ParseOtnsStatusPush("00:00:00.000 [D] Platform------: Clear ShortAddr entries")
	assert.False(t, ok)
	assert.Equal(t, "", status)

	// 'Otns' without any dash separator must not match.
	ok, status = ParseOtnsStatusPush("00:00:02.233 [-] Otns: transmit=1")
	assert.False(t, ok)
	assert.Equal(t, "", status)

	// Missing the ': ' separator must not match.
	ok, status = ParseOtnsStatusPush("00:00:02.233 [-] Otns---------- message here")
	assert.False(t, ok)
	assert.Equal(t, "", status)

	// Non-log lines must not match.
	ok, status = ParseOtnsStatusPush("not a log line")
	assert.False(t, ok)
	assert.Equal(t, "", status)

	ok, status = ParseOtnsStatusPush("")
	assert.False(t, ok)
	assert.Equal(t, "", status)
}

func TestSetAlternativeOtLogMarker(t *testing.T) {
	// [NOTE] format of a Posix host process: not touched, since not an RCP output
	line1 := "[NOTE]-BBA-----: BackboneAgent: Backbone Router becomes Primary!"
	result := setAlternativeOtLogMarker(line1)
	assert.Equal(t, line1, result)

	// Standard [D] format
	line2 := "00:00:13.613 [D] SubMac--------: RadioState: Receive -> CsmaBackoff"
	result = setAlternativeOtLogMarker(line2)
	assert.Equal(t, "00:00:13.613 [D] SubMac--------| RadioState: Receive -> CsmaBackoff", result)

	// longer format (hypothetical)
	line3 := "00:00:00.001 [D] P-SpinelDrivTest-HELPER--: Set state callback: OK"
	result = setAlternativeOtLogMarker(line3)
	assert.Equal(t, "00:00:00.001 [D] P-SpinelDrivTest-HELPER--| Set state callback: OK", result)

	// Marker not found: unchanged
	line4 := "not a log line"
	result = setAlternativeOtLogMarker(line4)
	assert.Equal(t, line4, result)
}
