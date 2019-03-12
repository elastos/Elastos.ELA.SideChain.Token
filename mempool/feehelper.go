package mempool

import (
	"sort"

	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
	"github.com/elastos/Elastos.ELA.SideChain/config"
	"github.com/elastos/Elastos.ELA.SideChain/mempool"
	"github.com/elastos/Elastos.ELA.SideChain/pow"
	"github.com/elastos/Elastos.ELA.SideChain/types"

	. "github.com/elastos/Elastos.ELA/common"
)

type FeeHelper struct {
	*mempool.FeeHelper
	chainParams *config.Params
	chainStore  *blockchain.ChainStore
}

func NewFeeHelper(cfg *Config) *FeeHelper {
	return &FeeHelper{
		FeeHelper: mempool.NewFeeHelper(&mempool.Config{
			ChainParams: cfg.ChainParams,
			ChainStore:  cfg.ChainStore,
			SpvService:  cfg.SpvService,
		}),
		chainParams: cfg.ChainParams,
		chainStore:  cfg.ChainStore,
	}
}

func (t *FeeHelper) GenerateBlockTransactions(cfg *pow.Config, msgBlock *types.Block, coinBaseTx *types.Transaction) {
	nextBlockHeight := cfg.Chain.GetBestHeight() + 1
	totalTxsSize := coinBaseTx.GetSize()
	txCount := 1
	totalFee := Fixed64(0)
	var txsByFeeDesc pow.ByFeeDesc
	txsInPool := cfg.TxMemPool.GetTxsInPool()
	txsByFeeDesc = make([]*types.Transaction, 0, len(txsInPool))
	for _, v := range txsInPool {
		txsByFeeDesc = append(txsByFeeDesc, v)
	}
	sort.Sort(txsByFeeDesc)

	for _, tx := range txsByFeeDesc {
		totalTxsSize = totalTxsSize + tx.GetSize()
		if totalTxsSize > types.MaxBlockSize {
			break
		}
		if txCount >= types.MaxTxPerBlock {
			break
		}

		if err := blockchain.CheckTransactionFinalize(tx, nextBlockHeight); err != nil {
			continue
		}

		fee, err := t.GetTxFee(tx, t.chainParams.ElaAssetId)
		if err != nil {
			continue
		}
		msgBlock.Transactions = append(msgBlock.Transactions, tx)
		totalFee += fee
		txCount++
	}

	reward := totalFee
	rewardFoundation := Fixed64(float64(reward) * 0.3)
	msgBlock.Transactions[0].Outputs[0].Value = rewardFoundation
	msgBlock.Transactions[0].Outputs[1].Value = Fixed64(reward) - rewardFoundation
}
