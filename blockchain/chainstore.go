package blockchain

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"

	"github.com/elastos/Elastos.ELA.SideChain.Token/core"
	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
	"github.com/elastos/Elastos.ELA.SideChain/config"
	"github.com/elastos/Elastos.ELA.SideChain/database"
	"github.com/elastos/Elastos.ELA.SideChain/types"
	. "github.com/elastos/Elastos.ELA/common"
)

const IX_Unspent_UTXO = 0x91

type TokenChainStore struct {
	*blockchain.ChainStore
	systemAssetID Uint256
}

type Config struct {
	ChainParams *config.Params
	GetTxFee    func(tx *types.Transaction) Fixed64
}

type AssetInfo struct {
	types.Asset
	Height uint32
}

func (a *AssetInfo) Serialize(w io.Writer) error {
	if err := a.Asset.Serialize(w); err != nil {
		return err
	}

	if err := WriteUint32(w, a.Height); err != nil {
		return err
	}
	return nil
}

func (a *AssetInfo) Deserialize(r io.Reader) error {
	var err error
	if err := a.Asset.Deserialize(r); err != nil {
		return err
	}
	if a.Asset.Name != "ELA" {
		if a.Height, err = ReadUint32(r); err != nil {
			return err
		}
	}
	return nil
}

type utxo struct {
	TxID    Uint256
	Index   uint32
	AssetID Uint256
	Value   []byte
}

func (u *utxo) ValueString() string {
	maxPrecision := 18
	if u.AssetID == types.GetSystemAssetId() {
		number, _ := Fixed64FromBytes(u.Value)
		return number.String()
	} else {
		total := new(big.Int).SetBytes(u.Value).String()
		if total == "0" {
			return total
		}
		numberLength := len(total)
		if numberLength >= maxPrecision+1 {
			fractionalPart := total[numberLength-maxPrecision:]
			integerPart := total[:numberLength-maxPrecision]
			return integerPart + "." + fractionalPart
		} else {
			return "0." + strings.Repeat("0", maxPrecision-len(total)) + total
		}
	}
}

func (u *utxo) Serialize(w io.Writer) error {
	if err := u.TxID.Serialize(w); err != nil {
		return err
	}

	if err := WriteUint32(w, u.Index); err != nil {
		return err
	}

	if err := u.AssetID.Serialize(w); err != nil {
		return err
	}
	if err := WriteVarBytes(w, u.Value); err != nil {
		return err
	}

	return nil
}

func (u *utxo) Deserialize(r io.Reader) error {
	var err error
	if err := u.TxID.Deserialize(r); err != nil {
		return err
	}

	u.Index, err = ReadUint32(r)
	if err != nil {
		return err
	}

	if err := u.AssetID.Deserialize(r); err != nil {
		return err
	}

	u.Value, err = ReadVarBytes(r, core.MaxTokenValueDataSize, "value")
	if err != nil {
		return nil
	}
	return nil
}

func NewChainStore(genesisBlock *types.Block, assetID Uint256, dataPath string) (*TokenChainStore, error) {
	chainStore, err := blockchain.NewChainStore(dataPath, genesisBlock)
	if err != nil {
		return nil, err
	}

	store := &TokenChainStore{
		ChainStore:    chainStore,
		systemAssetID: assetID,
	}
	store.RegisterFunctions(true, blockchain.StoreFuncNames.PersistUnspendUTXOs, store.persistUnspendUTXOs)
	store.RegisterFunctions(true, blockchain.StoreFuncNames.PersistTransactions, store.persistTransactions)
	store.RegisterFunctions(true, blockchain.StoreFuncNames.PersistUnspend, store.persistUnspend)

	store.RegisterFunctions(false, blockchain.StoreFuncNames.RollbackUnspendUTXOs, store.rollbackUnspendUTXOs)
	store.RegisterFunctions(false, blockchain.StoreFuncNames.RollbackTransactions, store.rollbackTransactions)
	store.RegisterFunctions(false, blockchain.StoreFuncNames.RollbackUnspend, store.rollbackUnspend)

	return store, nil
}

func (c *TokenChainStore) GetTxReference(tx *types.Transaction) (map[*types.Input]*types.Output, error) {
	//utxo input /  Outputs
	reference := make(map[*types.Input]*types.Output)
	// Key index，v UTXOInput
	for _, u := range tx.Inputs {
		transaction, _, err := c.GetTransaction(u.Previous.TxID)
		if err != nil {
			return nil, errors.New("GetTxReference failed, previous transaction not found")
		}
		index := u.Previous.Index
		if int(index) >= len(transaction.Outputs) {
			return nil, errors.New("GetTxReference failed, refIdx out of range.")
		}
		reference[u] = transaction.Outputs[index]
	}
	return reference, nil
}

