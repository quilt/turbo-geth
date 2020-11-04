package control

import (
	"context"
	"fmt"
	"sync"

	"github.com/golang/protobuf/ptypes/empty"
	proto_core "github.com/ledgerwatch/turbo-geth/cmd/headers/core"
	proto_sentry "github.com/ledgerwatch/turbo-geth/cmd/headers/sentry"
	"github.com/ledgerwatch/turbo-geth/eth"
	"github.com/ledgerwatch/turbo-geth/log"
	"github.com/ledgerwatch/turbo-geth/rlp"
	"github.com/ledgerwatch/turbo-geth/turbo/stages/headerdownload"
	"google.golang.org/grpc"
)

type ControlServer struct {
	proto_core.UnimplementedControlServer
	lock          sync.Mutex
	sentryClient  proto_sentry.SentryClient
	requestWakeUp chan struct{}
}

func NewControlServer(sentryClient proto_sentry.SentryClient) (*ControlServer, error) {
	return &ControlServer{sentryClient: sentryClient, requestWakeUp: make(chan struct{})}, nil
}

func (cs *ControlServer) ForwardInboundMessage(ctx context.Context, inreq *proto_core.InboundMessage) (*empty.Empty, error) {
	defer func() {
		select {
		case cs.requestWakeUp <- struct{}{}:
		default:
		}
	}()
	log.Info("new msg", "id", inreq.Id)
	switch inreq.Id {
	default:
		return nil, fmt.Errorf("not implemented for message Id: %s", inreq.Id)
	}
}

func (cs *ControlServer) GetStatus(context.Context, *empty.Empty) (*proto_core.StatusData, error) {
	return nil, nil
}

func (cs *ControlServer) sendRequests(ctx context.Context, reqs []*headerdownload.HeaderRequest) {
	for _, req := range reqs {
		log.Debug(fmt.Sprintf("Sending header request {hash: %x, height: %d, length: %d}", req.Hash, req.Number, req.Length))
		bytes, err := rlp.EncodeToBytes(&eth.GetBlockHeadersData{
			Amount:  uint64(req.Length),
			Reverse: true,
			Skip:    0,
			Origin:  eth.HashOrNumber{Hash: req.Hash},
		})
		if err != nil {
			log.Error("Could not encode header request", "err", err)
			continue
		}
		outreq := proto_sentry.SendMessageByMinBlockRequest{
			MinBlock: req.Number,
			Data: &proto_sentry.OutboundMessageData{
				Id:   proto_sentry.OutboundMessageId_GetBlockHeaders,
				Data: bytes,
			},
		}
		_, err = cs.sentryClient.SendMessageByMinBlock(ctx, &outreq, &grpc.EmptyCallOption{})
		if err != nil {
			log.Error("Could not send header request", "err", err)
			continue
		}
	}
}

func (cs *ControlServer) Loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-cs.requestWakeUp:
			log.Info("Woken up by the incoming request")
		}
	}
}
