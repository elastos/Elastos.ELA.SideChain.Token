package service

type AssetInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Precision   byte   `json:"precision"`
	Height      uint32 `json:"height"`
	ID          string `json:"assetid"`
}

type ServerInfo struct {
	Compile   string      `json:"compile"`   // The compile version of this server node
	Height    uint32      `json:"height"`    // The ServerNode latest block height
	Version   uint32      `json:"version"`   // The network protocol the ServerNode used
	Services  string      `json:"services"`  // The services the server supports
	Port      uint16      `json:"port"`      // The nodes's port
	RPCPort   uint16      `json:"rpcport"`   // The RPC service port
	RestPort  uint16      `json:"restport"`  // The RESTful service port
	Neighbors []*PeerInfo `json:"neighbors"` // The connected neighbor peers.
}

type PeerInfo struct {
	NetAddress     string `json:"netaddress"`
	Services       string `json:"services"`
	RelayTx        bool   `json:"relaytx"`
	LastSend       string `json:"lastsend"`
	LastRecv       string `json:"lastrecv"`
	ConnTime       string `json:"conntime"`
	TimeOffset     int64  `json:"timeoffset"`
	Version        uint32 `json:"version"`
	Inbound        bool   `json:"inbound"`
	StartingHeight uint32 `json:"startingheight"`
	LastBlock      uint32 `json:"lastblock"`
	LastPingTime   string `json:"lastpingtime"`
	LastPingMicros int64  `json:"lastpingmicros"`
}