func (c *TokenChainStore) GetUnspents(programHash Uint168) (map[Uint256][]*utxo, error) {
	uxtoUnspents := make(map[Uint256][]*utxo)

	prefix := []byte{byte(IX_Unspent_UTXO)}
	key := append(prefix, programHash.Bytes()...)
	iter := c.NewIterator(key)
	defer iter.Release()
	for iter.Next() {
		rk := bytes.NewReader(iter.Key())

		// read prefix
		_, _ = ReadBytes(rk, 1)
		var programHash Uint168
		programHash.Deserialize(rk)
		var assetid Uint256
		assetid.Deserialize(rk)

		r := bytes.NewReader(iter.Value())
		listNum, err := ReadVarUint(r, 0)
		if err != nil {
			return nil, err
		}

		// read unspent list in store
		unspents := make([]*utxo, listNum)
		for i := 0; i < int(listNum); i++ {
			var u utxo
			err := u.Deserialize(r)
			if err != nil {
				return nil, err
			}

			unspents[i] = &u
		}
		uxtoUnspents[assetid] = append(uxtoUnspents[assetid], unspents[:]...)
	}

	return uxtoUnspents, nil
}

func (c *TokenChainStore) GetUnspentElementFromProgramHash(programHash Uint168, assetid Uint256, height uint32) ([]*utxo, error) {
	prefix := []byte{byte(IX_Unspent_UTXO)}
	prefix = append(prefix, programHash.Bytes()...)
	prefix = append(prefix, assetid.Bytes()...)

	key := bytes.NewBuffer(prefix)
	if err := WriteUint32(key, height); err != nil {
		return nil, err
	}
	unspentsData, err := c.Get(key.Bytes())
	if err != nil {
		return nil, err
	}
	r := bytes.NewReader(unspentsData)
	listNum, err := ReadVarUint(r, 0)
	if err != nil {
		return nil, err
	}

	// read unspent list in store
	unspents := make([]*utxo, listNum)
	for i := 0; i < int(listNum); i++ {
		var u utxo
		err := u.Deserialize(r)
		if err != nil {
			return nil, err
		}

		unspents[i] = &u
	}

	return unspents, nil
}

func (c *TokenChainStore) PersistUnspentWithProgramHash(batch database.Batch, programHash Uint168, assetid Uint256, height uint32, unspents []*utxo) error {
	prefix := []byte{byte(IX_Unspent_UTXO)}
	prefix = append(prefix, programHash.Bytes()...)
	prefix = append(prefix, assetid.Bytes()...)
	key := bytes.NewBuffer(prefix)
	if err := WriteUint32(key, height); err != nil {
		return err
	}

	if len(unspents) == 0 {
		batch.Delete(key.Bytes())
		return nil
	}

	listnum := len(unspents)
	w := bytes.NewBuffer(nil)
	WriteVarUint(w, uint64(listnum))
	for i := 0; i < listnum; i++ {
		unspents[i].Serialize(w)
	}

	// BATCH PUT VALUE
	batch.Put(key.Bytes(), w.Bytes())

	return nil
}

