package pow

import (
	mp "github.com/elastos/Elastos.ELA.SideChain.Token/mempool"

	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
	"github.com/elastos/Elastos.ELA.SideChain/mempool"
	"github.com/elastos/Elastos.ELA.SideChain/pow"
	"github.com/elastos/Elastos.ELA.SideChain/types"
	"github.com/elastos/Elastos.ELA.Utility/common"
	"github.com/elastos/Elastos.ELA.Utility/p2p/server"
)

type Config struct {
	Foundation    common.Uint168
	MinerAddr     string
	MinerInfo     string
	LimitBits     uint32
	MaxBlockSize  int
	MaxTxPerBlock int
	Server        server.IServer
	Chain         *blockchain.BlockChain
	TxMemPool     *mempool.TxPool
	TxFeeHelper   *mp.FeeHelper

	CreateCoinBaseTx          func(cfg *pow.Config, nextBlockHeight uint32, addr string) (*types.Transaction, error)
	GenerateBlock             func(cfg *pow.Config) (*types.Block, error)
	GenerateBlockTransactions func(cfg *Config, msgBlock *types.Block, coinBaseTx *types.Transaction)
}

func NewService(cfg *Config) *pow.Service {
	service := pow.NewService(&pow.Config{
		Foundation:    cfg.Foundation,
		MinerAddr:     cfg.MinerAddr,
		MinerInfo:     cfg.MinerInfo,
		LimitBits:     cfg.LimitBits,
		MaxBlockSize:  cfg.MaxBlockSize,
		MaxTxPerBlock: cfg.MaxTxPerBlock,
		Server:        cfg.Server,
		Chain:         cfg.Chain,
		TxMemPool:     cfg.TxMemPool,
	})

	return service
}
