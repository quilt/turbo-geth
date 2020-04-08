package main

import (
	"context"
	"fmt"
	"io/ioutil"

	"github.com/ledgerwatch/turbo-geth/crypto"
	"github.com/ledgerwatch/turbo-geth/eth"
	"github.com/ledgerwatch/turbo-geth/log"
	"github.com/ledgerwatch/turbo-geth/p2p"
	"github.com/ledgerwatch/turbo-geth/p2p/enode"
)

func makeP2PServer(ctx context.Context, sp *SimulatorProtocol, protocols []string) *p2p.Server {
	serverKey, err := crypto.GenerateKey()
	if err != nil {
		panic(fmt.Sprintf("Failed to generate server key: %v", err))
	}

	p2pConfig := p2p.Config{
		PrivateKey: serverKey,
		Name:       "geth tester",
		Logger:     log.New(),
		MaxPeers:   1,
		Protocols: []p2p.Protocol{
			{
				Name:    eth.DebugName,
				Version: eth.DebugVersions[0],
				Length:  eth.DebugLengths[eth.DebugVersions[0]],
				Run: func(peer *p2p.Peer, rw p2p.MsgReadWriter) error {
					return sp.debugProtocolRun(ctx, peer, rw)
				},
			},
		},
	}

	return &p2p.Server{Config: p2pConfig}
}

func getTargetAddr() (*enode.Node, error) {
	addr, err := ioutil.ReadFile(p2p.EnodeAddressFileName)
	if err != nil {
		return nil, err
	}

	nodeToConnect, err := enode.ParseV4(string(addr))
	if err != nil {
		return nil, fmt.Errorf("could not parse the node info: %w", err)
	}

	log.Info("Parsed node: %s, IP: %s\n", nodeToConnect, nodeToConnect.IP())

	return nodeToConnect, nil
}