func (c *TokenChainStore) persistUnspendUTXOs(batch database.Batch, b *types.Block) error {
	unspendUTXOs := make(map[Uint168]map[Uint256]map[uint32][]*utxo)
	curHeight := b.Header.GetHeight()

	for _, txn := range b.Transactions {
		for index, output := range txn.Outputs {
			programHash := output.ProgramHash
			assetID := output.AssetID

			if _, ok := unspendUTXOs[programHash]; !ok {
				unspendUTXOs[programHash] = make(map[Uint256]map[uint32][]*utxo)
			}

			if _, ok := unspendUTXOs[programHash][assetID]; !ok {
				unspendUTXOs[programHash][assetID] = make(map[uint32][]*utxo, 0)
			}

			if _, ok := unspendUTXOs[programHash][assetID][curHeight]; !ok {
				var err error
				unspendUTXOs[programHash][assetID][curHeight], err = c.GetUnspentElementFromProgramHash(programHash, assetID, curHeight)
				if err != nil {
					unspendUTXOs[programHash][assetID][curHeight] = make([]*utxo, 0)
				}

			}
			var valueBytes []byte
			var u utxo
			if assetID.IsEqual(types.GetSystemAssetId()) {
				valueBytes, _ = output.Value.Bytes()
			} else {
				valueBytes = output.TokenValue.Bytes()
			}
			u = utxo{txn.Hash(), uint32(index), assetID, valueBytes}
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
					unspendUTXOs[programHash] = make(map[Uint256]map[uint32][]*utxo)
				}
				if _, ok := unspendUTXOs[programHash][assetID]; !ok {
					unspendUTXOs[programHash][assetID] = make(map[uint32][]*utxo)
				}

				if _, ok := unspendUTXOs[programHash][assetID][height]; !ok {
					unspendUTXOs[programHash][assetID][height], err = c.GetUnspentElementFromProgramHash(programHash, assetID, height)
					if err != nil {
						return errors.New(fmt.Sprintf("[persist] UTXOs programHash:%v, assetId:%v height:%v has no unspent utxo.", programHash, assetID, height))
					}
				}

				flag := false
				listnum := len(unspendUTXOs[programHash][assetID][height])
				for i := 0; i < listnum; i++ {
					if unspendUTXOs[programHash][assetID][height][i].TxID.IsEqual(referTxn.Hash()) && unspendUTXOs[programHash][assetID][height][i].Index == uint32(index) {
						unspendUTXOs[programHash][assetID][height][i] = unspendUTXOs[programHash][assetID][height][listnum-1]
						unspendUTXOs[programHash][assetID][height] = unspendUTXOs[programHash][assetID][height][:listnum-1]
						flag = true
						break
					}
				}
				if !flag {
					return errors.New(fmt.Sprintf("[persist] UTXOs NOT find utxo by txid: %x, index: %d.", referTxn.Hash(), index))
				}
			}
		}
	}

	// batch put the UTXOs
	for programHash, programHash_value := range unspendUTXOs {
		for assetId, unspents := range programHash_value {
			for height, unspent := range unspents {
				err := c.PersistUnspentWithProgramHash(batch, programHash, assetId, height, unspent)
				if err != nil {
					return err
				}
			}

		}
	}

	return nil
}

func (c *TokenChainStore) rollbackTransactions(batch database.Batch, b *types.Block) error {
	for _, txn := range b.Transactions {
		if err := c.RollbackTransaction(batch, txn); err != nil {
			return err
		}
		if txn.TxType == types.RegisterAsset {
			if c.systemAssetID.IsEqual(txn.Hash()) {
				if err := c.RollbackAsset(batch, txn.Hash()); err != nil {
					return err
				}
			} else {
				regPayload := txn.Payload.(*types.PayloadRegisterAsset)
				if err := c.RollbackAsset(batch, regPayload.Asset.Hash()); err != nil {
					return err
				}
			}
		}
		if txn.TxType == types.RechargeToSideChain {
			rechargePayload := txn.Payload.(*types.PayloadRechargeToSideChain)
			hash, err := rechargePayload.GetMainchainTxHash(txn.PayloadVersion)
			if err != nil {
				return err
			}
			c.RollbackMainchainTx(batch, *hash)
		}
	}

	return nil
}

func (c *TokenChainStore) rollbackUnspendUTXOs(batch database.Batch, b *types.Block) error {
	unspendUTXOs := make(map[Uint168]map[Uint256]map[uint32][]*utxo)
	height := b.Header.GetHeight()
	for _, txn := range b.Transactions {
		for index, output := range txn.Outputs {
			programHash := output.ProgramHash
			assetID := output.AssetID
			if _, ok := unspendUTXOs[programHash]; !ok {
				unspendUTXOs[programHash] = make(map[Uint256]map[uint32][]*utxo)
			}
			if _, ok := unspendUTXOs[programHash][assetID]; !ok {
				unspendUTXOs[programHash][assetID] = make(map[uint32][]*utxo)
			}
			if _, ok := unspendUTXOs[programHash][assetID][height]; !ok {
				var err error
				unspendUTXOs[programHash][assetID][height], err = c.GetUnspentElementFromProgramHash(programHash, assetID, height)
				if err != nil {
					return errors.New(fmt.Sprintf("[persist] UTXOs programHash:%v, assetId:%v has no unspent utxo.", programHash, assetID))
				}
			}
			var valueBytes []byte
			var u utxo
			if assetID.IsEqual(types.GetSystemAssetId()) {
				valueBytes, _ = output.Value.Bytes()
			} else {
				valueBytes = output.TokenValue.Bytes()
			}
			u = utxo{txn.Hash(), uint32(index), assetID, valueBytes}
			var position int
			for i, unspend := range unspendUTXOs[programHash][assetID][height] {
				if unspend.TxID == u.TxID && unspend.Index == u.Index {
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
					unspendUTXOs[programHash] = make(map[Uint256]map[uint32][]*utxo)
				}
				if _, ok := unspendUTXOs[programHash][assetID]; !ok {
					unspendUTXOs[programHash][assetID] = make(map[uint32][]*utxo)
				}
				if _, ok := unspendUTXOs[programHash][assetID][hh]; !ok {
					unspendUTXOs[programHash][assetID][hh], err = c.GetUnspentElementFromProgramHash(programHash, assetID, hh)
					if err != nil {
						unspendUTXOs[programHash][assetID][hh] = make([]*utxo, 0)
					}
				}
				var valueBytes []byte
				var u utxo
				if assetID.IsEqual(types.GetSystemAssetId()) {
					valueBytes, _ = referTxnOutput.Value.Bytes()
				} else {
					valueBytes = referTxnOutput.TokenValue.Bytes()
				}
				u = utxo{txn.Hash(), uint32(index), assetID, valueBytes}
				unspendUTXOs[programHash][assetID][hh] = append(unspendUTXOs[programHash][assetID][hh], &u)
			}
		}
	}

	for programHash, programHash_value := range unspendUTXOs {
		for assetId, unspents := range programHash_value {
			for height, unspent := range unspents {
				err := c.PersistUnspentWithProgramHash(batch, programHash, assetId, height, unspent)
				if err != nil {
					return err
				}
			}

		}
	}

	return nil
}

