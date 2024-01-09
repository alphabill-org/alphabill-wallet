package types

type BillsConfig struct {
	WalletConfig  *WalletConfig
	NodeURL       string
	Key           uint64
	ShowUnswapped bool
	BillID        BytesHex
	SystemID      uint32
}
