package mempool

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
	"github.com/elastos/Elastos.ELA.SideChain/config"
	"github.com/elastos/Elastos.ELA.SideChain/mempool"
	"github.com/elastos/Elastos.ELA.SideChain/spv"
	"github.com/elastos/Elastos.ELA.SideChain/types"
	"github.com/elastos/Elastos.ELA.Utility/common"
)

const (
	MinRegisterAssetTxFee = 1000000000
	CheckRegisterAssetTx = "checkregisterassettx"
)

type validator struct {
	*mempool.Validator

	systemAssetID common.Uint256
	foundation    common.Uint168
	spvService    *spv.Service
	db            *blockchain.ChainStore
}

func NewValidator(cfg *Config) *mempool.Validator {
	var val validator
	val.Validator = mempool.NewValidator(&mempool.Config{
		FoundationAddress: cfg.FoundationAddress,
		AssetId:           cfg.AssetId,
		ExchangeRage:      cfg.ExchangeRage,
		ChainStore:        cfg.ChainStore,
		SpvService:        cfg.SpvService,
		Validator:         cfg.Validator,
		FeeHelper:         nil,
	})
	val.systemAssetID = cfg.AssetId
	val.foundation = cfg.FoundationAddress
	val.spvService = cfg.SpvService
	val.db = cfg.ChainStore

	val.RegisterSanityFunc(mempool.FuncNames.CheckTransactionOutput, val.checkTransactionOutputImpl)
	val.RegisterSanityFunc(mempool.FuncNames.CheckAssetPrecision, val.checkAssetPrecisionImpl)
	val.RegisterSanityFunc(mempool.FuncNames.CheckTransactionPayload, val.checkTransactionPayloadImpl)
	val.RegisterContextFunc(mempool.FuncNames.CheckTransactionBalance, val.checkTransactionBalanceImpl)
	val.RegisterContextFunc(mempool.FuncNames.CheckReferencedOutput, val.checkReferencedOutputImpl)
	val.RegisterContextFunc(CheckRegisterAssetTx, val.CheckRegisterAssetTx)
	return val.Validator
}

func (v *validator) checkTransactionOutputImpl(txn *types.Transaction) error {
	if txn.IsCoinBaseTx() {
		if len(txn.Outputs) < 2 {
			return errors.New("coinbase output is not enough, at least 2")
		}

		var totalReward = common.Fixed64(0)
		var foundationReward = common.Fixed64(0)
		for _, output := range txn.Outputs {
			totalReward += output.Value
			if output.ProgramHash.IsEqual(v.foundation) {
				foundationReward += output.Value
			}
		}
		if common.Fixed64(foundationReward) < common.Fixed64(float64(totalReward)*0.3) {
			return errors.New("Reward to foundation in coinbase < 30%")
		}

		return nil
	}

	if len(txn.Outputs) < 1 {
		return errors.New("transaction has no outputs")
	}

	// check if output address is valid
	for _, output := range txn.Outputs {
		if output.AssetID == common.EmptyHash {
			return errors.New("asset id is nil")
		} else if output.AssetID == v.systemAssetID {
			if output.Value < 0 || output.TokenValue.Sign() != 0 {
				return errors.New("invalid transaction output with ela asset id")
			}
		} else {
			if txn.IsRechargeToSideChainTx() || txn.IsTransferCrossChainAssetTx() {
				return errors.New("cross chain asset tx asset id should only be ela asset id")
			}
			if output.TokenValue.Sign() < 0 || output.Value != 0 {
				return errors.New("invalid transaction output with token asset id")
			}
		}
		if !checkOutputProgramHash(output.ProgramHash) {
			return errors.New("output address is invalid")
		}
	}

	return nil
}

func checkOutputProgramHash(programHash common.Uint168) bool {
	var empty = common.Uint168{}
	prefix := programHash[0]
	if prefix == common.PrefixStandard ||
		prefix == common.PrefixMultisig ||
		prefix == common.PrefixCrossChain ||
		prefix == common.PrefixRegisterId ||
		programHash == empty {
		return true
	}
	return false
}

