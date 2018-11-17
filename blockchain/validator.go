package blockchain

import (
	"errors"
	"math/big"

	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
)

type validator struct {
	cfg *Config
}

func NewValidator(chain *blockchain.BlockChain, cfg *Config) *blockchain.Validator {
	val := blockchain.NewValidator(chain)
	val.RegisterFunc(blockchain.ValidateFuncNames.CheckTransactionsFee,
		(&validator{cfg:cfg}).checkTransactionsFee)
	return val
}

func (v *validator) checkTransactionsFee(params ...interface{}) (err error) {
	block := blockchain.AssertBlock(params[0])

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
		totalTxFee.Add(totalTxFee, v.cfg.GetTxFee(tx, v.cfg.ChainParams.ElaAssetId))
	}

	// Reward in coinbase must match total transaction fee
	if rewardInCoinbase.Cmp(totalTxFee) != 0 {
		return errors.New("[PowCheckBlockSanity] reward amount in coinbase not correct")
	}

	return nil
}
