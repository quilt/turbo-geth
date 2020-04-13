package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ledgerwatch/turbo-geth/common"
	"github.com/ledgerwatch/turbo-geth/core/types"
	"github.com/ledgerwatch/turbo-geth/eth"
	"github.com/ledgerwatch/turbo-geth/log"
	"github.com/ledgerwatch/turbo-geth/p2p"
	"github.com/ledgerwatch/turbo-geth/rlp"
)

type SimulatorProtocol struct {
	protocolVersion uint32
	networkID       uint64
	genesis         common.Hash
	feeder          BlockFeeder

	// Bitmap to remember which blocks (or just header if the blocks are empty) have been sent already
	blockMarkers []uint64
}

// Return true if the block has already been marked. If the block has not been marked, returns false and marks it
func (sp *SimulatorProtocol) markBlockSent(blockNumber uint) bool {
	lengthNeeded := (blockNumber+63)/64 + 1

	if lengthNeeded > uint(len(sp.blockMarkers)) {
		sp.blockMarkers = append(sp.blockMarkers, make([]uint64, lengthNeeded-uint(len(sp.blockMarkers)))...)
	}

	bitMask := (uint64(1) << (blockNumber & 63))
	result := (sp.blockMarkers[blockNumber/64] & bitMask) != 0
	sp.blockMarkers[blockNumber/64] |= bitMask

	return result
}

func NewSimulatorProtocol() *SimulatorProtocol {
	return &SimulatorProtocol{}
}

func (sp *SimulatorProtocol) protocolRun(ctx context.Context, p *p2p.Peer, rw p2p.MsgReadWriter) error {
	log.Info("Ethereum peer connected", "peer", p.Name())
	log.Debug("Protocol version", "version", sp.protocolVersion)

	err := syncHandshake(sp, rw)
	if err != nil {
		return fmt.Errorf("Handshake failed: %s", err)
	}

	sentBlocks := 0
	emptyBlocks := 0
	signaledHead := false
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Read the next message
		msg, err := rw.ReadMsg()
		if err != nil {
			return fmt.Errorf("failed to receive message from peer: %w", err)
		}

		switch {
		case msg.Code == eth.GetBlockHeadersMsg:
			if emptyBlocks, err = sp.handleGetBlockHeaderMsg(msg, rw, sp.feeder, emptyBlocks); err != nil {
				return err
			}
		case msg.Code == eth.GetBlockBodiesMsg:
			if sentBlocks, err = sp.handleGetBlockBodiesMsg(msg, rw, sp.feeder, sentBlocks); err != nil {
				return err
			}
		case msg.Code == eth.NewBlockHashesMsg:
			if signaledHead, err = sp.handleNewBlockHashesMsg(msg, rw); err != nil {
				return err
			}
		default:
			log.Trace("Next message", "msg", msg)
		}

		if signaledHead {
			break
		}
	}

	return nil
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

// hashOrNumber is a combined field for specifying an origin block.
type hashOrNumber struct {
	Hash   common.Hash // Block hash from which to retrieve headers (excludes Number)
	Number uint64      // Block hash from which to retrieve headers (excludes Hash)
}

// getBlockHeadersData represents a block header query.
type getBlockHeadersData struct {
	Origin  hashOrNumber // Block from which to retrieve headers
	Amount  uint64       // Maximum number of headers to retrieve
	Skip    uint64       // Blocks to skip between consecutive headers
	Reverse bool         // Query direction (false = rising towards latest, true = falling towards genesis)
}

// newBlockHashesData is the network packet for the block announcements.
type newBlockHashesData []struct {
	Hash   common.Hash // Hash of one particular block being announced
	Number uint64      // Number of one particular block being announced
}

// EncodeRLP is a specialized encoder for hashOrNumber to encode only one of the
// two contained union fields.
func (hn *hashOrNumber) EncodeRLP(w io.Writer) error {
	if hn.Hash == (common.Hash{}) {
		return rlp.Encode(w, hn.Number)
	}
	if hn.Number != 0 {
		return fmt.Errorf("both origin hash (%x) and number (%d) provided", hn.Hash, hn.Number)
	}
	return rlp.Encode(w, hn.Hash)
}

// DecodeRLP is a specialized decoder for hashOrNumber to decode the contents
// into either a block hash or a block number.
func (hn *hashOrNumber) DecodeRLP(s *rlp.Stream) error {
	_, size, _ := s.Kind()
	origin, err := s.Raw()
	if err == nil {
		switch {
		case size == 32:
			err = rlp.DecodeBytes(origin, &hn.Hash)
		case size <= 8:
			err = rlp.DecodeBytes(origin, &hn.Number)
		default:
			err = fmt.Errorf("invalid input size %d for origin", size)
		}
	}
	return err
}