func (v *validator) checkReferencedOutputImpl(txn *types.Transaction) error {
	// check referenced Output value
	for _, input := range txn.Inputs {
		referHash := input.Previous.TxID
		referTxnOutIndex := input.Previous.Index
		referTxn, _, err := v.db.GetTransaction(referHash)
		if err != nil {
			desc := "Referenced transaction can not be found" + common.BytesToHexString(referHash.Bytes())
			return mempool.RuleError{ErrorCode: mempool.ErrUnknownReferedTx, Description: desc}
		}
		referTxnOut := referTxn.Outputs[referTxnOutIndex]
		if referTxnOut.AssetID.IsEqual(v.systemAssetID) {
			if referTxnOut.Value <= 0 {
				desc := "Value of referenced transaction output is invalid"
				return mempool.RuleError{ErrorCode: mempool.ErrInvalidReferedTx, Description: desc}
			}
		} else {
			if referTxnOut.TokenValue.Sign() <= 0 {
				desc := "TokenValue of referenced transaction output is invalid"
				return mempool.RuleError{ErrorCode: mempool.ErrInvalidReferedTx, Description: desc}
			}
		}
		// coinbase transaction only can be spent after got SpendCoinbaseSpan times confirmations
		if referTxn.IsCoinBaseTx() {
			lockHeight := referTxn.LockTime
			currentHeight := v.db.GetHeight()
			if currentHeight-lockHeight < config.Parameters.ChainParam.SpendCoinbaseSpan {
				desc := fmt.Sprintf("output is locked till %d, current %d", lockHeight, currentHeight)
				return mempool.RuleError{ErrorCode: mempool.ErrIneffectiveCoinbase, Description: desc}
			}
		}
	}
	return nil
}

func (v *validator) CheckRegisterAssetTx(txn *types.Transaction) error {
	if txn.TxType == types.RegisterAsset {
		if err := v.checkRegisterAssetTransaction(txn); err != nil {
			desc := "[CheckRegisterAssetTransaction]," + err.Error()
			return mempool.RuleError{ErrorCode: mempool.ErrInvalidReferedTx, Description: desc}
		}
	}
	return nil
}

func (v *validator) checkRegisterAssetTransaction(txn *types.Transaction) error {
	payload, ok := txn.Payload.(*types.PayloadRegisterAsset)
	if !ok {
		return fmt.Errorf("Invalid register asset transaction payload")
	}

	//asset name should be different
	assets := v.db.GetAssets()
	for char := range payload.Asset.Name {
		if char > 127 {
			return fmt.Errorf("allow only ASCII characters in asset name")
		}
	}

	for char := range payload.Asset.Description {
		if char > 127 {
			return fmt.Errorf("allow only ASCII characters in asset description")
		}
	}

	for _, asset := range assets {
		if asset.Name == payload.Asset.Name {
			return fmt.Errorf("Asset name has been registed")
		}
	}

	//amount and program hash should be same in output and payload
	totalToken := big.NewInt(0)
	for _, output := range txn.Outputs {
		if output.AssetID.IsEqual(payload.Asset.Hash()) {
			if !output.ProgramHash.IsEqual(payload.Controller) {
				return fmt.Errorf("Register asset program hash not same as program hash in payload")
			}
			totalToken.Add(totalToken, &output.TokenValue)
		}
	}
	regAmount := big.NewInt(payload.Amount.IntValue())
	regAmount.Mul(regAmount, getPrecisionBigInt())

	if totalToken.Cmp(regAmount) != 0 {
		return fmt.Errorf("Invalid register asset amount")
	}

	return nil
}

func checkAmountPrecise(amount common.Fixed64, precision byte, assetPrecision byte) bool {
	return amount.IntValue()%int64(math.Pow10(int(assetPrecision-precision))) == 0
}

func checkTokenAmountPrecise(amount big.Int, precision byte, assetPrecision byte) bool {
	value := amount.String()
	var decimal string
	if len(value) <= 18 {
		decimal = value
	} else {
		decimal = value[len(value)-18:]
	}
	decimalValue, ok := new(big.Int).SetString(decimal, 10)
	if !ok {
		fmt.Errorf("invalid token amount")
		return false
	}

	return decimalValue.Int64()%int64(math.Pow10(int(assetPrecision-precision))) == 0
}

