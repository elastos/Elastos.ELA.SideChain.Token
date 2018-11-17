package mempool

import (
	"bytes"
	"errors"
	"math/big"
	"sort"

	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
	"github.com/elastos/Elastos.ELA.SideChain/config"
	"github.com/elastos/Elastos.ELA.SideChain/mempool"
	"github.com/elastos/Elastos.ELA.SideChain/pow"
	"github.com/elastos/Elastos.ELA.SideChain/types"

	. "github.com/elastos/Elastos.ELA.Utility/common"
	"github.com/elastos/Elastos.ELA/core"
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
		}),
		chainParams: cfg.ChainParams,
		chainStore:  cfg.ChainStore,
	}
}

func (t *FeeHelper) GetTxFee(tx *types.Transaction, assetId Uint256) *big.Int {
	feeMap, err := t.GetTxFeeMap(tx)
	if err != nil {
		return big.NewInt(0)
	}

	return feeMap[assetId]
}

func (t *FeeHelper) GetTxFeeMap(tx *types.Transaction) (map[Uint256]*big.Int, error) {
	feeMap := make(map[Uint256]*big.Int)

	if tx.IsRechargeToSideChainTx() {
		depositPayload := tx.Payload.(*types.PayloadRechargeToSideChain)
		mainChainTransaction := new(core.Transaction)
		reader := bytes.NewReader(depositPayload.MainChainTransaction)
		if err := mainChainTransaction.Deserialize(reader); err != nil {
			return nil, errors.New("GetTxFeeMap mainChainTransaction deserialize failed")
		}

		crossChainPayload := mainChainTransaction.Payload.(*core.PayloadTransferCrossChainAsset)

		for _, v := range tx.Outputs {
			for i := 0; i < len(crossChainPayload.CrossChainAddresses); i++ {
				targetAddress, err := v.ProgramHash.ToAddress()
				if err != nil {
					return nil, err
				}
				if targetAddress == crossChainPayload.CrossChainAddresses[i] {
					mcAmount := mainChainTransaction.Outputs[crossChainPayload.OutputIndexes[i]].Value

					amount, ok := feeMap[v.AssetID]
					if ok {
						amount.Add(amount, big.NewInt(int64(float64(mcAmount)*t.chainParams.ExchangeRate)))
						feeMap[v.AssetID] = amount.Sub(amount, big.NewInt(int64(v.Value)))
					} else {
						value := big.NewInt(int64(float64(mcAmount) * t.chainParams.ExchangeRate))
						feeMap[v.AssetID] = value.Sub(value, big.NewInt(int64(v.Value)))
					}
				}
			}
		}

		return feeMap, nil
	}

	reference, err := t.chainStore.GetTxReference(tx)
	if err != nil {
		return nil, err
	}

	var inputs = make(map[Uint256]big.Int)
	var outputs = make(map[Uint256]big.Int)
	for _, v := range reference {
		value := big.Int{}
		if v.AssetID.IsEqual(t.chainParams.ElaAssetId) {
			value = *big.NewInt(int64(v.Value))
		} else {
			value = v.TokenValue
		}

		amount, ok := inputs[v.AssetID]
		if ok {
			inputs[v.AssetID] = *new(big.Int).Add(&amount, &value)
		} else {
			inputs[v.AssetID] = value
		}
	}

	for _, v := range tx.Outputs {
		value := big.Int{}
		if v.AssetID.IsEqual(t.chainParams.ElaAssetId) {
			value = *big.NewInt(int64(v.Value))
		} else {
			value = v.TokenValue
		}

		amount, ok := outputs[v.AssetID]
		if ok {
			outputs[v.AssetID] = *new(big.Int).Add(&amount, &value)
		} else {
			outputs[v.AssetID] = value
		}
	}

	//calc the balance of input vs output
	for outputAssetid, outputValue := range outputs {
		if inputValue, ok := inputs[outputAssetid]; ok {
			feeMap[outputAssetid] = inputValue.Sub(&inputValue, &outputValue)
		} else {
			value, ok := feeMap[outputAssetid]
			if ok {
				feeMap[outputAssetid] = value.Sub(value, &outputValue)
			} else {
				val := big.NewInt(0)
				feeMap[outputAssetid] = val.Sub(val, &outputValue)
			}
		}
	}
	for inputAssetid, inputValue := range inputs {
		if _, exist := feeMap[inputAssetid]; !exist {
			value, ok := feeMap[inputAssetid]
			if ok {
				feeMap[inputAssetid] = new(big.Int).Add(value, &inputValue)
			} else {
				feeMap[inputAssetid] = new(big.Int).Add(value, &inputValue)
			}
		}
	}
	return feeMap, nil
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

		fee := t.GetTxFee(tx, t.chainParams.ElaAssetId)
		if fee.Cmp(big.NewInt(int64(tx.Fee))) != 0 {
			continue
		}
		msgBlock.Transactions = append(msgBlock.Transactions, tx)
		totalFee += Fixed64(fee.Int64())
		txCount++
	}

	reward := totalFee
	rewardFoundation := Fixed64(float64(reward) * 0.3)
	msgBlock.Transactions[0].Outputs[0].Value = rewardFoundation
	msgBlock.Transactions[0].Outputs[1].Value = Fixed64(reward) - rewardFoundation
}
