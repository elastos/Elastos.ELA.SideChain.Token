package pow

import (
	"math/big"
	"sort"

	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
	"github.com/elastos/Elastos.ELA.SideChain/pow"
	"github.com/elastos/Elastos.ELA.SideChain/types"
	"github.com/elastos/Elastos.ELA.Utility/common"
)

type Service struct {
	*pow.Service

	cfg *Config
}

func NewPowHelper(cfg *Config) *pow.Service {
	s := &Service{
		Service: pow.NewService(&pow.Config{
			Foundation:    cfg.Foundation,
			MinerAddr:     cfg.MinerAddr,
			MinerInfo:     cfg.MinerInfo,
			LimitBits:     cfg.LimitBits,
			MaxBlockSize:  cfg.MaxBlockSize,
			MaxTxPerBlock: cfg.MaxTxPerBlock,
			Server:        cfg.Server,
			Chain:         cfg.Chain,
			TxMemPool:     cfg.TxMemPool,
		}),
		cfg: cfg,
	}
	return s.Service
}

func GenerateBlockTransactions(cfg *Config, msgBlock *types.Block, coinBaseTx *types.Transaction) {
	nextBlockHeight := cfg.Chain.GetBestHeight() + 1
	totalTxsSize := coinBaseTx.GetSize()
	txCount := 1
	totalFee := common.Fixed64(0)
	var txsByFeeDesc pow.ByFeeDesc
	txsInPool := cfg.TxMemPool.GetTxsInPool()
	txsByFeeDesc = make([]*types.Transaction, 0, len(txsInPool))
	for _, v := range txsInPool {
		txsByFeeDesc = append(txsByFeeDesc, v)
	}
	sort.Sort(txsByFeeDesc)

	for _, tx := range txsByFeeDesc {
		totalTxsSize = totalTxsSize + tx.GetSize()
		if totalTxsSize > cfg.MaxBlockSize {
			break
		}
		if txCount >= cfg.MaxTxPerBlock {
			break
		}

		if err := blockchain.CheckTransactionFinalize(tx, nextBlockHeight); err != nil {
			continue
		}

		fee := cfg.TxFeeHelper.GetTxFee(tx, cfg.Chain.AssetID)
		if fee.Cmp(big.NewInt(int64(tx.Fee))) != 0 {
			continue
		}
		msgBlock.Transactions = append(msgBlock.Transactions, tx)
		totalFee += common.Fixed64(fee.Int64())
		txCount++
	}

	reward := totalFee
	rewardFoundation := common.Fixed64(float64(reward) * 0.3)
	msgBlock.Transactions[0].Outputs[0].Value = rewardFoundation
	msgBlock.Transactions[0].Outputs[1].Value = common.Fixed64(reward) - rewardFoundation
}
