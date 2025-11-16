// Copyright (c) 2020-2024, The OTNS Authors.
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

package web

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/openthread/ot-ns/logger"
	"github.com/openthread/ot-ns/progctx"
)

const (
	MainTab   = "visualize"
	EnergyTab = "energyViewer"
	StatsTab  = "statsViewer"

	defaultWebClientConnectTime = 200 * time.Millisecond
)

var (
	webParams struct {
		webSiteHost string
		webSitePort int
	}
)

func ConfigWeb(webSiteHost string, webSitePort int) {
	webParams.webSiteHost = webSiteHost
	webParams.webSitePort = webSitePort
	logger.Debugf("ConfigWeb: %+v", webParams)
}

func OpenWeb(ctx *progctx.ProgCtx, tabResourceName string) error {
	err := openWebBrowser(fmt.Sprintf("http://%s:%d/%s", webParams.webSiteHost, webParams.webSitePort, tabResourceName))
	if err != nil {
		return err
	}

	// give some time for new gRPC client to connect. If it connects later, also no problem.
	time.Sleep(defaultWebClientConnectTime)
	return nil
}

// openWebBrowser opens the specified URL in the default browser of the user.
func openWebBrowser(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}

	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}
