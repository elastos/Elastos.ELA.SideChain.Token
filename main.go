package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"time"

	bc "github.com/elastos/Elastos.ELA.SideChain.Token/blockchain"
	mp "github.com/elastos/Elastos.ELA.SideChain.Token/mempool"
	sv "github.com/elastos/Elastos.ELA.SideChain.Token/service"

	"github.com/elastos/Elastos.ELA.SideChain.Token/core"
	"github.com/elastos/Elastos.ELA.SideChain/blockchain"
	"github.com/elastos/Elastos.ELA.SideChain/mempool"
	"github.com/elastos/Elastos.ELA.SideChain/pow"
	"github.com/elastos/Elastos.ELA.SideChain/server"
	"github.com/elastos/Elastos.ELA.SideChain/service"
	"github.com/elastos/Elastos.ELA.SideChain/spv"

	"github.com/elastos/Elastos.ELA/utils/elalog"
	"github.com/elastos/Elastos.ELA/utils/http/jsonrpc"
	"github.com/elastos/Elastos.ELA/utils/signal"
)

const (
	printStateInterval = time.Minute

	DataPath = "elastos_token"
	DataDir  = "data"
	ChainDir = "chain"
	SpvDir   = "spv"
)

var (
	// Build version generated when build program.
	Version string

	// The go source code version at build.
	GoVersion string
)

func main() {
	core.Init()
	// Use all processor cores.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Block and transaction processing can cause bursty allocations.  This
	// limits the garbage collector from excessively overallocating during
	// bursts.  This value was arrived at with the help of profiling live
	// usage.
	debug.SetGCPercent(10)

	eladlog.Infof("Node version: %s", Version)
	eladlog.Info(GoVersion)

	if loadConfigErr != nil {
		eladlog.Fatalf("load config file failed %s", loadConfigErr)
		os.Exit(-1)
	}

	// listen interrupt signals.
	interrupt := signal.NewInterrupt()

	eladlog.Info("1. BlockChain init")
	chainStore, err := bc.NewChainStore(activeNetParams.GenesisBlock, activeNetParams.ElaAssetId,
		filepath.Join(DataPath, DataDir, ChainDir))
	if err != nil {
		eladlog.Fatalf("open chain store failed, %s", err)
		os.Exit(1)
	}
	defer chainStore.Close()

	eladlog.Info("2. SPV module init")
	genesisHash := activeNetParams.GenesisBlock.Hash()

	programHash, err := mempool.GenesisToProgramHash(&genesisHash)
	if err != nil {
		eladlog.Fatalf("Genesis block hash to programHash failed, %s", err)
		os.Exit(1)
	}

	genesisAddress, err := programHash.ToAddress()
	if err != nil {
		eladlog.Fatalf("Genesis program hash to address failed, %s", err)
		os.Exit(1)
	}

	spvCfg := spv.Config{
		DataDir:        filepath.Join(DataPath, DataDir, SpvDir),
		Magic:          activeNetParams.SpvParams.Magic,
		DefaultPort:    activeNetParams.SpvParams.DefaultPort,
		SeedList:       activeNetParams.SpvParams.SeedList,
		Foundation:     activeNetParams.SpvParams.Foundation,
		GenesisAddress: genesisAddress,
	}
	spvService, err := spv.NewService(&spvCfg)
	if err != nil {
		eladlog.Fatalf("SPV module initialize failed, %s", err)
		os.Exit(1)
	}

	defer spvService.Stop()
	spvService.Start()

	mempoolCfg := mp.Config{
		ChainParams: activeNetParams,
		ChainStore:  chainStore.ChainStore,
		SpvService:  spvService,
	}
	txFeeHelper := mp.NewFeeHelper(&mempoolCfg)
	mempoolCfg.FeeHelper = txFeeHelper

	txValidator := mp.NewValidator(&mempoolCfg)
	mempoolCfg.Validator = txValidator

	chainCfg := blockchain.Config{
		ChainParams:    activeNetParams,
		ChainStore:     chainStore.ChainStore,
		GetTxFee:       txFeeHelper.GetTxFee,
		CheckTxSanity:  txValidator.CheckTransactionSanity,
		CheckTxContext: txValidator.CheckTransactionContext,
	}

	chain, err := blockchain.New(&chainCfg)
	if err != nil {
		eladlog.Fatalf("BlockChain initialize failed, %s", err)
		os.Exit(1)
	}
	chainCfg.Validator = blockchain.NewValidator(chain)

	mpCfg := mempool.Config{
		ChainParams: activeNetParams,
		ChainStore:  chainStore.ChainStore,
		Validator:   txValidator,
	}
	mpCfg.FeeHelper = txFeeHelper.FeeHelper
	txPool := mempool.New(&mpCfg)

	eladlog.Info("3. Start the P2P networks")
	server, err := server.New(filepath.Join(DataPath, DataDir), chain, txPool, activeNetParams)
	if err != nil {
		eladlog.Fatalf("initialize P2P networks failed, %s", err)
		os.Exit(1)
	}
	defer server.Stop()
	server.Start()

	eladlog.Info("4. --Initialize pow service")
	powCfg := pow.Config{
		ChainParams:               activeNetParams,
		MinerAddr:                 cfg.MinerAddr,
		MinerInfo:                 cfg.MinerInfo,
		Server:                    server,
		Chain:                     chain,
		TxMemPool:                 txPool,
		TxFeeHelper:               txFeeHelper.FeeHelper,
		CreateCoinBaseTx:          pow.CreateCoinBaseTx,
		GenerateBlock:             pow.GenerateBlock,
		GenerateBlockTransactions: txFeeHelper.GenerateBlockTransactions,
	}

	powService := pow.NewService(&powCfg)
	if cfg.Mining {
		eladlog.Info("Start POW Services")
		go powService.Start()
	}

	eladlog.Info("5. --Start the RPC service")
	service := sv.NewHttpService(&service.Config{
		Server:             server,
		Chain:              chain,
		Store:              chainStore.ChainStore,
		GenesisAddress:     genesisAddress,
		TxMemPool:          txPool,
		PowService:         powService,
		SetLogLevel:        setLogLevel,
		SpvService:         spvService,
		GetBlockInfo:       service.GetBlockInfo,
		GetTransactionInfo: sv.GetTransactionInfo,
		GetTransaction:     service.GetTransaction,
		GetPayloadInfo:     sv.GetPayloadInfo,
		GetPayload:         service.GetPayload,
	}, chainStore)

	rpcServer := newJsonRpcServer(cfg.HttpJsonPort, service)
	defer rpcServer.Stop()
	go func() {
		if err := rpcServer.Start(); err != nil {
			eladlog.Errorf("Start HttpJsonRpc server failed, %s", err.Error())
		}
	}()

	if cfg.MonitorState {
		go printSyncState(chainStore.ChainStore, server)
	}

	<-interrupt.C
}

