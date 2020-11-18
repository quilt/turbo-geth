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
	"github.com/ledgerwatch/turbo-geth/eth"
	"github.com/ledgerwatch/turbo-geth/log"
	"github.com/ledgerwatch/turbo-geth/rlp"
	"google.golang.org/grpc"
)

type ControlServer struct {
	proto_core.UnimplementedControlServer
	lock         sync.Mutex
	sentryClient proto_sentry.SentryClient
	pool         *core.TxPool
}

func NewControlServer(sentryClient proto_sentry.SentryClient, pool *core.TxPool) (*ControlServer, error) {
	return &ControlServer{sentryClient: sentryClient, pool: pool}, nil
}

func (cs *ControlServer) ForwardInboundMessage(ctx context.Context, inreq *proto_core.InboundMessage) (*empty.Empty, error) {
	switch inreq.Id {
	case proto_core.InboundMessageId_NewPooledTransactionHashes:
		// log.Info(fmt.Sprintf("[%x] NewPooledTransactionHashes", inreq.PeerId[:]))
		return cs.newPooledTransactionHashes(ctx, inreq)
	case proto_core.InboundMessageId_PooledTransactions:
		log.Info(fmt.Sprintf("[%x] PooledTransactions", inreq.PeerId[:]))
		return cs.newTransactions(ctx, inreq)
	case proto_core.InboundMessageId_NewTransactions:
		log.Info(fmt.Sprintf("[%x] NewTransactions", inreq.PeerId[:]))
		return cs.newTransactions(ctx, inreq)
	default:
		return nil, fmt.Errorf("not implemented for message Id: %s", inreq.Id)
	}
}

func (cs *ControlServer) newPooledTransactionHashes(ctx context.Context, inreq *proto_core.InboundMessage) (*empty.Empty, error) {
	var hashes []common.Hash
	if err := rlp.DecodeBytes(inreq.Data, &hashes); err != nil {
		return nil, errResp(eth.ErrDecode, "decode NewPooledTransactionHashesMsg: %v", err)
	}
	outreq := proto_sentry.SendMessageByIdRequest{
		PeerId: inreq.PeerId,
		Data: &proto_sentry.OutboundMessageData{
			Id:   proto_sentry.OutboundMessageId_GetPooledTransactions,
			Data: inreq.Data,
		},
	}
	sentPeers, err := cs.sentryClient.SendMessageById(ctx, &outreq, &grpc.EmptyCallOption{})
	if err != nil {
		return nil, fmt.Errorf("send get pooled transactions request: %v", err)
	}
	if len(sentPeers.Peers) == 0 {
		return nil, fmt.Errorf("couldn't find peer to send to")
	}
	return &empty.Empty{}, nil
}

func (cs *ControlServer) newTransactions(ctx context.Context, inreq *proto_core.InboundMessage) (*empty.Empty, error) {
	// var txs []types.Transaction
	// if err := rlp.DecodeBytes(inreq.Data, &txs); err != nil {
	//         return nil, errResp(eth.ErrDecode, "decode NewTransactionsMsg: %v", err)
	// }
	return &empty.Empty{}, nil
}

func (cs *ControlServer) GetStatus(context.Context, *empty.Empty) (*proto_core.StatusData, error) {
	return nil, nil
}

func (cs *ControlServer) Loop(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	}
}

func errResp(code int, format string, v ...interface{}) error {
	return fmt.Errorf("%v - %v", code, fmt.Sprintf(format, v...))
}
