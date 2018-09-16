package blockchain

import (
	"bytes"
	"errors"

	"github.com/elastos/Elastos.ELA.SideChain/config"
	"github.com/elastos/Elastos.ELA.SideChain/core"

	. "github.com/elastos/Elastos.ELA.Utility/common"
	"math/big"
	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
)

var TxFeeHelper *TxFeeHelperBase

type TxFeeHelperBase struct {
	GetTxFee         func(tx *core.Transaction, assetId Uint256) *big.Int
	GetTxFeeMap      func(tx *core.Transaction) (map[Uint256]*big.Int, error)
}

func InitTxFeeHelper() {
	TxFeeHelper = &TxFeeHelperBase{}
	TxFeeHelper.Init()
}

func (t *TxFeeHelperBase) Init() {
	t.GetTxFee = t.GetTxFeeImpl
	t.GetTxFeeMap = t.GetTxFeeMapImpl
}


func (t *TxFeeHelperBase)GetTxFeeImpl(tx *core.Transaction, assetId Uint256) *big.Int {
	feeMap, err := t.GetTxFeeMap(tx)
	if err != nil {
		return big.NewInt(0)
	}

	return feeMap[assetId]
}

func(t *TxFeeHelperBase) GetTxFeeMapImpl(tx *core.Transaction) (map[Uint256]*big.Int, error) {
	feeMap := make(map[Uint256]*big.Int)

	if tx.IsRechargeToSideChainTx() {
		depositPayload := tx.Payload.(*core.PayloadRechargeToSideChain)
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
						amount.Add(amount, big.NewInt(int64(float64(mcAmount)*config.Parameters.ExchangeRate)))
						feeMap[v.AssetID] = amount.Sub(amount, big.NewInt(int64(v.Value)))
					} else {
						value := big.NewInt(int64(float64(mcAmount) * config.Parameters.ExchangeRate))
						feeMap[v.AssetID] = value.Sub(value, big.NewInt(int64(v.Value)))
					}
				}
			}
		}

		return feeMap, nil
	}

	reference, err := blockchain.DefaultLedger.Store.GetTxReference(tx)
	if err != nil {
		return nil, err
	}

	var inputs = make(map[Uint256]big.Int)
	var outputs = make(map[Uint256]big.Int)
	for _, v := range reference {
		value := big.Int{}
		if v.AssetID.IsEqual(blockchain.DefaultLedger.Blockchain.AssetID) {
			value = *big.NewInt(int64(v.Value))
		} else {
			value = v.TokenValue
		}

		amount, ok := inputs[v.AssetID]
		if ok {
			inputs[v.AssetID] = *amount.Add(&amount, &value)
		} else {
			inputs[v.AssetID] = value
		}
	}

	for _, v := range tx.Outputs {
		value := big.Int{}
		if v.AssetID.IsEqual(blockchain.DefaultLedger.Blockchain.AssetID) {
			value = *big.NewInt(int64(v.Value))
		} else {
			value = v.TokenValue
		}

		amount, ok := outputs[v.AssetID]
		if ok {
			outputs[v.AssetID] = *amount.Add(&amount, &value)
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
				feeMap[inputAssetid] = value.Add(value, &inputValue)
			} else {
				val := big.NewInt(0)
				feeMap[inputAssetid] = val.Add(value, &inputValue)
			}
		}
	}
	return feeMap, nil
}
