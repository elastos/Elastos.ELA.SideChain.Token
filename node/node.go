package node

import (
	"crypto/sha256"
	"strconv"
	"time"
	"encoding/binary"
	"bytes"
	"fmt"
	"runtime"

	bc "github.com/elastos/Elastos.ELA.SideChain.Token/blockchain"

	"github.com/elastos/Elastos.ELA.SideChain/protocol"
	"github.com/elastos/Elastos.ELA.SideChain/node"
	"github.com/elastos/Elastos.ELA.SideChain/log"
	"github.com/elastos/Elastos.ELA.SideChain/core"
	. "github.com/elastos/Elastos.ELA.SideChain/errors"
	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
	. "github.com/elastos/Elastos.ELA.Utility/common"
	"github.com/elastos/Elastos.ELA.SideChain/config"
	"github.com/elastos/Elastos.ELA.Utility/p2p"
)



type TokenNode struct {
	node.Node
}

func (n *TokenNode) AppendToTxnPool(txn *core.Transaction) ErrCode {
	//verify transaction with Concurrency
	if errCode := blockchain.TransactionValidator.CheckTransactionSanity(txn); errCode != Success {
		log.Info("Transaction verification failed", txn.Hash())
		return errCode
	}
	if errCode := blockchain.TransactionValidator.CheckTransactionContext(txn); errCode != Success {
		log.Info("Transaction verification with ledger failed", txn.Hash())
		return errCode
	}
	//verify transaction by pool with lock
	if errCode := n.VerifyTransactionWithTxnPool(txn); errCode != Success {
		log.Warn("[TxPool verifyTransactionWithTxnPool] failed", txn.Hash())
		return errCode
	}

	txn.Fee = Fixed64(bc.TxFeeHelper.GetTxFee(txn, blockchain.DefaultLedger.Blockchain.AssetID).Int64())
	buf := new(bytes.Buffer)
	txn.Serialize(buf)
	txn.FeePerKB = txn.Fee * 1000 / Fixed64(len(buf.Bytes()))
	//add the transaction to process scope
	n.AddToTxList(txn)
	return Success
}

func RmNode(node *TokenNode) {
	log.Debug(fmt.Sprintf("Remove unused/deuplicate Node: 0x%0x", node.ID()))
}

func NewNode(magic uint32) *TokenNode {
	tokenNode := new(TokenNode)
	tokenNode.MsgHelper = p2p.NewMsgHelper(magic, uint32(config.Parameters.MaxBlockSize), nil, node.NewMsgHandlerV1(tokenNode))
	runtime.SetFinalizer(tokenNode, RmNode)
	return tokenNode
}

func InitLocalNode() protocol.Noder {
	node.LocalNode = NewNode(config.Parameters.Magic)
	node.LocalNode.SetVersion(protocol.ProtocolVersion)

	node.LocalNode.SetSyncBlkReqSem(protocol.MakeSemaphore(protocol.MaxSyncHdrReq))
	node.LocalNode.SetSyncHdrReqSem(protocol.MakeSemaphore(protocol.MaxSyncHdrReq))

	node.LocalNode.SetLinkPort(config.Parameters.NodePort)
	if config.Parameters.OpenService {
		node.LocalNode.SetServices(node.LocalNode.Services() + protocol.OpenService)
	}
	node.LocalNode.SetRelay(true)
	idHash := sha256.Sum256([]byte(strconv.Itoa(int(time.Now().UnixNano()))))
	id := node.LocalNode.ID()
	binary.Read(bytes.NewBuffer(idHash[:8]), binary.LittleEndian, &id)
	log.Info(fmt.Sprintf("Init Node ID to 0x%x", id))
	node.LocalNode.NbrNodesInit()
	node.LocalNode.KnownAddressListInit()
	node.LocalNode.TxPoolInit()
	node.LocalNode.EventQueueInit()
	node.LocalNode.IdCacheInit()
	node.LocalNode.CachedHashesInit()
	node.LocalNode.NodeDisconnectSubscriberInit()
	node.LocalNode.RequestedBlockListInit()
	node.LocalNode.HandshakeQueueInit()
	node.LocalNode.SyncTimerInit()
	node.LocalNode.InitConnection()

	return node.LocalNode
}
