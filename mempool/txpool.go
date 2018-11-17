package mempool

import (
	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
	"github.com/elastos/Elastos.ELA.SideChain/config"
	"github.com/elastos/Elastos.ELA.SideChain/mempool"
	"github.com/elastos/Elastos.ELA.SideChain/spv"
	"github.com/elastos/Elastos.ELA.SideChain/types"
)

type GetReference func(*types.Transaction) (map[*types.Input]*types.Output, error)

type Config struct {
	ChainParams *config.Params
	ChainStore  *blockchain.ChainStore
	SpvService  *spv.Service
	Validator   *mempool.Validator
	FeeHelper   *FeeHelper
}