func (sp *SimulatorProtocol) handleGetBlockHeaderMsg(msg p2p.Msg, rw p2p.MsgReadWriter, blockFeeder BlockFeeder, emptyBlocks int) (int, error) {
	newEmptyBlocks := emptyBlocks

	var query getBlockHeadersData
	if err := msg.Decode(&query); err != nil {
		return newEmptyBlocks, fmt.Errorf("failed to decode msg %v: %w", msg, err)
	}

	log.Trace("GetBlockHeadersMsg", "query", query)
	headers := []*types.Header{}
	if query.Origin.Hash == (common.Hash{}) && !query.Reverse {
		number := query.Origin.Number
		for i := 0; i < int(query.Amount); i++ {
			if header := blockFeeder.GetHeaderByNumber(number); header != nil {
				headers = append(headers, header)
				if header.TxHash == types.EmptyRootHash {
					if !sp.markBlockSent(uint(number)) {
						newEmptyBlocks++
					}
				}
			} else {
				//fmt.Printf("Could not find header with number %d\n", number)
			}
			number += query.Skip + 1
		}
	}
	if query.Origin.Hash != (common.Hash{}) && query.Amount == 1 && query.Skip == 0 && !query.Reverse {
		if header := blockFeeder.GetHeaderByHash(query.Origin.Hash); header != nil {
			log.Trace("Going to send header", "number", header.Number.Uint64())
			headers = append(headers, header)
		}
	}
	if err := p2p.Send(rw, eth.BlockHeadersMsg, headers); err != nil {
		return newEmptyBlocks, fmt.Errorf("failed to send headers: %w", err)
	}
	log.Info(fmt.Sprintf("Sent %d headers, empty blocks so far %d", len(headers), newEmptyBlocks))
	return newEmptyBlocks, nil
}

func (sp *SimulatorProtocol) handleGetBlockBodiesMsg(msg p2p.Msg, rw p2p.MsgReadWriter, blockFeeder BlockFeeder, sentBlocks int) (int, error) {
	newSentBlocks := sentBlocks
	msgStream := rlp.NewStream(msg.Payload, uint64(msg.Size))
	log.Trace("GetBlockBodiesMsg with size", "size", msg.Size)
	if _, err := msgStream.List(); err != nil {
		return newSentBlocks, err
	}
	// Gather blocks until the fetch or network limits is reached
	var (
		hash   common.Hash
		bodies []rlp.RawValue
	)
	for {
		// Retrieve the hash of the next block
		if err := msgStream.Decode(&hash); err == rlp.EOL {
			break
		} else if err != nil {
			return newSentBlocks, fmt.Errorf("failed to decode msg %v: %w", msg, err)
		}
		// Retrieve the requested block body, stopping if enough was found
		if block, err := blockFeeder.GetBlockByHash(hash); err != nil {
			return newSentBlocks, fmt.Errorf("failed to read block %w", err)
		} else if block != nil {
			if !sp.markBlockSent(uint(block.NumberU64())) {
				newSentBlocks++
			}

			body := block.Body()
			// Need to transform because our blocks also contain list of tx senders
			smallBody := types.SmallBody{Transactions: body.Transactions, Uncles: body.Uncles}
			data, err := rlp.EncodeToBytes(smallBody)
			if err != nil {
				return newSentBlocks, fmt.Errorf("failed to encode body: %w", err)
			}
			bodies = append(bodies, data)
		}
	}
	if err := p2p.Send(rw, eth.BlockBodiesMsg, bodies); err != nil {
		return newSentBlocks, err
	}
	log.Info("Sending bodies", "progress", newSentBlocks)

	return newSentBlocks, nil
}

func (sp *SimulatorProtocol) handleNewBlockHashesMsg(msg p2p.Msg, rw p2p.MsgReadWriter) (bool, error) {
	var blockHashMsg newBlockHashesData
	if err := msg.Decode(&blockHashMsg); err != nil {
		return false, fmt.Errorf("failed to decode msg %v: %w", msg, err)
	}
	log.Trace("NewBlockHashesMsg", "query", blockHashMsg)
	signaledHead := false
	for _, bh := range blockHashMsg {
		if bh.Number == sp.feeder.Head().NumberU64() {
			signaledHead = true
			break
		}
	}
	return signaledHead, nil
}
