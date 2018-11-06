package service

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/elastos/Elastos.ELA.SideChain.Token/blockchain"
	"github.com/elastos/Elastos.ELA.SideChain/service"
	"github.com/elastos/Elastos.ELA.SideChain/types"
	. "github.com/elastos/Elastos.ELA.Utility/common"
	"github.com/elastos/Elastos.ELA.Utility/http/util"
)

type HttpServiceExtend struct {
	*service.HttpService
	chain *blockchain.TokenChainStore
}

func NewHttpService(cfg *service.Config, store *blockchain.TokenChainStore) *HttpServiceExtend {
	server := &HttpServiceExtend{
		HttpService: service.NewHttpService(cfg),
		chain:       store,
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
			outputs[i].Value = new(big.Int).Div(&v.TokenValue, big.NewInt(int64(math.Pow10(18)))).String()
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

func (s *HttpServiceExtend) GetReceivedByAddress(param util.Params) (interface{}, error) {
	tokenValueList := make(map[Uint256]*big.Int)
	var elaValue Fixed64
	str, ok := param.String("addr")
	if !ok {
		return nil, fmt.Errorf(service.InvalidParams.String())
	}

	programHash, err := Uint168FromAddress(str)
	if err != nil {
		return nil, fmt.Errorf(service.InvalidParams.String())
	}
	unspends, err := s.chain.GetUnspents(*programHash)
	for assetID, utxos := range unspends {
		for _, u := range utxos {
			if assetID == types.GetSystemAssetId() {
				value, _ := Fixed64FromBytes(u.Value)
				elaValue += *value
			} else {
				value := new(big.Int).SetBytes(u.Value)
				if _, ok := tokenValueList[assetID]; !ok {
					tokenValueList[assetID] = new(big.Int)
				}
				tokenValueList[assetID] = tokenValueList[assetID].Add(tokenValueList[assetID], value)
			}
		}
	}
	valueList := make(map[string]string)
	valueList[types.GetSystemAssetId().String()] = elaValue.String()
	for k, v := range tokenValueList {
		reverse, _ := Uint256FromBytes(BytesReverse(k.Bytes()))
		totalValue, _ := new(big.Int).SetString(v.String(), 10)
		valueList[reverse.String()] = totalValue.Div(totalValue, big.NewInt(int64(math.Pow10(18)))).String()
	}
	if assetID, ok := param.String("assetid"); ok {
		return map[string]string{assetID: valueList[assetID]}, nil
	} else {
		return valueList, nil
	}
}

func (s *HttpServiceExtend) ListUnspent(param util.Params) (interface{}, error) {
	bestHeight := s.chain.GetHeight()
	type UTXOInfo struct {
		AssetId       string `json:"assetid"`
		Txid          string `json:"txid"`
		VOut          uint32 `json:"vout"`
		Address       string `json:"address"`
		Amount        string `json:"amount"`
		Confirmations uint32 `json:"confirmations"`
		OutputLock    uint32 `json:"outputlock"`
	}

	var allResults, results []UTXOInfo

	if _, ok := param["addresses"]; !ok {
		return nil, errors.New("need a param called address")
	}
	var addressStrings []string
	switch addresses := param["addresses"].(type) {
	case []interface{}:
		for _, v := range addresses {
			str, ok := v.(string)
			if !ok {
				return nil, errors.New("please send a string")
			}
			addressStrings = append(addressStrings, str)
		}
	default:
		return nil, errors.New("wrong type")
	}

	for _, address := range addressStrings {
		programHash, err := Uint168FromAddress(address)
		if err != nil {
			return nil, errors.New("Invalid address: " + address)
		}
		differentAssets, err := s.chain.GetUnspents(*programHash)
		if err != nil {
			return nil, errors.New("cannot get asset with program")
		}
		for _, asset := range differentAssets {
			for _, unspent := range asset {
				tx, height, err := s.chain.GetTransaction(unspent.TxID)
				if err != nil {
					return nil, errors.New("unknown transaction " + unspent.TxID.String() + " from persisted utxo")
				}
				allResults = append(allResults, UTXOInfo{
					Amount:        unspent.ValueString(),
					AssetId:       BytesToHexString(BytesReverse(unspent.AssetID[:])),
					Txid:          BytesToHexString(BytesReverse(unspent.TxID[:])),
					VOut:          unspent.Index,
					Address:       address,
					Confirmations: bestHeight - height + 1,
					OutputLock:    tx.Outputs[unspent.Index].OutputLock,
				})
			}
		}
	}
	if assetID, ok := param.String("assetid"); ok {
		for _, result := range allResults {
			if result.AssetId == assetID {
				results = append(results, result)
			}
		}
	} else {
		results = allResults
	}
	return results, nil
}

func (s *HttpServiceExtend) GetUnspendsByAddr(param util.Params) (interface{}, error) {
	addr, ok := param.String("addr")
	if !ok {
		return nil, errors.New(service.InvalidParams.String())
	}

	programHash, err := Uint168FromAddress(addr)
	if err != nil {
		return nil, errors.New(service.InvalidParams.String())
	}
	type UTXOUnspentInfo struct {
		TxID  string
		Index uint32
		Value string
	}
	type Result struct {
		AssetId   string
		AssetName string
		Utxo      []UTXOUnspentInfo
	}
	var results []Result
	unspends, err := s.chain.GetUnspents(*programHash)

	for k, u := range unspends {
		asset, err := s.chain.GetAsset(k)
		if err != nil {
			return nil, errors.New(service.InternalError.String())
		}
		var unspendsInfo []UTXOUnspentInfo
		for _, v := range u {
			unspendsInfo = append(unspendsInfo, UTXOUnspentInfo{
				Value: v.ValueString(),
				TxID: BytesToHexString(BytesReverse(v.TxID[:])),
				Index: v.Index,})
		}
		results = append(results, struct {
			AssetId   string
			AssetName string
			Utxo      []UTXOUnspentInfo
		}{BytesToHexString(BytesReverse(k[:])), asset.Name, unspendsInfo})
	}
	return results, nil
}

func (s *HttpServiceExtend) GetBalanceByAddr(param util.Params) (interface{}, error) {
	tokenValueList := make(map[Uint256]*big.Int)
	var elaValue Fixed64
	str, ok := param.String("addr")
	if !ok {
		return nil, fmt.Errorf(service.InvalidParams.String())
	}

	programHash, err := Uint168FromAddress(str)
	if err != nil {
		return nil, fmt.Errorf(service.InvalidParams.String())
	}
	unspends, err := s.chain.GetUnspents(*programHash)
	for assetID, utxos := range unspends {
		for _, u := range utxos {
			if assetID == types.GetSystemAssetId() {
				value, _ := Fixed64FromBytes(u.Value)
				elaValue += *value
			} else {
				value := new(big.Int).SetBytes(u.Value)
				if _, ok := tokenValueList[assetID]; !ok {
					tokenValueList[assetID] = new(big.Int)
				}
				tokenValueList[assetID] = tokenValueList[assetID].Add(tokenValueList[assetID], value)
			}
		}
	}
	valueList := make(map[string]string)
	valueList[types.GetSystemAssetId().String()] = elaValue.String()
	for k, v := range tokenValueList {
		reverse, _ := Uint256FromBytes(BytesReverse(k.Bytes()))
		totalValue, _ := new(big.Int).SetString(v.String(), 10)
		valueList[reverse.String()] = totalValue.Div(totalValue, big.NewInt(int64(math.Pow10(18)))).String()
	}
	if assetID, ok := param.String("assetid"); ok {
		return map[string]string{assetID: valueList[assetID]}, nil
	} else {
		return valueList, nil
	}
}

func (s *HttpServiceExtend) GetAssetList(param util.Params) (interface{}, error) {
	type AssetInfo struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Precision   byte   `json:"precision"`
		Height      uint32 `json:"height"`
		ID          string `json:"assetid"`
	}
	var assetArray []AssetInfo
	assets := s.chain.GetAssets()
	for assetID, asset := range assets {
		assetArray = append(assetArray, AssetInfo{
			asset.Name,
			asset.Description,
			asset.Precision,
			asset.Height,
			BytesToHexString(BytesReverse(assetID[:]))})
	}

	return assetArray, nil
}
