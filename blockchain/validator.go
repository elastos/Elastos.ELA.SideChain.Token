package blockchain

import (
	"errors"
	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
	"github.com/elastos/Elastos.ELA.Utility/common"
)

type validator struct {
	cfg *Config
}

func NewValidator(chain *blockchain.BlockChain, cfg *Config) *blockchain.Validator {
	val := blockchain.NewValidator(chain)
	val.RegisterFunc(blockchain.ValidateFuncNames.CheckTransactionsFee,
		(&validator{cfg: cfg}).checkTransactionsFee)
	return val
}

func (v *validator) checkTransactionsFee(params ...interface{}) (err error) {
	block := blockchain.AssertBlock(params[0])

	transactions := block.Transactions
	var rewardInCoinbase = common.Fixed64(0)
	var totalTxFee = common.Fixed64(0)
	for index, tx := range transactions {
		// The first transaction in a block must be a coinbase.
		if index == 0 {
			if !tx.IsCoinBaseTx() {
				return errors.New("[PowCheckBlockSanity] first transaction in block is not a coinbase")
			}
			// Calculate reward in coinbase
			for _, output := range tx.Outputs {
				rewardInCoinbase += output.Value
			}
			continue
		}

		// A block must not have more than one coinbase.
		if tx.IsCoinBaseTx() {
			return errors.New("[PowCheckBlockSanity] block contains second coinbase")
		}

		// Calculate transaction fee
		totalTxFee += v.cfg.GetTxFee(tx)
	}

	// Reward in coinbase must match total transaction fee
	if rewardInCoinbase != totalTxFee {
		return errors.New("[PowCheckBlockSanity] reward amount in coinbase not correct")
	}

	return nil
}
