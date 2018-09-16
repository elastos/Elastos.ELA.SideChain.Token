package blockchain

import (
	"errors"

	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
	ucore "github.com/elastos/Elastos.ELA.SideChain/core"
	"math/big"
)

func InitBlockValidator() {
	blockchain.BlockValidator = &blockchain.BlockValidateBase{}
	blockchain.BlockValidator.Init()
	blockchain.BlockValidator.PowCheckTransactionsFee = PowCheckTransactionsFeeImpl
}

func PowCheckTransactionsFeeImpl(block *ucore.Block) error {
	transactions := block.Transactions
	var rewardInCoinbase = big.NewInt(0)
	var totalTxFee = big.NewInt(0)
	for index, tx := range transactions {
		// The first transaction in a block must be a coinbase.
		if index == 0 {
			if !tx.IsCoinBaseTx() {
				return errors.New("[PowCheckBlockSanity] first transaction in block is not a coinbase")
			}
			// Calculate reward in coinbase
			for _, output := range tx.Outputs {
				rewardInCoinbase.Add(rewardInCoinbase, big.NewInt(int64(output.Value)))
			}
			continue
		}

		// A block must not have more than one coinbase.
		if tx.IsCoinBaseTx() {
			return errors.New("[PowCheckBlockSanity] block contains second coinbase")
		}

		// Calculate transaction fee
		totalTxFee.Add(totalTxFee, TxFeeHelper.GetTxFee(tx, blockchain.DefaultLedger.Blockchain.AssetID))
	}

	// Reward in coinbase must match total transaction fee
	if rewardInCoinbase.Cmp(totalTxFee) != 0 {
		return errors.New("[PowCheckBlockSanity] reward amount in coinbase not correct")
	}

	return nil
}