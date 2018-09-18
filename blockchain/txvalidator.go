package blockchain

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
	"github.com/elastos/Elastos.ELA.SideChain/config"
	ucore "github.com/elastos/Elastos.ELA.SideChain/core"
	. "github.com/elastos/Elastos.ELA.SideChain/errors"
	"github.com/elastos/Elastos.ELA.SideChain/log"
	. "github.com/elastos/Elastos.ELA.Utility/common"
)

func InitTransactionValidtor() {
	blockchain.TransactionValidator = &blockchain.TransactionValidateBase{}
	blockchain.TransactionValidator.Init()
	blockchain.TransactionValidator.CheckTransactionOutput = CheckTransactionOutputImpl
	blockchain.TransactionValidator.CheckTransactionContext = CheckTransactionContextImpl
	blockchain.TransactionValidator.CheckAssetPrecision = CheckAssetPrecisionImpl
	blockchain.TransactionValidator.CheckTransactionPayload = CheckTransactionPayloadImpl
	blockchain.TransactionValidator.CheckTransactionBalance = CheckTransactionFee
	blockchain.TransactionValidator.CheckReferencedOutput = CheckReferencedOutputImpl
}

func CheckTransactionOutputImpl(txn *ucore.Transaction) error {
	if txn.IsCoinBaseTx() {
		if len(txn.Outputs) < 2 {
			return errors.New("coinbase output is not enough, at least 2")
		}

		var totalReward = Fixed64(0)
		var foundationReward = Fixed64(0)
		for _, output := range txn.Outputs {
			totalReward += output.Value
			if output.ProgramHash.IsEqual(blockchain.FoundationAddress) {
				foundationReward += output.Value
			}
		}
		if Fixed64(foundationReward) < Fixed64(float64(totalReward)*0.3) {
			return errors.New("Reward to foundation in coinbase < 30%")
		}

		return nil
	}

	if len(txn.Outputs) < 1 {
		return errors.New("transaction has no outputs")
	}

	// check if output address is valid
	for _, output := range txn.Outputs {
		if output.AssetID == EmptyHash {
			return errors.New("asset id is nil")
		} else if output.AssetID == blockchain.DefaultLedger.Blockchain.AssetID {
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
		if !blockchain.TransactionValidator.CheckOutputProgramHash(output.ProgramHash) {
			return errors.New("output address is invalid")
		}
	}

	return nil
}

// CheckTransactionContext verifys a transaction with history transaction in ledger
func CheckTransactionContextImpl(txn *ucore.Transaction) ErrCode {
	if ok, errcode := blockchain.TransactionValidator.CheckTxHashDuplicate(txn); !ok {
		return errcode
	}
	if ok, errcode := blockchain.TransactionValidator.CheckCoinBaseTx(txn); !ok {
		return errcode
	}
	if ok, errcode := blockchain.TransactionValidator.CheckSignature(txn); !ok {
		return errcode
	}
	if ok, errcode := blockchain.TransactionValidator.CheckRechargeToSideChainTx(txn); !ok {
		return errcode
	}
	if ok, errcode := blockchain.TransactionValidator.CheckTransferCrossChainAssetTx(txn); !ok {
		return errcode
	}
	if ok, errcode := CheckRegisterAssetTx(txn); !ok {
		return errcode
	}
	if ok, errcode := blockchain.TransactionValidator.CheckDoubleSpend(txn); !ok {
		return errcode
	}
	if ok, errcode := blockchain.TransactionValidator.CheckUTXOLock(txn); !ok {
		return errcode
	}
	if ok, errcode := blockchain.TransactionValidator.CheckBalance(txn); !ok {
		return errcode
	}
	if ok, errcode := blockchain.TransactionValidator.CheckReferencedOutput(txn); !ok {
		return errcode
	}

	return Success
}

func CheckReferencedOutputImpl(txn *ucore.Transaction) (bool, ErrCode) {
	// check referenced Output value
	for _, input := range txn.Inputs {
		referHash := input.Previous.TxID
		referTxnOutIndex := input.Previous.Index
		referTxn, _, err := blockchain.DefaultLedger.Store.GetTransaction(referHash)
		if err != nil {
			log.Warn("Referenced transaction can not be found", BytesToHexString(referHash.Bytes()))
			return false, ErrUnknownReferedTxn
		}
		referTxnOut := referTxn.Outputs[referTxnOutIndex]
		if referTxnOut.AssetID.IsEqual(blockchain.DefaultLedger.Blockchain.AssetID) {
			if referTxnOut.Value <= 0 {
				log.Warn("Value of referenced transaction output is invalid")
				return false, ErrInvalidReferedTxn
			}
		} else {
			if referTxnOut.TokenValue.Sign() <= 0 {
				log.Warn("TokenValue of referenced transaction output is invalid")
				return false, ErrInvalidReferedTxn
			}
		}
		// coinbase transaction only can be spent after got SpendCoinbaseSpan times confirmations
		if referTxn.IsCoinBaseTx() {
			lockHeight := referTxn.LockTime
			currentHeight := blockchain.DefaultLedger.Store.GetHeight()
			if currentHeight-lockHeight < config.Parameters.ChainParam.SpendCoinbaseSpan {
				return false, ErrIneffectiveCoinbase
			}
		}
	}
	return true, Success
}

func CheckRegisterAssetTx(txn *ucore.Transaction) (bool, ErrCode) {
	if txn.TxType == ucore.RegisterAsset {
		if err := CheckRegisterAssetTransaction(txn); err != nil {
			log.Warn("[CheckRegisterAssetTransaction],", err)
			return false, ErrInvalidOutput
		}
	}
	return true, Success
}

func CheckRegisterAssetTransaction(txn *ucore.Transaction) error {
	payload, ok := txn.Payload.(*ucore.PayloadRegisterAsset)
	if !ok {
		return fmt.Errorf("Invalid register asset transaction payload")
	}

	//asset name should be different
	assets := blockchain.DefaultLedger.Store.GetAssets()
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

func CheckAssetPrecisionImpl(txn *ucore.Transaction) error {
	if txn.TxType == ucore.RegisterAsset {
		return nil
	}

	if len(txn.Outputs) == 0 {
		return nil
	}
	assetOutputs := make(map[Uint256][]*ucore.Output, len(txn.Outputs))

	for _, v := range txn.Outputs {
		assetOutputs[v.AssetID] = append(assetOutputs[v.AssetID], v)
	}
	for k, outputs := range assetOutputs {
		asset, err := blockchain.DefaultLedger.GetAsset(k)
		if err != nil {
			return errors.New("The asset not exist in local blockchain.")
		}
		precision := asset.Precision
		for _, output := range outputs {
			if output.AssetID.IsEqual(blockchain.DefaultLedger.Blockchain.AssetID) {
				if !blockchain.TransactionValidator.CheckAmountPrecise(output.Value, precision, 8) {
					return errors.New("Invalide ela asset value,out of precise.")
				}
			} else {
				if !blockchain.TransactionValidator.CheckAmountPrecise(output.Value, precision, 18) {
					return errors.New("Invalide asset value,out of precise.")
				}
			}
		}
	}
	return nil
}

func CheckTransactionPayloadImpl(txn *ucore.Transaction) error {
	switch pld := txn.Payload.(type) {
	case *ucore.PayloadRegisterAsset:
		if pld.Asset.Precision < ucore.MinPrecision || pld.Asset.Precision > 18 {
			return errors.New("Invalide asset Precision.")
		}
		hash := txn.Hash()
		if hash.IsEqual(blockchain.DefaultLedger.Blockchain.AssetID) {
			if !blockchain.TransactionValidator.CheckAmountPrecise(pld.Amount, pld.Asset.Precision, 8) {
				return errors.New("Invalide ela asset value,out of precise.")
			}
		}
	case *ucore.PayloadTransferAsset:
	case *ucore.PayloadRecord:
	case *ucore.PayloadCoinBase:
	case *ucore.PayloadRechargeToSideChain:
	case *ucore.PayloadTransferCrossChainAsset:
	default:
		return errors.New("[txValidator],invalidate transaction payload type.")
	}

	return nil
}

func CheckTransactionFee(txn *ucore.Transaction) error {
	var elaInputAmount = Fixed64(0)
	var tokenInputAmount = new(big.Int).SetInt64(0)
	var elaOutputAmount = Fixed64(0)
	var tokenOutputAmount = new(big.Int).SetInt64(0)

	references, err := blockchain.DefaultLedger.Store.GetTxReference(txn)
	if err != nil {
		return err
	}

	for _, output := range references {
		if output.AssetID.IsEqual(blockchain.DefaultLedger.Blockchain.AssetID) {
			elaInputAmount += output.Value
		} else {
			tokenInputAmount.Add(tokenInputAmount, &(output.TokenValue))
		}
	}
	for _, output := range txn.Outputs {
		if output.AssetID.IsEqual(blockchain.DefaultLedger.Blockchain.AssetID) {
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
	} else {
		if int(elaBalance) < config.Parameters.PowConfiguration.MinTxFee {
			return errors.New("transaction fee is not enough")
		}
	}

	tokenBalance := tokenInputAmount.Sub(tokenInputAmount, tokenOutputAmount)
	if txn.TxType != ucore.RegisterAsset && tokenBalance.Sign() != 0 {
		return errors.New("token amount is not balanced")
	}
	return nil
}

func getPrecisionBigInt() *big.Int {
	value := big.Int{}
	value.SetString("1000000000000000000", 10)
	return &value
}
