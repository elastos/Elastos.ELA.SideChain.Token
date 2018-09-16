package blockchain

import (
	"errors"
	"fmt"


	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
	ucore "github.com/elastos/Elastos.ELA.SideChain/core"
	. "github.com/elastos/Elastos.ELA.Utility/common"
	"bytes"
)

type TokenChainStore struct {
	blockchain.ChainStore
}

func NewChainStore() (blockchain.IChainStore, error) {
	chainStore, err := blockchain.NewChainStore()
	if err != nil {
		return nil, err
	}

	store := &TokenChainStore{
		ChainStore: *chainStore,
	}
	store.Init()

	go store.Loop()

	return store, nil
}

func (c *TokenChainStore) GetTxReference(tx *ucore.Transaction) (map[*ucore.Input]*ucore.Output, error) {
	//UTXO input /  Outputs
	reference := make(map[*ucore.Input]*ucore.Output)
	// Key index，v UTXOInput
	for _, utxo := range tx.Inputs {
		transaction, _, err := c.GetTransaction(utxo.Previous.TxID)
		if err != nil {
			return nil, errors.New("GetTxReference failed, previous transaction not found")
		}
		index := utxo.Previous.Index
		if int(index) >= len(transaction.Outputs) {
			return nil, errors.New("GetTxReference failed, refIdx out of range.")
		}
		reference[utxo] = transaction.Outputs[index]
	}
	return reference, nil
}

func (c *TokenChainStore) Init() {
	c.PersistUnspendUTXOs = c.PersistUnspendUTXOsImpl
	c.RollbackUnspendUTXOs = c.RollbackUnspendUTXOsImpl
	c.PersistTransactions = c.PersistTransactionsImpl
	c.PersistUnspend = c.PersistUnspendImpl
	c.RollbackUnspend = c.RollbackUnspendImpl
}

func (c *TokenChainStore) PersistUnspendUTXOsImpl(b *ucore.Block) error {
	unspendUTXOs := make(map[Uint168]map[Uint256]map[uint32][]*blockchain.UTXO)
	curHeight := b.Header.Height

	for _, txn := range b.Transactions {
		for index, output := range txn.Outputs {
			programHash := output.ProgramHash
			assetID := output.AssetID
			value := output.Value

			if _, ok := unspendUTXOs[programHash]; !ok {
				unspendUTXOs[programHash] = make(map[Uint256]map[uint32][]*blockchain.UTXO)
			}

			if _, ok := unspendUTXOs[programHash][assetID]; !ok {
				unspendUTXOs[programHash][assetID] = make(map[uint32][]*blockchain.UTXO, 0)
			}

			if _, ok := unspendUTXOs[programHash][assetID][curHeight]; !ok {
				var err error
				unspendUTXOs[programHash][assetID][curHeight], err = c.GetUnspentElementFromProgramHash(programHash, assetID, curHeight)
				if err != nil {
					unspendUTXOs[programHash][assetID][curHeight] = make([]*blockchain.UTXO, 0)
				}

			}

			u := blockchain.UTXO{
				TxId:  txn.Hash(),
				Index: uint32(index),
				Value: value,
			}
			unspendUTXOs[programHash][assetID][curHeight] = append(unspendUTXOs[programHash][assetID][curHeight], &u)
		}

		if !txn.IsCoinBaseTx() {
			for _, input := range txn.Inputs {
				referTxn, height, err := c.GetTransaction(input.Previous.TxID)
				if err != nil {
					return err
				}
				index := input.Previous.Index
				referTxnOutput := referTxn.Outputs[index]
				programHash := referTxnOutput.ProgramHash
				assetID := referTxnOutput.AssetID

				if _, ok := unspendUTXOs[programHash]; !ok {
					unspendUTXOs[programHash] = make(map[Uint256]map[uint32][]*blockchain.UTXO)
				}
				if _, ok := unspendUTXOs[programHash][assetID]; !ok {
					unspendUTXOs[programHash][assetID] = make(map[uint32][]*blockchain.UTXO)
				}

				if _, ok := unspendUTXOs[programHash][assetID][height]; !ok {
					unspendUTXOs[programHash][assetID][height], err = c.GetUnspentElementFromProgramHash(programHash, assetID, height)
					if err != nil {
						return errors.New(fmt.Sprintf("[persist] UTXOs programHash:%v, assetId:%v height:%v has no unspent UTXO.", programHash, assetID, height))
					}
				}

				flag := false
				listnum := len(unspendUTXOs[programHash][assetID][height])
				for i := 0; i < listnum; i++ {
					if unspendUTXOs[programHash][assetID][height][i].TxId.IsEqual(referTxn.Hash()) && unspendUTXOs[programHash][assetID][height][i].Index == uint32(index) {
						unspendUTXOs[programHash][assetID][height][i] = unspendUTXOs[programHash][assetID][height][listnum-1]
						unspendUTXOs[programHash][assetID][height] = unspendUTXOs[programHash][assetID][height][:listnum-1]
						flag = true
						break
					}
				}
				if !flag {
					return errors.New(fmt.Sprintf("[persist] UTXOs NOT find UTXO by txid: %x, index: %d.", referTxn.Hash(), index))
				}
			}
		}
	}

	// batch put the UTXOs
	for programHash, programHash_value := range unspendUTXOs {
		for assetId, unspents := range programHash_value {
			for height, unspent := range unspents {
				err := c.PersistUnspentWithProgramHash(programHash, assetId, height, unspent)
				if err != nil {
					return err
				}
			}

		}
	}

	return nil
}

