// Copyright (c) 2022-2025, The OTNS Authors.
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

package web_site

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"sync"

	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"strings"

	"github.com/openthread/ot-ns/logger"
	"google.golang.org/grpc"
)

var httpServer *http.Server = nil
var debugServer *http.Server = nil
var canServe bool = true
var httpServerMutex sync.Mutex
var Started = make(chan struct{})

func Serve(listenAddr string, wrappedGrpcServer *grpcweb.WrappedGrpcServer, nativeGrpcServer *grpc.Server) error {
	defer logger.Debugf("webserver exit.")

	assetDir := os.Getenv("HOME")
	if assetDir == "" {
		assetDir = "/tmp"
	}
	assetDir = filepath.Join(assetDir, ".otns-web")
	if err := os.MkdirAll(assetDir, 0755); err != nil {
		return err
	}

	for _, name := range AssetNames() {
		data, err := Asset(name)
		if err != nil {
			return err
		}

		fp := filepath.Join(assetDir, name)
		if err := os.MkdirAll(filepath.Dir(fp), 0755); err != nil {
			return err
		}

		f, err := os.OpenFile(fp, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0755)
		if err != nil {
			return err
		}

		if _, err := f.Write(data); err != nil {
			return err
		}
	}

	mux := http.NewServeMux()

	templates := template.Must(template.ParseGlob(filepath.Join(assetDir, "templates", "*.html")))

	fs := http.FileServer(http.Dir(filepath.Join(assetDir, "static")))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	mux.HandleFunc("/visualize", func(writer http.ResponseWriter, request *http.Request) {
		logger.Debugf("visualizing new client")
		err := templates.ExecuteTemplate(writer, "visualize.html", nil)
		if err != nil {
			writer.WriteHeader(501)
		}
	})

	mux.HandleFunc("/energyViewer", func(writer http.ResponseWriter, request *http.Request) {
		logger.Debugf("energyViewer visualizing")
		err := templates.ExecuteTemplate(writer, "energyViewer.html", nil)
		if err != nil {
			writer.WriteHeader(501)
		}
	})

	mux.HandleFunc("/statsViewer", func(writer http.ResponseWriter, request *http.Request) {
		logger.Debugf("statsViewer visualizing")
		err := templates.ExecuteTemplate(writer, "statsViewer.html", nil)
		if err != nil {
			writer.WriteHeader(501)
		}
	})

	httpServerMutex.Lock()
	if !canServe {
		httpServer = nil
		httpServerMutex.Unlock()
		close(Started)
		return http.ErrServerClosed
	}

	// Create a multiplexing handler that checks the request's content type.
	multiplexHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use the IsGrpcWebRequest helper to determine if it's a gRPC-Web HTTP/1.1 request.
		if wrappedGrpcServer != nil && (wrappedGrpcServer.IsGrpcWebRequest(r) || wrappedGrpcServer.IsAcceptableGrpcCorsRequest(r)) {
			wrappedGrpcServer.ServeHTTP(w, r)
			return
		}
		// Check if it's a gRPC-Web HTTP/2 request.
		if nativeGrpcServer != nil && r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc") {
			nativeGrpcServer.ServeHTTP(w, r)
			return
		}
		// Or, fall back to the standard HTTP mux.
		mux.ServeHTTP(w, r)
	})

	httpServer = &http.Server{
		Addr:    listenAddr,
		Handler: h2c.NewHandler(multiplexHandler, &http2.Server{}),
	}
	logger.Infof("OTNS webserver now serving on %s ...", listenAddr)
	defer logger.Tracef("webserver: httpServer.ListenAndServe() done")
	httpServerMutex.Unlock()
	close(Started)
	return httpServer.ListenAndServe()
}

// ServeDebugPort starts the Go pprof debug server on the specified port. Function does not block.
func ServeDebugPort(httpDebugPort int) {
	httpServerMutex.Lock()
	defer httpServerMutex.Unlock()

	if !canServe {
		debugServer = nil
		return
	}
	debugServer = &http.Server{Addr: fmt.Sprintf("localhost:%d", httpDebugPort)}
	go func() {
		err := debugServer.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Errorf("pprof debug server failed: %v", err)
		}
	}()
}

func StopServe() {
	logger.Debugf("requesting OTNS webserver to exit ...")
	httpServerMutex.Lock()
	defer httpServerMutex.Unlock()

	if httpServer != nil {
		_ = httpServer.Close()
	}
	if debugServer != nil {
		_ = debugServer.Close()
	}
	canServe = false // prevent serving again in same execution.
}
