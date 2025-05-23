// Copyright (c) 2020-2025, The OTNS Authors.
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

package otns_main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/pkg/errors"

	"github.com/openthread/ot-ns/cli"
	"github.com/openthread/ot-ns/dispatcher"
	"github.com/openthread/ot-ns/logger"
	"github.com/openthread/ot-ns/pcap"
	"github.com/openthread/ot-ns/prng"
	"github.com/openthread/ot-ns/progctx"
	"github.com/openthread/ot-ns/simulation"
	. "github.com/openthread/ot-ns/types"
	visualizeGrpc "github.com/openthread/ot-ns/visualize/grpc"
	visualizeMulti "github.com/openthread/ot-ns/visualize/multi"
	visualizeStatslog "github.com/openthread/ot-ns/visualize/statslog"
	"github.com/openthread/ot-ns/web"
	webSite "github.com/openthread/ot-ns/web/site"
)

type MainArgs struct {
	Speed          string
	OtCliPath      string
	OtCliMtdPath   string
	InitScriptName string
	AutoGo         bool
	ReadOnly       bool
	LogLevel       string
	LogFileLevel   string
	WatchLevel     string
	OpenWeb        bool
	Realtime       bool
	ListenAddr     string
	DispatcherHost string
	DispatcherPort int
	DumpPackets    bool
	PcapType       string
	NoReplay       bool
	RandomSeed     int64
	PhyTxStats     bool
}

var (
	args MainArgs
)

func parseArgs() {
	defaultOtCli := os.Getenv("OTNS_OT_CLI")
	defaultOtCliMtd := os.Getenv("OTNS_OT_CLI_MTD")
	if defaultOtCli == "" && defaultOtCliMtd == "" {
		defaultOtCli = simulation.DefaultExecutableConfig.Ftd
		defaultOtCliMtd = simulation.DefaultExecutableConfig.Mtd
	} else if defaultOtCliMtd == "" {
		defaultOtCliMtd = defaultOtCli // use same CLI for MTD, by default. FTD can simulate being MTD.
	} else if defaultOtCli == "" {
		defaultOtCli = simulation.DefaultExecutableConfig.Ftd // only use custom MTD, not FTD.
	}

	flag.StringVar(&args.Speed, "speed", "1", "set simulation speed")
	flag.StringVar(&args.OtCliPath, "ot-cli", defaultOtCli, "specify the OT CLI executable, for FTD and also for MTD if not configured otherwise.")
	flag.StringVar(&args.OtCliMtdPath, "ot-cli-mtd", defaultOtCliMtd, "specify the OT CLI MTD executable, separately from FTD executable.")
	flag.StringVar(&args.InitScriptName, "ot-script", "", "specify the OT node init script filename, to use for init of new nodes. By default an internal script is used. Use 'none' for no script.")
	flag.BoolVar(&args.AutoGo, "autogo", true, "auto go (runs the simulation at given speed, without issuing 'go' commands.)")
	flag.BoolVar(&args.ReadOnly, "readonly", false, "readonly simulation can not be manipulated")
	flag.StringVar(&args.LogLevel, "log", "warn", "set OTNS display logging level: trace, debug, info, warn, error.")
	flag.StringVar(&args.LogFileLevel, "logfile", "debug", "set OTNS + node file logging level: trace, debug, info, warn, error, off.")
	flag.StringVar(&args.WatchLevel, "watch", "off", "set default watch (display) level for new nodes: trace, debug, info, note, warn, error, off.")
	flag.BoolVar(&args.OpenWeb, "web", true, "open web visualization")
	flag.BoolVar(&args.Realtime, "realtime", false, "use real-time mode (forced speed=1 and autogo)")
	flag.StringVar(&args.ListenAddr, "listen", fmt.Sprintf("localhost:%d", InitialDispatcherPort), "specify TCP/UDP host and port base value for web-GUI/RPC. Recommended ports are 9000, 9010, 9020, etc.")
	flag.BoolVar(&args.DumpPackets, "dump-packets", false, "dump packets")
	flag.StringVar(&args.PcapType, "pcap", pcap.FrameTypeWpanStr, "PCAP file type: 'off', 'wpan', or 'wpan-tap'. PCAP is saved to file 'current.pcap'.")
	flag.BoolVar(&args.NoReplay, "no-replay", false, "do not generate Replay file (named \"otns_?.replay\")")
	flag.Int64Var(&args.RandomSeed, "seed", 0, "set specific random-seed value (for reproducability)")
	flag.BoolVar(&args.PhyTxStats, "phy-tx-stats", false, "generate PHY Tx statistics CSV file")
	flag.Parse()
}

