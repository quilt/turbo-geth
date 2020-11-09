package control

import (
	"context"
	"fmt"
	"sync"

	"github.com/golang/protobuf/ptypes/empty"
	proto_core "github.com/ledgerwatch/turbo-geth/cmd/headers/core"
	proto_sentry "github.com/ledgerwatch/turbo-geth/cmd/headers/sentry"
	"github.com/ledgerwatch/turbo-geth/common"
	"github.com/ledgerwatch/turbo-geth/core"
	"github.com/ledgerwatch/turbo-geth/core/types"
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
	pool          *core.TxPool
	requestWakeUp chan struct{}
}

func NewControlServer(sentryClient proto_sentry.SentryClient, pool *core.TxPool) (*ControlServer, error) {
	return &ControlServer{sentryClient: sentryClient, requestWakeUp: make(chan struct{}), pool: pool}, nil
}

func (cs *ControlServer) ForwardInboundMessage(ctx context.Context, inreq *proto_core.InboundMessage) (*empty.Empty, error) {
	defer func() {
		select {
		case cs.requestWakeUp <- struct{}{}:
		default:
		}
	}()

	switch inreq.Id {
	case proto_core.InboundMessageId_NewPooledTransactionHashes:
		return cs.newPooledTransactionHashes(ctx, inreq)
	case proto_core.InboundMessageId_PooledTransactions:
	case proto_core.InboundMessageId_NewTransactions:
		return cs.newTransactions(ctx, inreq)
	default:
		return nil, fmt.Errorf("not implemented for message Id: %s", inreq.Id)
	}

	return nil, nil
}

func (cs *ControlServer) newPooledTransactionHashes(ctx context.Context, inreq *proto_core.InboundMessage) (*empty.Empty, error) {
	var hashes []common.Hash
	if err := rlp.DecodeBytes(inreq.Data, &hashes); err != nil {
		return nil, errResp(eth.ErrDecode, "decode NewPooledTransactionHashesMsg: %v", err)
	}
	log.Info("New Pooled Transaction Hashes", "count", len(hashes))
	outreq := proto_sentry.SendMessageByIdRequest{
		PeerId: inreq.PeerId,
		Data: &proto_sentry.OutboundMessageData{
			Id:   proto_sentry.OutboundMessageId_GetPooledTransactions,
			Data: inreq.Data,
		},
	}
	_, err := cs.sentryClient.SendMessageById(ctx, &outreq, &grpc.EmptyCallOption{})
	if err != nil {
		return nil, fmt.Errorf("send get pooled transactions request: %v", err)
	}
	return &empty.Empty{}, nil
}

func (cs *ControlServer) newTransactions(ctx context.Context, inreq *proto_core.InboundMessage) (*empty.Empty, error) {
	var txs []types.Transaction
	if err := rlp.DecodeBytes(inreq.Data, &txs); err != nil {
		return nil, errResp(eth.ErrDecode, "decode NewTransactionsMsg: %v", err)
	}
	log.Info("New transactions", "count", len(txs))
	return &empty.Empty{}, nil
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

func errResp(code int, format string, v ...interface{}) error {
	return fmt.Errorf("%v - %v", code, fmt.Sprintf(format, v...))
}
