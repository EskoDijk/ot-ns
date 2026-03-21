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
