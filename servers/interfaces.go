package servers

import (
	"math/big"

	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
	"github.com/elastos/Elastos.ELA.SideChain/service"
	"github.com/elastos/Elastos.ELA.SideChain/types"
	. "github.com/elastos/Elastos.ELA.Utility/common"
)

type HttpServiceExtend struct {
	*service.HttpService
	chain *blockchain.BlockChain
}

func NewHttpService(cfg *service.Config) *HttpServiceExtend {
	server := &HttpServiceExtend{
		HttpService: service.NewHttpService(cfg),
		chain:       cfg.Chain,
	}
	return server
}

func GetPayloadInfo(p types.Payload, pVersion byte) service.PayloadInfo {
	switch object := p.(type) {
	case *types.PayloadCoinBase:
		obj := new(service.CoinbaseInfo)
		obj.CoinbaseData = string(object.CoinbaseData)
		return obj
	case *types.PayloadRegisterAsset:
		obj := new(service.RegisterAssetInfo)
		obj.Asset = object.Asset
		value := big.NewInt(int64(object.Amount))
		obj.Amount = value.Mul(value, big.NewInt(1000000000000000000)).String()
		obj.Controller = BytesToHexString(BytesReverse(object.Controller.Bytes()))
		return obj
	case *types.PayloadTransferCrossChainAsset:
		obj := new(service.TransferCrossChainAssetInfo)
		obj.CrossChainAssets = make([]service.CrossChainAssetInfo, 0)
		for i := 0; i < len(object.CrossChainAddresses); i++ {
			assetInfo := service.CrossChainAssetInfo{
				CrossChainAddress: object.CrossChainAddresses[i],
				OutputIndex:       object.OutputIndexes[i],
				CrossChainAmount:  object.CrossChainAmounts[i].String(),
			}
			obj.CrossChainAssets = append(obj.CrossChainAssets, assetInfo)
		}
		return obj
	case *types.PayloadTransferAsset:
	case *types.PayloadRecord:
	case *types.PayloadRechargeToSideChain:
		if pVersion == types.RechargeToSideChainPayloadVersion0 {
			obj := new(service.RechargeToSideChainInfoV0)
			obj.MainChainTransaction = BytesToHexString(object.MainChainTransaction)
			obj.Proof = BytesToHexString(object.MerkleProof)
			return obj
		} else if pVersion == types.RechargeToSideChainPayloadVersion1 {
			obj := new(service.RechargeToSideChainInfoV1)
			obj.MainChainTransactionHash = service.ToReversedString(object.MainChainTransactionHash)
			return obj
		}
	}
	return nil
}

func GetTransactionInfo(cfg *service.Config, header *types.Header, tx *types.Transaction) *service.TransactionInfo {
	inputs := make([]service.InputInfo, len(tx.Inputs))
	for i, v := range tx.Inputs {
		inputs[i].TxID = service.ToReversedString(v.Previous.TxID)
		inputs[i].VOut = v.Previous.Index
		inputs[i].Sequence = v.Sequence
	}

	outputs := make([]service.OutputInfo, len(tx.Outputs))
	for i, v := range tx.Outputs {
		if v.AssetID.IsEqual(types.GetSystemAssetId()) {
			outputs[i].Value = v.Value.String()
		} else {
			outputs[i].Value = v.TokenValue.String()
		}
		outputs[i].Index = uint32(i)
		var address string
		destroyHash := Uint168{}
		if v.ProgramHash == destroyHash {
			address = service.DestroyAddress
		} else {
			address, _ = v.ProgramHash.ToAddress()
		}
		outputs[i].Address = address
		outputs[i].AssetID = service.ToReversedString(v.AssetID)
		outputs[i].OutputLock = v.OutputLock
	}

	attributes := make([]service.AttributeInfo, len(tx.Attributes))
	for i, v := range tx.Attributes {
		attributes[i].Usage = v.Usage
		attributes[i].Data = BytesToHexString(v.Data)
	}

	programs := make([]service.ProgramInfo, len(tx.Programs))
	for i, v := range tx.Programs {
		programs[i].Code = BytesToHexString(v.Code)
		programs[i].Parameter = BytesToHexString(v.Parameter)
	}

	var txHash = tx.Hash()
	var txHashStr = service.ToReversedString(txHash)
	var size = uint32(tx.GetSize())
	var blockHash string
	var confirmations uint32
	var time uint32
	var blockTime uint32
	if header != nil {
		confirmations = cfg.Chain.GetBestHeight() - header.Height + 1
		blockHash = service.ToReversedString(header.Hash())
		time = header.Timestamp
		blockTime = header.Timestamp
	}

	return &service.TransactionInfo{
		TxId:           txHashStr,
		Hash:           txHashStr,
		Size:           size,
		VSize:          size,
		Version:        0x00,
		LockTime:       tx.LockTime,
		Inputs:         inputs,
		Outputs:        outputs,
		BlockHash:      blockHash,
		Confirmations:  confirmations,
		Time:           time,
		BlockTime:      blockTime,
		TxType:         tx.TxType,
		PayloadVersion: tx.PayloadVersion,
		Payload:        cfg.GetPayloadInfo(tx.Payload, tx.PayloadVersion),
		Attributes:     attributes,
		Programs:       programs,
	}
}
