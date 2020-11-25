package txpool_provider

import (
	"context"

	"github.com/ledgerwatch/turbo-geth/rlp"
	"github.com/ledgerwatch/turbo-geth/turbo/txpool-provider/pb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type TxPoolControlServer struct {
	pb.UnimplementedTxpoolControlServer
}

func NewTxPoolControlServer() *TxPoolControlServer {
	return &TxPoolControlServer{}
}

func (c *TxPoolControlServer) AccountInfo(context.Context, *pb.AccountInfoRequest) (*pb.AccountInfoReply, error) {
	balance, _ := rlp.EncodeToBytes(100)
	nonce, _ := rlp.EncodeToBytes(42)

	return &pb.AccountInfoReply{
		Balance: balance,
		Nonce:   nonce,
	}, nil
}

func (c *TxPoolControlServer) BlockStream(*pb.BlockStreamRequest, pb.TxpoolControl_BlockStreamServer) error {
	return status.Errorf(codes.Unimplemented, "method BlockStream not implemented")
}

func (c *TxPoolControlServer) mustEmbedUnimplementedTxpoolControlServer() {}
