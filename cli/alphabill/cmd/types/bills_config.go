package types

import "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/args"

type BillsConfig struct {
	WalletConfig  *WalletConfig
	RpcUrl        string
	Key           uint64
	ShowUnswapped bool
	BillID        BytesHex
	PartitionID   uint32
}

func (c *BillsConfig) GetRpcUrl() string {
	return args.BuildRpcUrl(c.RpcUrl)
}
