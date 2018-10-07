package blockchain

import (
	"math/big"

	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
	"github.com/elastos/Elastos.ELA.SideChain/types"
	. "github.com/elastos/Elastos.ELA.Utility/common"
)

type Config struct {
	FoundationAddress Uint168
	ChainStore        *blockchain.ChainStore
	AssetId           Uint256
	PowLimit          *big.Int
	MaxOrphanBlocks   int
	MinMemoryNodes    uint32
	CheckTxSanity     func(*types.Transaction) error
	CheckTxContext    func(*types.Transaction) error
	GetTxFee          func(tx *types.Transaction, assetId Uint256) *big.Int
}
