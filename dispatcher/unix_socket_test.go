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

package dispatcher

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// chdirTemp changes the working directory to a new temporary dir dir/sub for the duration of the test.
// (t.Chdir() requires Go 1.24.)
func chdirTemp(t *testing.T, sub string) string {
	dir := filepath.Join(t.TempDir(), sub)
	require.NoError(t, os.MkdirAll(dir, 0700))
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(oldWd) })
	return dir
}

func TestNewUnixSocket(t *testing.T) {
	// a working dir deeper than the Unix socket path limit: the relative socket path still works.
	dir := chdirTemp(t, strings.Repeat("d", 120))

	ln, path, err := newUnixSocket(3)
	require.NoError(t, err)
	assert.Equal(t, "3_otns.sock", path)

	fi, err := os.Lstat(filepath.Join(dir, "3_otns.sock"))
	require.NoError(t, err)
	assert.Equal(t, os.ModeSocket, fi.Mode().Type())

	conn, err := net.Dial("unix", path)
	require.NoError(t, err)
	_ = conn.Close()

	require.NoError(t, ln.Close())
	assert.NoFileExists(t, path)
}

func TestNewUnixSocketInUse(t *testing.T) {
	chdirTemp(t, "otns")

	ln, path, err := newUnixSocket(1)
	require.NoError(t, err)
	go func() { // accept and drop connections, like the dispatcher does for a non-node client.
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	// another OTNS instance with the same simulation ID must not take over the socket.
	_, _, err = newUnixSocket(1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "simulation ID 1 is already in use")
	fi, err := os.Lstat(path)
	require.NoError(t, err)
	assert.Equal(t, os.ModeSocket, fi.Mode().Type())

	// a different simulation ID can be used next to it.
	ln2, _, err := newUnixSocket(2)
	require.NoError(t, err)
	_ = ln2.Close()
	_ = ln.Close()
}

func TestNewUnixSocketReplacesStaleSocket(t *testing.T) {
	chdirTemp(t, "otns")

	// a socket file left behind by an OTNS instance that was killed.
	stale, err := net.ListenUnix("unix", &net.UnixAddr{Name: "0_otns.sock", Net: "unix"})
	require.NoError(t, err)
	stale.SetUnlinkOnClose(false)
	require.NoError(t, stale.Close())
	require.FileExists(t, "0_otns.sock")

	ln, _, err := newUnixSocket(0)
	require.NoError(t, err)
	_ = ln.Close()
}
