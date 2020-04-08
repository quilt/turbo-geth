package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ledgerwatch/turbo-geth/common"
	"github.com/ledgerwatch/turbo-geth/eth"
	"github.com/ledgerwatch/turbo-geth/log"
	"github.com/ledgerwatch/turbo-geth/p2p"
)

type SimulatorProtocol struct {
	protocolVersion  uint32
	networkId        uint64
	genesisBlockHash common.Hash
}

func NewSimulatorProtocol() *SimulatorProtocol {
	return &SimulatorProtocol{}
}

func (sp *SimulatorProtocol) debugProtocolRun(ctx context.Context, peer *p2p.Peer, rw p2p.MsgReadWriter) error {
	block, err := json.Marshal(genesis())
	if err != nil {
		return err
	}

	err = p2p.Send(rw, eth.DebugSetGenesisMsg, block)
	if err != nil {
		return fmt.Errorf("failed to send DebugSetGenesisMsg message to peer: %w", err)
	}

	log.Info("eth set custom genesis.config")

	return nil
}
