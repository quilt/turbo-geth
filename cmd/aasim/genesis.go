package main

import (
	"math/big"

	"github.com/ledgerwatch/turbo-geth/common"
	"github.com/ledgerwatch/turbo-geth/core"
)

func genesis() *core.Genesis {
	genesis := core.DefaultGenesisBlock()

	genesis.Config.HomesteadBlock = big.NewInt(0)
	genesis.Config.DAOForkBlock = nil
	genesis.Config.DAOForkSupport = true
	genesis.Config.EIP150Block = big.NewInt(0)
	genesis.Config.EIP150Hash = common.HexToHash("0x41941023680923e0fe4d74a34bdac8141f2540e3ae90623718e47d66d1ca4a2d")
	genesis.Config.EIP155Block = big.NewInt(10)
	genesis.Config.EIP158Block = big.NewInt(10)
	genesis.Config.ByzantiumBlock = big.NewInt(17)
	genesis.Config.ConstantinopleBlock = big.NewInt(18)
	genesis.Config.PetersburgBlock = big.NewInt(19)
	genesis.Config.IstanbulBlock = big.NewInt(20)
	genesis.Config.MuirGlacierBlock = big.NewInt(21)

	return genesis
}