func newJsonRpcServer(port uint16, service *sv.HttpServiceExtend) *jsonrpc.Server {
	s := jsonrpc.NewServer(&jsonrpc.Config{ServePort: port,
		RpcConfiguration: cfg.RpcConfiguration,
	})

	s.RegisterAction("setloglevel", service.SetLogLevel, "level")
	s.RegisterAction("getblock", service.GetBlockByHash, "blockhash", "verbosity")
	s.RegisterAction("getcurrentheight", service.GetBlockHeight)
	s.RegisterAction("getblockhash", service.GetBlockHash, "height")
	s.RegisterAction("getconnectioncount", service.GetConnectionCount)
	s.RegisterAction("getrawmempool", service.GetTransactionPool)
	s.RegisterAction("getrawtransaction", service.GetRawTransaction, "txid", "verbose")
	s.RegisterAction("getneighbors", service.GetNeighbors)
	s.RegisterAction("getnodestate", service.GetNodeState)
	s.RegisterAction("sendrechargetransaction", service.SendRechargeToSideChainTxByHash, "txid")
	s.RegisterAction("sendrawtransaction", service.SendRawTransaction, "data")
	s.RegisterAction("getbestblockhash", service.GetBestBlockHash)
	s.RegisterAction("getblockcount", service.GetBlockCount)
	s.RegisterAction("getblockbyheight", service.GetBlockByHeight, "height")
	s.RegisterAction("getwithdrawtransactionsbyheight", service.GetWithdrawTransactionsByHeight, "height")
	s.RegisterAction("getexistdeposittransactions", service.GetExistDepositTransactions)
	s.RegisterAction("getwithdrawtransaction", service.GetWithdrawTransactionByHash, "txid")
	s.RegisterAction("submitsideauxblock", service.SubmitAuxBlock, "blockhash", "auxpow")
	s.RegisterAction("createauxblock", service.CreateAuxBlock, "paytoaddress")
	s.RegisterAction("togglemining", service.ToggleMining, "mining")
	s.RegisterAction("discretemining", service.DiscreteMining, "count")
	s.RegisterAction("getreceivedbyaddress", service.GetReceivedByAddress, "address", "assetid")
	s.RegisterAction("listunspent", service.ListUnspent, "addresses", "assetid")
	s.RegisterAction("getassetbyhash", service.GetAssetByHash, "hash")
	s.RegisterAction("getassetlist", service.GetAssetList)
	s.RegisterAction("getillegalevidencebyheight", service.GetIllegalEvidenceByHeight, "height")
	s.RegisterAction("checkillegalevidence", service.CheckIllegalEvidence, "evidence")

	return s
}

func printSyncState(db *blockchain.ChainStore, server server.Server) {
	logger := elalog.NewBackend(logWriter).Logger("STAT",
		elalog.LevelInfo)

	ticker := time.NewTicker(printStateInterval)
	defer ticker.Stop()

	for {
		var buf bytes.Buffer
		buf.WriteString("-> ")
		buf.WriteString(strconv.FormatUint(uint64(db.GetHeight()), 10))
		peers := server.ConnectedPeers()
		buf.WriteString(" [")
		for i, p := range peers {
			buf.WriteString(strconv.FormatUint(uint64(p.ToPeer().Height()), 10))
			buf.WriteString(" ")
			buf.WriteString(p.ToPeer().String())
			if i != len(peers)-1 {
				buf.WriteString(", ")
			}
		}
		buf.WriteString("]")
		logger.Info(buf.String())
		<-ticker.C
	}
}