func (c *TokenChainStore) RollbackUnspendUTXOsImpl(b *ucore.Block) error {
	unspendUTXOs := make(map[Uint168]map[Uint256]map[uint32][]*blockchain.UTXO)
	height := b.Header.Height
	for _, txn := range b.Transactions {
		for index, output := range txn.Outputs {
			programHash := output.ProgramHash
			assetID := output.AssetID
			value := output.Value
			if _, ok := unspendUTXOs[programHash]; !ok {
				unspendUTXOs[programHash] = make(map[Uint256]map[uint32][]*blockchain.UTXO)
			}
			if _, ok := unspendUTXOs[programHash][assetID]; !ok {
				unspendUTXOs[programHash][assetID] = make(map[uint32][]*blockchain.UTXO)
			}
			if _, ok := unspendUTXOs[programHash][assetID][height]; !ok {
				var err error
				unspendUTXOs[programHash][assetID][height], err = c.GetUnspentElementFromProgramHash(programHash, assetID, height)
				if err != nil {
					return errors.New(fmt.Sprintf("[persist] UTXOs programHash:%v, assetId:%v has no unspent UTXO.", programHash, assetID))
				}
			}
			u := blockchain.UTXO{
				TxId:  txn.Hash(),
				Index: uint32(index),
				Value: value,
			}
			var position int
			for i, unspend := range unspendUTXOs[programHash][assetID][height] {
				if unspend.TxId == u.TxId && unspend.Index == u.Index {
					position = i
					break
				}
			}
			unspendUTXOs[programHash][assetID][height] = append(unspendUTXOs[programHash][assetID][height][:position], unspendUTXOs[programHash][assetID][height][position+1:]...)
		}

		if !txn.IsCoinBaseTx() {
			for _, input := range txn.Inputs {
				referTxn, hh, err := c.GetTransaction(input.Previous.TxID)
				if err != nil {
					return err
				}
				index := input.Previous.Index
				referTxnOutput := referTxn.Outputs[index]
				programHash := referTxnOutput.ProgramHash
				assetID := referTxnOutput.AssetID
				if _, ok := unspendUTXOs[programHash]; !ok {
					unspendUTXOs[programHash] = make(map[Uint256]map[uint32][]*blockchain.UTXO)
				}
				if _, ok := unspendUTXOs[programHash][assetID]; !ok {
					unspendUTXOs[programHash][assetID] = make(map[uint32][]*blockchain.UTXO)
				}
				if _, ok := unspendUTXOs[programHash][assetID][hh]; !ok {
					unspendUTXOs[programHash][assetID][hh], err = c.GetUnspentElementFromProgramHash(programHash, assetID, hh)
					if err != nil {
						unspendUTXOs[programHash][assetID][hh] = make([]*blockchain.UTXO, 0)
					}
				}
				u := blockchain.UTXO{
					TxId:  referTxn.Hash(),
					Index: uint32(index),
					Value: referTxnOutput.Value,
				}
				unspendUTXOs[programHash][assetID][hh] = append(unspendUTXOs[programHash][assetID][hh], &u)
			}
		}
	}

	for programHash, programHash_value := range unspendUTXOs {
		for assetId, unspents := range programHash_value {
			for height, unspent := range unspents {
				err := c.PersistUnspentWithProgramHash(programHash, assetId, height, unspent)
				if err != nil {
					return err
				}
			}

		}
	}

	return nil
}

