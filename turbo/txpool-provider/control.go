package txpoolprovider

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ledgerwatch/turbo-geth/common"
	"github.com/ledgerwatch/turbo-geth/ethdb"
	"github.com/ledgerwatch/turbo-geth/rlp"
	"github.com/ledgerwatch/turbo-geth/rpc"
	"github.com/ledgerwatch/turbo-geth/turbo/adapter"
	"github.com/ledgerwatch/turbo-geth/turbo/rpchelper"
	"github.com/ledgerwatch/turbo-geth/turbo/txpool-provider/pb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type TxPoolControlServer struct {
	pb.UnimplementedTxpoolControlServer
	kv ethdb.KV
}

func NewTxPoolControlServer(kv ethdb.KV) *TxPoolControlServer {
	return &TxPoolControlServer{kv: kv}
}

func (c *TxPoolControlServer) AccountInfo(ctx context.Context, request *pb.AccountInfoRequest) (*pb.AccountInfoReply, error) {
	blockHash := common.BytesToHash(request.BlockHash)
	address := common.BytesToAddress(request.Account)

	rawdb := ethdb.NewObjectDatabase(c.kv)
	blockNumber, _, err := rpchelper.GetBlockNumber(rpc.BlockNumberOrHash{BlockHash: &blockHash, RequireCanonical: true}, rawdb)
	if err != nil {
		return nil, err
	}

	nonce := uint64(0)
	balance := common.Big0
	getter := func(tx ethdb.Tx) error {
		reader := adapter.NewStateReader(tx, blockNumber)
		account, err := reader.ReadAccountData(address)
		if account != nil {
			nonce = account.Nonce
			balance = account.Balance.ToBig()
		}

		return err
	}
	if err := c.kv.View(ctx, getter); err != nil {
		return nil, fmt.Errorf("cant get a balance for account %q for block %v", address.String(), blockNumber)
	}

	return buildAccountInfoReply(nonce, balance)
}

func (c *TxPoolControlServer) BlockStream(*pb.BlockStreamRequest, pb.TxpoolControl_BlockStreamServer) error {
	return status.Errorf(codes.Unimplemented, "method BlockStream not implemented")
}

func (c *TxPoolControlServer) mustEmbedUnimplementedTxpoolControlServer() {}

func buildAccountInfoReply(nonce uint64, balance *big.Int) (*pb.AccountInfoReply, error) {
	balanceBytes, err := rlp.EncodeToBytes(balance)
	if err != nil {
		return nil, err
	}
	nonceBytes, err := rlp.EncodeToBytes(nonce)
	if err != nil {
		return nil, err
	}

	return &pb.AccountInfoReply{
		Balance: balanceBytes,
		Nonce:   nonceBytes,
	}, nil
}