func (v *validator) checkAssetPrecisionImpl(txn *types.Transaction) error {
	if txn.TxType == types.RegisterAsset {
		return nil
	}

	if len(txn.Outputs) == 0 {
		return nil
	}
	assetOutputs := make(map[common.Uint256][]*types.Output, len(txn.Outputs))

	for _, v := range txn.Outputs {
		assetOutputs[v.AssetID] = append(assetOutputs[v.AssetID], v)
	}
	for k, outputs := range assetOutputs {
		asset, err := v.db.GetAsset(k)
		if err != nil {
			desc := fmt.Sprint("[checkAssetPrecision] The asset not exist in local blockchain.")
			return mempool.RuleError{ErrorCode: mempool.ErrAssetPrecision, Description: desc}
		}
		precision := asset.Precision
		for _, output := range outputs {
			if output.AssetID.IsEqual(v.systemAssetID) {
				if !checkAmountPrecise(output.Value, precision, 8) {
					return errors.New("Invalide ela asset value,out of precise.")
					desc := fmt.Sprint("[checkAssetPrecision] The precision of asset is incorrect.")
					return mempool.RuleError{ErrorCode: mempool.ErrAssetPrecision, Description: desc}
				}
			} else {
				if !checkTokenAmountPrecise(output.TokenValue, precision, 18) {
					desc := fmt.Sprint("[checkAssetPrecision] Invalide asset value,out of precise.")
					return mempool.RuleError{ErrorCode: mempool.ErrAssetPrecision, Description: desc}
				}
			}
		}
	}

	return nil
}

func (v *validator) checkTransactionPayloadImpl(txn *types.Transaction) error {
	switch pld := txn.Payload.(type) {
	case *types.PayloadRegisterAsset:
		if pld.Asset.Precision < types.MinPrecision || pld.Asset.Precision > 18 {
			return errors.New("Invalide asset Precision.")
		}
		hash := txn.Hash()
		if hash.IsEqual(v.systemAssetID) {
			if !checkAmountPrecise(pld.Amount, pld.Asset.Precision, 8) {
				return errors.New("Invalide ela asset value,out of precise.")
			}
		}
	case *types.PayloadTransferAsset:
	case *types.PayloadRecord:
	case *types.PayloadCoinBase:
	case *types.PayloadRechargeToSideChain:
	case *types.PayloadTransferCrossChainAsset:
	default:
		return errors.New("[txValidator],invalidate transaction payload type.")
	}

	return nil
}

func (v *validator) checkTransactionBalanceImpl(txn *types.Transaction) error {
	var elaInputAmount = common.Fixed64(0)
	var tokenInputAmount = new(big.Int).SetInt64(0)
	var elaOutputAmount = common.Fixed64(0)
	var tokenOutputAmount = new(big.Int).SetInt64(0)

	references, err := v.db.GetTxReference(txn)
	if err != nil {
		return err
	}

	for _, output := range references {
		if output.AssetID.IsEqual(v.systemAssetID) {
			elaInputAmount += output.Value
		} else {
			tokenInputAmount.Add(tokenInputAmount, &(output.TokenValue))
		}
	}
	for _, output := range txn.Outputs {
		if output.AssetID.IsEqual(v.systemAssetID) {
			elaOutputAmount += output.Value
		} else {
			tokenOutputAmount.Add(tokenOutputAmount, &(output.TokenValue))
		}
	}

	elaBalance := elaInputAmount - elaOutputAmount
	if txn.IsTransferCrossChainAssetTx() || txn.IsRechargeToSideChainTx() {
		if int(elaBalance) < config.Parameters.MinCrossChainTxFee {
			return errors.New("crosschain transaction fee is not enough")
		}
	} else if txn.IsRegisterAssetTx() {
		if int(elaBalance) < MinRegisterAssetTxFee {
			return errors.New("register asset transaction fee is not enough")
		}
	} else {
		if int(elaBalance) < config.Parameters.PowConfiguration.MinTxFee {
			return errors.New("transaction fee is not enough")
		}
	}

	tokenBalance := tokenInputAmount.Sub(tokenInputAmount, tokenOutputAmount)
	if txn.TxType != types.RegisterAsset && tokenBalance.Sign() != 0 {
		return errors.New("token amount is not balanced")
	}
	return nil
}

func getPrecisionBigInt() *big.Int {
	value := big.Int{}
	value.SetString("1000000000000000000", 10)
	return &value
}