func (c *TokenChainStore) PersistTransactionsImpl(b *ucore.Block) error {
	for _, txn := range b.Transactions {
		if err := c.PersistTransaction(txn, b.Header.Height); err != nil {
			return err
		}
		if txn.TxType == ucore.RegisterAsset {
			regPayload := txn.Payload.(*ucore.PayloadRegisterAsset)
			if blockchain.DefaultLedger.Blockchain.AssetID.IsEqual(txn.Hash()) {
				if err := c.PersistAsset(txn.Hash(), regPayload.Asset); err != nil {
					return err
				}
			} else {
				if err := c.PersistAsset(regPayload.Asset.Hash(), regPayload.Asset); err != nil {
					return err
				}
			}
		}
		if txn.TxType == ucore.RechargeToSideChain {
			rechargePayload := txn.Payload.(*ucore.PayloadRechargeToSideChain)
			hash, err := rechargePayload.GetMainchainTxHash()
			if err != nil {
				return err
			}
			c.PersistMainchainTx(*hash)
		}
	}
	return nil
}

func (c *TokenChainStore) PersistUnspendImpl(b *ucore.Block) error {
	unspentPrefix := []byte{byte(blockchain.IX_Unspent)}
	unspents := make(map[Uint256][]uint16)
	for _, txn := range b.Transactions {
		txnHash := txn.Hash()
		for index := range txn.Outputs {
			unspents[txnHash] = append(unspents[txnHash], uint16(index))
		}
		if !txn.IsCoinBaseTx() {
			for index, input := range txn.Inputs {
				referTxnHash := input.Previous.TxID
				if _, ok := unspents[referTxnHash]; !ok {
					unspentValue, err := c.Get(append(unspentPrefix, referTxnHash.Bytes()...))
					if err != nil {
						return err
					}
					unspents[referTxnHash], err = blockchain.GetUint16Array(unspentValue)
					if err != nil {
						return err
					}
				}

				unspentLen := len(unspents[referTxnHash])
				for k, outputIndex := range unspents[referTxnHash] {
					if outputIndex == uint16(txn.Inputs[index].Previous.Index) {
						unspents[referTxnHash][k] = unspents[referTxnHash][unspentLen-1]
						unspents[referTxnHash] = unspents[referTxnHash][:unspentLen-1]
						break
					}
				}
			}
		}
	}

	for txhash, value := range unspents {
		key := bytes.NewBuffer(nil)
		key.WriteByte(byte(blockchain.IX_Unspent))
		txhash.Serialize(key)

		if len(value) == 0 {
			c.BatchDelete(key.Bytes())
		} else {
			unspentArray := blockchain.ToByteArray(value)
			c.BatchPut(key.Bytes(), unspentArray)
		}
	}

	return nil
}

func (c *TokenChainStore) RollbackUnspendImpl(b *ucore.Block) error {
	unspentPrefix := []byte{byte(blockchain.IX_Unspent)}
	unspents := make(map[Uint256][]uint16)
	for _, txn := range b.Transactions {
		// remove all utxos created by this transaction
		txnHash := txn.Hash()
		c.BatchDelete(append(unspentPrefix, txnHash.Bytes()...))
		if !txn.IsCoinBaseTx() {

			for _, input := range txn.Inputs {
				referTxnHash := input.Previous.TxID
				referTxnOutIndex := input.Previous.Index
				if _, ok := unspents[referTxnHash]; !ok {
					var err error
					unspentValue, _ := c.Get(append(unspentPrefix, referTxnHash.Bytes()...))
					if len(unspentValue) != 0 {
						unspents[referTxnHash], err = blockchain.GetUint16Array(unspentValue)
						if err != nil {
							return err
						}
					}
				}
				unspents[referTxnHash] = append(unspents[referTxnHash], referTxnOutIndex)
			}
		}
	}

	for txhash, value := range unspents {
		key := bytes.NewBuffer(nil)
		key.WriteByte(byte(blockchain.IX_Unspent))
		txhash.Serialize(key)

		if len(value) == 0 {
			c.BatchDelete(key.Bytes())
		} else {
			unspentArray := blockchain.ToByteArray(value)
			c.BatchPut(key.Bytes(), unspentArray)
		}
	}

	return nil
}