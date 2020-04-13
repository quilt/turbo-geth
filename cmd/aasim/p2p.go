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
	}

	pMap := map[string]p2p.Protocol{
		eth.ProtocolName: {
			Name:    eth.ProtocolName,
			Version: eth.ProtocolVersions[0],
			Length:  eth.ProtocolLengths[eth.ProtocolVersions[0]],
			Run: func(peer *p2p.Peer, rw p2p.MsgReadWriter) error {
				return sp.protocolRun(ctx, peer, rw)
			},
		},
		eth.DebugName: {
			Name:    eth.DebugName,
			Version: eth.DebugVersions[0],
			Length:  eth.DebugLengths[eth.DebugVersions[0]],
			Run: func(peer *p2p.Peer, rw p2p.MsgReadWriter) error {
				return sp.debugProtocolRun(ctx, peer, rw)
			},
		},
	}

	for _, protocolName := range protocols {
		p2pConfig.Protocols = append(p2pConfig.Protocols, pMap[protocolName])
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

func syncHandshake(sp *SimulatorProtocol, rw p2p.MsgReadWriter) error {
	err := p2p.Send(rw, eth.StatusMsg, &eth.StatusData{
		ProtocolVersion: sp.protocolVersion,
		NetworkID:       sp.networkID,
		TD:              sp.feeder.TotalDifficulty(),
		Head:            sp.feeder.Head().Hash(),
		Genesis:         sp.genesis,
		ForkID:          sp.feeder.ForkID(),
	})
	if err != nil {
		return fmt.Errorf("failed to send status message to peer: %w", err)
	}

	return nil
}

func readStatus(rw p2p.MsgReadWriter, status *eth.StatusData, sp *SimulatorProtocol) error {
	msg, err := rw.ReadMsg()
	if err != nil {
		return err
	}

	if msg.Code != eth.StatusMsg {
		return fmt.Errorf("first msg has code %x (!= %x)", msg.Code, eth.StatusMsg)
	}

	if msg.Size > eth.ProtocolMaxMsgSize {
		return fmt.Errorf("%v > %v", msg.Size, eth.ProtocolMaxMsgSize)
	}

	// Decode the handshake and make sure everything matches
	if err := msg.Decode(&status); err != nil {
		return fmt.Errorf("msg %v: %v", msg, err)
	}

	if status.NetworkID != sp.networkID {
		return fmt.Errorf("%d (!= %d)", status.NetworkID, sp.networkID)
	}

	if status.ProtocolVersion != sp.protocolVersion {
		return fmt.Errorf("%d (!= %d)", status.ProtocolVersion, sp.protocolVersion)
	}

	if status.Genesis != sp.genesis {
		return fmt.Errorf("%x (!= %x)", status.Genesis, genesis)
	}

	// if err := forkFilter(status.ForkID); err != nil {
	//         return errResp(ErrForkIDRejected, "%v", err)
	// }

	return nil
}