func (c *TokenChainStore) persistTransactions(batch database.Batch, b *types.Block) error {
	for _, txn := range b.Transactions {
		if err := c.PersistTransaction(batch, txn, b.Header.GetHeight()); err != nil {
			return err
		}
		if txn.TxType == types.RegisterAsset {
			regPayload := txn.Payload.(*types.PayloadRegisterAsset)
			if err := c.PersistAsset(batch, AssetInfo{regPayload.Asset, b.GetHeight()}); err != nil {
				return err
			}
		}
		if txn.TxType == types.RechargeToSideChain {
			rechargePayload := txn.Payload.(*types.PayloadRechargeToSideChain)
			hash, err := rechargePayload.GetMainchainTxHash(txn.PayloadVersion)
			if err != nil {
				return err
			}
			c.PersistMainchainTx(batch, *hash)
		}
	}
	return nil
}

func (c *TokenChainStore) persistUnspend(batch database.Batch, b *types.Block) error {
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
			batch.Delete(key.Bytes())
		} else {
			unspentArray := blockchain.ToByteArray(value)
			batch.Put(key.Bytes(), unspentArray)
		}
	}

	return nil
}

func (c *TokenChainStore) rollbackUnspend(batch database.Batch, b *types.Block) error {
	unspentPrefix := []byte{byte(blockchain.IX_Unspent)}
	unspents := make(map[Uint256][]uint16)
	for _, txn := range b.Transactions {
		// remove all utxos created by this transaction
		txnHash := txn.Hash()
		batch.Delete(append(unspentPrefix, txnHash.Bytes()...))
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
			batch.Delete(key.Bytes())
		} else {
			unspentArray := blockchain.ToByteArray(value)
			batch.Put(key.Bytes(), unspentArray)
		}
	}

	return nil
}

func (c *TokenChainStore) GetAsset(hash Uint256) (*AssetInfo, error) {
	assetInfo := new(AssetInfo)
	prefix := []byte{byte(blockchain.ST_Info)}
	data, err := c.Get(append(prefix, hash.Bytes()...))
	if err != nil {
		return nil, err
	}
	err = assetInfo.Deserialize(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	return assetInfo, nil
}

func (c *TokenChainStore) GetAssets() map[Uint256]AssetInfo {
	assets := make(map[Uint256]AssetInfo)

	iter := c.NewIterator([]byte{byte(blockchain.ST_Info)})
	defer iter.Release()
	for iter.Next() {
		reader := bytes.NewReader(iter.Key())

		// read prefix
		_, _ = ReadBytes(reader, 1)
		var assetID Uint256
		assetID.Deserialize(reader)

		asset := new(AssetInfo)
		asset.Deserialize(bytes.NewReader(iter.Value()))

		assets[assetID] = *asset
	}

	return assets
}

func (c *TokenChainStore) PersistAsset(batch database.Batch, asset AssetInfo) error {
	//we will not register "ELA" here, because ELA has been registered in the genesis block.
	if asset.Name == "ELA" {
		return errors.New("you can't register \"ELA\", man")
	}
	w := bytes.NewBuffer(nil)

	asset.Serialize(w)
	// generate key
	assetKey := new(bytes.Buffer)
	// add asset prefix.
	assetKey.WriteByte(byte(blockchain.ST_Info))
	// contact asset id
	assetID := asset.Hash()
	assetID.Serialize(assetKey)

	return batch.Put(assetKey.Bytes(), w.Bytes())
}

func (c *TokenChainStore) RollbackAsset(batch database.Batch, assetID Uint256) error {
	// it is impossible to rollback ela asset
	key := new(bytes.Buffer)
	key.WriteByte(byte(blockchain.ST_Info))
	assetID.Serialize(key)
	batch.Delete(key.Bytes())
	return nil
}