func parseListenAddr() (int, error) {
	var err error

	notifyInvalidListenAddr := func() {
		err = fmt.Errorf("invalid listen address: %s (port must be larger than or equal to 9000 and must be a multiple of 10", args.ListenAddr)
	}

	subs := strings.Split(args.ListenAddr, ":")
	if len(subs) != 2 {
		notifyInvalidListenAddr()
	}

	args.DispatcherHost = subs[0]
	if args.DispatcherPort, err = strconv.Atoi(subs[1]); err != nil {
		notifyInvalidListenAddr()
	}

	if args.DispatcherPort < InitialDispatcherPort || args.DispatcherPort%10 != 0 {
		notifyInvalidListenAddr()
	}

	simId := (args.DispatcherPort - InitialDispatcherPort) / 10
	return simId, err
}

func Main(ctx *progctx.ProgCtx, cliOptions *cli.CliOptions) {
	handleSignals(ctx)
	parseArgs()
	simId, err := parseListenAddr()
	logger.FatalIfError(err)

	prng.Init(args.RandomSeed)
	sim, err := createSimulation(simId, ctx)
	logger.FatalIfError(err)

	visGrpcServerAddr := fmt.Sprintf("%s:%d", args.DispatcherHost, args.DispatcherPort-1)

	replayFn := ""
	if !args.NoReplay {
		replayFn = fmt.Sprintf("otns_%d.replay", simId)
	}

	chanGrpcClientNotifier := make(chan string, 1)

	vis := visualizeMulti.NewMultiVisualizer(
		visualizeGrpc.NewGrpcVisualizer(visGrpcServerAddr, replayFn, chanGrpcClientNotifier),
		visualizeStatslog.NewStatslogVisualizer(sim.GetConfig().OutputDir, simId, visualizeStatslog.NodeStatsType),
	)
	if args.PhyTxStats {
		vis.AddVisualizer(visualizeStatslog.NewStatslogVisualizer(sim.GetConfig().OutputDir, simId, visualizeStatslog.TxBytesStatsType))
		vis.AddVisualizer(visualizeStatslog.NewStatslogVisualizer(sim.GetConfig().OutputDir, simId, visualizeStatslog.ChanSampleCountStatsType))
	}

	ctx.WaitAdd("webserver", 1)
	go func() {
		defer ctx.WaitDone("webserver")
		siteAddr := fmt.Sprintf("%s:%d", args.DispatcherHost, args.DispatcherPort-3)
		err := webSite.Serve(siteAddr) // blocks until webSite.StopServe() called
		if err != nil && ctx.Err() == nil {
			logger.Errorf("webserver stopped unexpectedly: %+v, OTNS-Web won't be available!", err)
		}
	}()
	<-webSite.Started

	rt := cli.NewCmdRunner(ctx, sim)
	vis.Init()
	sim.SetVisualizer(vis)

	ctx.WaitAdd("cli", 1)
	go func() {
		defer ctx.WaitDone("cli")
		err := cli.Cli.Run(rt, cliOptions)
		ctx.Cancel(errors.Wrapf(err, "cli-exit"))
	}()
	<-cli.Cli.Started
	logger.SetStdoutCallback(cli.Cli)

	logger.Infof("PRNG root seed: %d", prng.GetRootSeed())

	ctx.WaitAdd("simulation", 1)
	go func() {
		defer ctx.WaitDone("simulation")
		sim.Run()
	}()
	<-sim.Started

	web.ConfigWeb(args.DispatcherHost, args.DispatcherPort-2, args.DispatcherPort-1, args.DispatcherPort-3)
	logger.Debugf("open web: %v", args.OpenWeb)
	if args.OpenWeb {
		sim.PostAsync(func() {
			err := web.OpenWeb(ctx, web.MainTab)
			if err != nil {
				logger.Error(err)
			}
		})
	}

	ctx.WaitAdd("autogo", 1)
	go sim.AutoGoRoutine(ctx, sim)

	vis.Run() // visualize must run in the main thread
	ctx.Cancel("main")

	logger.Debugf("waiting for OTNS to stop gracefully ...")
	cli.Cli.Stop()
	webSite.StopServe()
	ctx.Wait()
}

