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

package visualize_grpc

import (
	"context"
	"time"

	"google.golang.org/grpc"

	"net"

	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"github.com/openthread/ot-ns/logger"
	"github.com/openthread/ot-ns/visualize/grpc/pb"
)

type visualizeStreamType int

const (
	meshTopologyVizType visualizeStreamType = 1
	nodeStatsVizType    visualizeStreamType = 2
)

type grpcServer struct {
	vis                *grpcVisualizer
	server             *grpc.Server
	webServer          *grpcweb.WrappedGrpcServer
	address            string
	visualizingStreams map[*grpcStream]struct{}
	energyStreams      map[*grpcEnergyStream]struct{}
	grpcClientAdded    chan string
}

func (gs *grpcServer) Visualize(req *pb.VisualizeRequest, stream pb.VisualizeGrpcService_VisualizeServer) error {
	return gs.runVisualizeStream(meshTopologyVizType, stream, req.String())
}

func (gs *grpcServer) NodeStats(req *pb.NodeStatsRequest, stream pb.VisualizeGrpcService_NodeStatsServer) error {
	return gs.runVisualizeStream(nodeStatsVizType, stream, req.String())
}

func (gs *grpcServer) runVisualizeStream(vizType visualizeStreamType, stream pb.VisualizeGrpcService_VisualizeServer,
	reqId string) error {
	var err error
	contextDone := stream.Context().Done()
	heartbeatEvent := &pb.VisualizeEvent{
		Type: &pb.VisualizeEvent_Heartbeat{Heartbeat: &pb.HeartbeatEvent{}},
	}
	var heartbeatTicker *time.Ticker

	gstream := newGrpcStream(vizType, stream)
	logger.Debugf("New gRPC visualize request (type %d) received.", vizType)

	gs.vis.Lock()
	err = gs.prepareStream(gstream)
	if err != nil {
		gs.vis.Unlock()
		goto exit
	}

	gs.visualizingStreams[gstream] = struct{}{}
	// if web.OpenWeb goroutine is waiting for a new client, then notify it.
	select {
	case gs.grpcClientAdded <- reqId:
		break
	default:
		break
	}
	gs.vis.Unlock()

	defer gs.disposeStream(gstream)

	heartbeatTicker = time.NewTicker(time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-heartbeatTicker.C:
			err = stream.Send(heartbeatEvent)
			if err != nil {
				goto exit
			}
		case <-contextDone:
			err = stream.Context().Err()
			goto exit
		}
	}

exit:
	logger.Debugf("Visualize stream exit: %v", err)
	return err
}

func (gs *grpcServer) Energy(req *pb.EnergyRequest, stream pb.VisualizeGrpcService_EnergyServer) error {
	var err error
	contextDone := stream.Context().Done()

	//TODO: do we need a heartbeat and a idle checker here too?

	gstream := newGrpcEnergyStream(stream)
	logger.Debugf("New energy report request got.")

	gs.energyStreams[gstream] = struct{}{}
	defer gs.disposeEnergyStream(gstream)

	energyHist := gs.vis.energyAnalyser.GetNetworkEnergyHistory()
	energyHistByNodes := gs.vis.energyAnalyser.GetEnergyHistoryByNodes()
	for i := 0; i < len(energyHistByNodes); i++ {
		gs.vis.UpdateNodesEnergy(energyHistByNodes[i], energyHist[i].Timestamp, (i+1) == len(energyHistByNodes))
	}

	//Wait for the first event
	<-contextDone
	err = stream.Context().Err()

	logger.Debugf("energy report stream exit: %v", err)
	return err
}

func (gs *grpcServer) Command(ctx context.Context, req *pb.CommandRequest) (*pb.CommandResponse, error) {
	output, err := gs.vis.simctrl.Command(req.Command)
	return &pb.CommandResponse{
		Output: output,
	}, err
}

func (gs *grpcServer) Run() error {
	lis, err := net.Listen("tcp", gs.address)
	if err != nil {
		return err
	}
	logger.Infof("gRPC visualizer server serving on %s ...", lis.Addr())
	return gs.server.Serve(lis)
}

func (gs *grpcServer) SendEvent(event *pb.VisualizeEvent) {
	for stream := range gs.visualizingStreams {
		if stream.acceptsEvent(event) {
			_ = stream.Send(event)
		}
	}
}

func (gs *grpcServer) SendEnergyEvent(event *pb.EnergyEvent) {
	for stream := range gs.energyStreams {
		_ = stream.Send(event)
	}
}

func (gs *grpcServer) stop() {
	for stream := range gs.visualizingStreams {
		stream.close()
	}
	gs.server.GracefulStop()
}

func (gs *grpcServer) disposeStream(stream *grpcStream) {
	gs.vis.Lock()
	delete(gs.visualizingStreams, stream)
	gs.vis.Unlock()
	stream.close()
}

func (gs *grpcServer) disposeEnergyStream(stream *grpcEnergyStream) {
	gs.vis.Lock()
	delete(gs.energyStreams, stream)
	gs.vis.Unlock()
	stream.close()
}

func (gs *grpcServer) prepareStream(stream *grpcStream) error {
	return gs.vis.prepareStream(stream)
}

func newGrpcServer(vis *grpcVisualizer, chanNewClientNotifier chan string) *grpcServer {
	server := grpc.NewServer(grpc.ReadBufferSize(1024*8), grpc.WriteBufferSize(1024*1024*1))
	wrappedServer := grpcweb.WrapServer(server)
	gs := &grpcServer{
		vis:                vis,
		server:             server,
		webServer:          wrappedServer,
		address:            "localhost:9000", // FIXME
		visualizingStreams: map[*grpcStream]struct{}{},
		energyStreams:      map[*grpcEnergyStream]struct{}{},
		grpcClientAdded:    chanNewClientNotifier,
	}
	pb.RegisterVisualizeGrpcServiceServer(server, gs)
	return gs
}
