package main

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"math/rand"
	"time"

	"github.com/ledgerwatch/turbo-geth/common"
	"github.com/ledgerwatch/turbo-geth/consensus/ethash"
	"github.com/ledgerwatch/turbo-geth/core"
	"github.com/ledgerwatch/turbo-geth/core/forkid"
	"github.com/ledgerwatch/turbo-geth/core/types"
	"github.com/ledgerwatch/turbo-geth/core/vm"
	"github.com/ledgerwatch/turbo-geth/crypto"
	"github.com/ledgerwatch/turbo-geth/ethdb"
	"github.com/ledgerwatch/turbo-geth/log"
)

type BlockFeeder interface {
	Head() *types.Block
	TotalDifficulty() *big.Int
	ForkID() forkid.ID
	Genesis() *types.Block

	GetHeaderByHash(hash common.Hash) *types.Header
	GetHeaderByNumber(number uint64) *types.Header
	GetBlockByHash(hash common.Hash) (*types.Block, error)
	GetBlockByNumber(number uint64) (*types.Block, error)
	GetTotalDifficultyByNumber(number uint64) *big.Int
}

type BlockGenerator struct {
	// input               *os.File
	genesis         *types.Block
	coinbaseKey     *ecdsa.PrivateKey
	blocks          []*types.Block
	blockByHash     map[common.Hash]*types.Block
	blockByNumber   map[uint64]*types.Block
	headersByHash   map[common.Hash]*types.Header
	headersByNumber map[uint64]*types.Header
	tdByNumber      map[uint64]*big.Int
	head            *types.Block
	totalDifficulty *big.Int
	forkId          forkid.ID
}

func (bg *BlockGenerator) GetHeaderByHash(hash common.Hash) *types.Header {
	return bg.headersByHash[hash]
}

func (bg *BlockGenerator) Genesis() *types.Block {
	return bg.genesis
}

func (bg *BlockGenerator) GetHeaderByNumber(number uint64) *types.Header {
	return bg.headersByNumber[number]
}

func (bg *BlockGenerator) GetTdByNumber(number uint64) *big.Int {
	return bg.tdByNumber[number]
}

func (bg *BlockGenerator) GetBlockByHash(hash common.Hash) (*types.Block, error) {
	if block, ok := bg.blockByHash[hash]; ok {
		return block, nil
	}

	return nil, nil
}

func (bg *BlockGenerator) GetBlockByNumber(number uint64) (*types.Block, error) {
	if uint64(len(bg.blocks)) >= number {
		return nil, fmt.Errorf("Invalid block number")
	}

	return bg.blocks[number], nil
}
func (bg *BlockGenerator) GetTotalDifficultyByNumber(number uint64) *big.Int {
	return bg.tdByNumber[number]
}

func (bg *BlockGenerator) TotalDifficulty() *big.Int {
	return bg.totalDifficulty
}

func (bg *BlockGenerator) Head() *types.Block {
	return bg.head
}

func (bg *BlockGenerator) ForkID() forkid.ID {
	return bg.forkId
}

func NewBlockGenerator(ctx context.Context, initialHeight int) (*BlockGenerator, error) {
	r := rand.New(rand.NewSource(4589489854))
	db, genesis, extra, engine := ethdb.NewMemDatabase(), genesis(), []byte("BlockGenerator"), ethash.NewFullFaker()
	genesisBlock := genesis.MustCommit(db)

	coinbaseKey, err2 := crypto.HexToECDSA("ad0f3019b6b8634c080b574f3d8a47ef975f0e4b9f63e82893e9a7bb59c2d609")
	if err2 != nil {
		return nil, err2
	}

	coinbase := crypto.PubkeyToAddress(coinbaseKey.PublicKey)

	blockGen := func(i int, gen *core.BlockGen) {
		makeGenBlock(db, genesis, extra, coinbaseKey, false, 0, 0, r)(coinbase, i, gen)
	}

	parent := genesisBlock
	blocks, _ := core.GenerateChain(ctx, genesis.Config, parent, engine, db, 10, blockGen)

	bg := &BlockGenerator{
		genesis:         genesisBlock,
		coinbaseKey:     coinbaseKey,
		blockByHash:     make(map[common.Hash]*types.Block),
		blockByNumber:   make(map[uint64]*types.Block),
		headersByHash:   make(map[common.Hash]*types.Header),
		headersByNumber: make(map[uint64]*types.Header),
		tdByNumber:      make(map[uint64]*big.Int),
	}

	bg.headersByHash[genesisBlock.Header().Hash()] = genesisBlock.Header()
	bg.headersByNumber[0] = genesisBlock.Header()

	///
	td := new(big.Int)
	for _, block := range blocks {
		log.Info("Saving block", "number", block.Number(), "hash", block.Hash(), "difficulty", block.Difficulty(), "age", common.PrettyAge(time.Unix(int64(block.Time()), 0)))

		header := block.Header()
		hash := header.Hash()
		bg.headersByHash[hash] = header
		bg.headersByNumber[block.NumberU64()] = header
		bg.blockByHash[hash] = block
		bg.blockByNumber[block.NumberU64()] = block
		td = new(big.Int).Add(td, block.Difficulty())
		bg.tdByNumber[block.NumberU64()] = td
		parent = block
	}

	bg.head = parent
	bg.totalDifficulty = td
	////

	blockchain, err := core.NewBlockChain(db, nil, genesis.Config, ethash.NewFullFaker(), vm.Config{}, nil)
	if err != nil {
		return nil, err
	}

	bg.forkId = forkid.NewID(blockchain)

	return bg, nil
}

func makeGenBlock(db ethdb.Database,
	genesis *core.Genesis,
	extra []byte,
	coinbaseKey *ecdsa.PrivateKey,
	isFork bool,
	forkBase, forkHeight uint64,
	r *rand.Rand,
) func(coinbase common.Address, i int, gen *core.BlockGen) {
	return func(coinbase common.Address, i int, gen *core.BlockGen) {
		gen.SetExtra(extra)
	}
}
