package servers

import (
	"math/big"
	"bytes"

	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
	"github.com/elastos/Elastos.ELA.SideChain/errors"
	ucore "github.com/elastos/Elastos.ELA.SideChain/core"
	"github.com/elastos/Elastos.ELA.SideChain/servers"
	. "github.com/elastos/Elastos.ELA.Utility/common"
)

func InitHttpServers() {
	servers.HttpServers = &servers.HttpServersBase{}
	servers.HttpServers.Init()
	servers.HttpServers.GetPayloadInfo = GetPayloadInfo
	servers.HttpServers.GetTransactionInfo = GetTransactionInfo
	servers.HttpServers.SendRawTransaction = SendRawTransaction
}

func GetPayloadInfo(p ucore.Payload) servers.PayloadInfo {
	switch object := p.(type) {
	case *ucore.PayloadCoinBase:
		obj := new(servers.CoinbaseInfo)
		obj.CoinbaseData = string(object.CoinbaseData)
		return obj
	case *ucore.PayloadRegisterAsset:
		obj := new(servers.RegisterAssetInfo)
		obj.Asset = object.Asset
		value := big.NewInt(int64(object.Amount))
		obj.Amount = value.Mul(value, big.NewInt(1000000000000000000)).String()
		obj.Controller = BytesToHexString(BytesReverse(object.Controller.Bytes()))
		return obj
	case *ucore.PayloadTransferCrossChainAsset:
		obj := new(servers.TransferCrossChainAssetInfo)
		obj.CrossChainAddresses = object.CrossChainAddresses
		obj.OutputIndexes = object.OutputIndexes
		obj.CrossChainAmounts = object.CrossChainAmounts
		return obj
	case *ucore.PayloadTransferAsset:
	case *ucore.PayloadRecord:
	case *ucore.PayloadRechargeToSideChain:
		obj := new(servers.RechargeToSideChainInfo)
		obj.MainChainTransaction = BytesToHexString(object.MainChainTransaction)
		obj.Proof = BytesToHexString(object.MerkleProof)
		return obj
	}
	return nil
}

func GetTransactionInfo(header *ucore.Header, tx *ucore.Transaction) *servers.TransactionInfo {
	inputs := make([]servers.InputInfo, len(tx.Inputs))
	for i, v := range tx.Inputs {
		inputs[i].TxID = servers.ToReversedString(v.Previous.TxID)
		inputs[i].VOut = v.Previous.Index
		inputs[i].Sequence = v.Sequence
	}

	outputs := make([]servers.OutputInfo, len(tx.Outputs))
	for i, v := range tx.Outputs {
		outputs[i].Value = v.Value.String()
		outputs[i].TokenValue = v.TokenValue.String()
		outputs[i].Index = uint32(i)
		var address string
		destroyHash := Uint168{}
		if v.ProgramHash == destroyHash {
			address = servers.DESTROY_ADDRESS
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
		confirmations = blockchain.DefaultLedger.Blockchain.GetBestHeight() - header.Height + 1
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
		Payload:        servers.HttpServers.GetPayloadInfo(tx.Payload),
		Attributes:     attributes,
		Programs:       programs,
	}
}

func SendRawTransaction(param servers.Params) map[string]interface{} {
	str, ok := param.String("data")
	if !ok {
		return servers.ResponsePack(errors.InvalidParams, "need a string parameter named data")
	}

	bys, err := HexStringToBytes(str)
	if err != nil {
		return servers.ResponsePack(errors.InvalidParams, "hex string to bytes error")
	}
	var txn ucore.Transaction
	if err := txn.Deserialize(bytes.NewReader(bys)); err != nil {
		return servers.ResponsePack(errors.InvalidTransaction, "transaction deserialize error")
	}

	if errCode := servers.HttpServers.VerifyAndSendTx(&txn); errCode != errors.Success {
		return servers.ResponsePack(errCode, errCode.Message())
	}

	return servers.ResponsePack(errors.Success, servers.ToReversedString(txn.Hash()))
}