func handleSignals(ctx *progctx.ProgCtx) {
	c := make(chan os.Signal, 1)
	sigHandlerReady := make(chan struct{})
	signal.Notify(c, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGINT, syscall.SIGHUP)
	signal.Ignore(syscall.SIGALRM)

	ctx.WaitAdd("handleSignals", 1)
	go func() {
		defer ctx.WaitDone("handleSignals")
		defer logger.Debugf("handleSignals exit.")

		done := ctx.Done()
		close(sigHandlerReady)
		for {
			select {
			case sig := <-c:
				signal.Reset()
				logger.Infof("Unix signal received: %v", sig)
				ctx.Cancel("signal-" + sig.String())
				return
			case <-done:
				return
			}
		}
	}()
	<-sigHandlerReady
}

func createSimulation(simId int, ctx *progctx.ProgCtx) (*simulation.Simulation, error) {
	var speed float64
	var err error

	simcfg := simulation.DefaultConfig()

	simcfg.LogLevel, err = logger.ParseLevelString(args.LogLevel)
	if err != nil {
		return nil, err
	}
	logger.SetLevel(simcfg.LogLevel)
	simcfg.LogFileLevel, err = logger.ParseLevelString(args.LogFileLevel)
	if err != nil {
		return nil, err
	}
	if args.LogFileLevel == logger.NoneLevelString || args.LogFileLevel == logger.OffLevelString {
		simcfg.NewNodeConfig.NodeLogFile = false
	}
	simcfg.ExeConfig.Ftd = args.OtCliPath
	simcfg.ExeConfig.Mtd = args.OtCliMtdPath
	args.Speed = strings.ToLower(args.Speed)
	if args.Speed == "max" {
		speed = dispatcher.MaxSimulateSpeed
	} else {
		speed, err = strconv.ParseFloat(args.Speed, 64)
		if err != nil {
			return nil, err
		}
	}
	simcfg.Speed = speed
	simcfg.ReadOnly = args.ReadOnly
	simcfg.Realtime = args.Realtime
	simcfg.DispatcherHost = args.DispatcherHost
	simcfg.DispatcherPort = args.DispatcherPort
	simcfg.DumpPackets = args.DumpPackets
	simcfg.AutoGo = args.AutoGo
	simcfg.Id = simId
	if len(args.InitScriptName) > 0 {
		if args.InitScriptName == "none" {
			simcfg.NewNodeScripts = &simulation.YamlScriptConfig{}
		} else {
			simcfg.NewNodeScripts, err = simulation.ReadNodeScript(args.InitScriptName)
			if err != nil {
				return nil, err
			}
		}
	}
	simcfg.RandomSeed = prng.GetRootSeed()

	dispatcherCfg := dispatcher.DefaultConfig()
	dispatcherCfg.SimulationId = simcfg.Id
	dispatcherCfg.PcapEnabled = args.PcapType != pcap.FrameTypeOffStr
	dispatcherCfg.PcapFrameType = pcap.ParseFrameTypeStr(args.PcapType)
	if dispatcherCfg.PcapFrameType == pcap.FrameTypeUnknown {
		logger.Fatalf("Unknown PCAP frame type '%s', use -h flag for an overview.", args.PcapType)
	}
	dispatcherCfg.DefaultWatchLevel = args.WatchLevel
	watchLevel, err := logger.ParseLevelString(args.WatchLevel)
	if err != nil {
		return nil, err
	}
	dispatcherCfg.DefaultWatchOn = watchLevel != logger.OffLevel
	dispatcherCfg.PhyTxStats = args.PhyTxStats

	sim, err := simulation.NewSimulation(ctx, simcfg, dispatcherCfg)
	return sim, err
}
