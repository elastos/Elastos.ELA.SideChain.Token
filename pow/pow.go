package pow

import (
	"math/big"
	"sort"

	bc "github.com/elastos/Elastos.ELA.SideChain.Token/blockchain"

	"github.com/elastos/Elastos.ELA.SideChain/pow"
	ucore "github.com/elastos/Elastos.ELA.SideChain/core"
	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
	"github.com/elastos/Elastos.ELA.Utility/common"
	"github.com/elastos/Elastos.ELA.SideChain/config"
	"github.com/elastos/Elastos.ELA.SideChain/protocol"
	"github.com/elastos/Elastos.ELA.SideChain/log"
	"github.com/elastos/Elastos.ELA.SideChain/servers"

)

func InitTokenPowService(localNode protocol.Noder) {
	servers.LocalPow = pow.NewPowService(localNode)
	servers.LocalPow.Init()
	servers.LocalPow.GenerateBlockTransactions = GenerateBlockTransactions
	log.Trace("pow Service Init succeed")
}

func GenerateBlockTransactions(msgBlock *ucore.Block, coinBaseTx *ucore.Transaction) {
	nextBlockHeight := blockchain.DefaultLedger.Blockchain.GetBestHeight() + 1
	totalTxsSize := coinBaseTx.GetSize()
	txCount := 1
	totalFee := common.Fixed64(0)
	var txsByFeeDesc pow.ByFeeDesc
	txsInPool := servers.LocalPow.LocalNode.GetTxsInPool()
	txsByFeeDesc = make([]*ucore.Transaction, 0, len(txsInPool))
	for _, v := range txsInPool {
		txsByFeeDesc = append(txsByFeeDesc, v)
	}
	sort.Sort(txsByFeeDesc)

	for _, tx := range txsByFeeDesc {
		totalTxsSize = totalTxsSize + tx.GetSize()
		if totalTxsSize > config.Parameters.MaxBlockSize {
			break
		}
		if txCount >= config.Parameters.MaxTxInBlock {
			break
		}

		if !blockchain.BlockValidator.IsFinalizedTransaction(tx, nextBlockHeight) {
			continue
		}

		fee := bc.TxFeeHelper.GetTxFee(tx, blockchain.DefaultLedger.Blockchain.AssetID)
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