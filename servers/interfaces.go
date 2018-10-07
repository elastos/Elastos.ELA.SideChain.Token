package servers

import (
	"math/big"

	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
	"github.com/elastos/Elastos.ELA.SideChain/servers"
	"github.com/elastos/Elastos.ELA.SideChain/types"
	. "github.com/elastos/Elastos.ELA.Utility/common"
)

type HttpServiceExtend struct {
	*servers.HttpService
	chain *blockchain.BlockChain
}

func NewHttpService(cfg *servers.Config) *HttpServiceExtend {
	server := &HttpServiceExtend{
		HttpService: servers.NewHttpService(cfg),
		chain:       cfg.Chain,
	}
	return server
}

func GetPayloadInfo(p types.Payload) servers.PayloadInfo {
	switch object := p.(type) {
	case *types.PayloadCoinBase:
		obj := new(servers.CoinbaseInfo)
		obj.CoinbaseData = string(object.CoinbaseData)
		return obj
	case *types.PayloadRegisterAsset:
		obj := new(servers.RegisterAssetInfo)
		obj.Asset = object.Asset
		value := big.NewInt(int64(object.Amount))
		obj.Amount = value.Mul(value, big.NewInt(1000000000000000000)).String()
		obj.Controller = BytesToHexString(BytesReverse(object.Controller.Bytes()))
		return obj
	case *types.PayloadTransferCrossChainAsset:
		obj := new(servers.TransferCrossChainAssetInfo)
		obj.CrossChainAddresses = object.CrossChainAddresses
		obj.OutputIndexes = object.OutputIndexes
		obj.CrossChainAmounts = object.CrossChainAmounts
		return obj
	case *types.PayloadTransferAsset:
	case *types.PayloadRecord:
	case *types.PayloadRechargeToSideChain:
		obj := new(servers.RechargeToSideChainInfo)
		obj.MainChainTransaction = BytesToHexString(object.MainChainTransaction)
		obj.Proof = BytesToHexString(object.MerkleProof)
		return obj
	}
	return nil
}

func GetTransactionInfo(cfg *servers.Config, header *types.Header, tx *types.Transaction) *servers.TransactionInfo {
	inputs := make([]servers.InputInfo, len(tx.Inputs))
	for i, v := range tx.Inputs {
		inputs[i].TxID = servers.ToReversedString(v.Previous.TxID)
		inputs[i].VOut = v.Previous.Index
		inputs[i].Sequence = v.Sequence
	}

	outputs := make([]servers.OutputInfo, len(tx.Outputs))
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
			address = servers.DestroyAddress
		} else {
			address, _ = v.ProgramHash.ToAddress()
		}
		outputs[i].Address = address
		outputs[i].AssetID = servers.ToReversedString(v.AssetID)
		outputs[i].OutputLock = v.OutputLock
	}

	attributes := make([]servers.AttributeInfo, len(tx.Attributes))
	for i, v := range tx.Attributes {
		attributes[i].Usage = v.Usage
		attributes[i].Data = BytesToHexString(v.Data)
	}

	programs := make([]servers.ProgramInfo, len(tx.Programs))
	for i, v := range tx.Programs {
		programs[i].Code = BytesToHexString(v.Code)
		programs[i].Parameter = BytesToHexString(v.Parameter)
	}

	var txHash = tx.Hash()
	var txHashStr = servers.ToReversedString(txHash)
	var size = uint32(tx.GetSize())
	var blockHash string
	var confirmations uint32
	var time uint32
	var blockTime uint32
	if header != nil {
		confirmations = cfg.Chain.GetBestHeight() - header.Height + 1
		blockHash = servers.ToReversedString(header.Hash())
		time = header.Timestamp
		blockTime = header.Timestamp
	}

	return &servers.TransactionInfo{
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
		Payload:        cfg.GetPayloadInfo(tx.Payload),
		Attributes:     attributes,
		Programs:       programs,
	}
}
