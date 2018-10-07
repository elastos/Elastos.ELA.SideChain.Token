package mempool

import (
	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
	"github.com/elastos/Elastos.ELA.SideChain/mempool"
	"github.com/elastos/Elastos.ELA.SideChain/spv"
	"github.com/elastos/Elastos.ELA.SideChain/types"
	. "github.com/elastos/Elastos.ELA.Utility/common"
)

type GetReference func(*types.Transaction) (map[*types.Input]*types.Output, error)

type Config struct {
	FoundationAddress Uint168
	AssetId           Uint256
	ExchangeRage      float64
	ChainStore        *blockchain.ChainStore
	SpvService        *spv.Service
	Validator         *mempool.Validator
	FeeHelper         *FeeHelper
}
