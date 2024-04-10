package args

import "strings"

const (
	RpcUrl                 = "rpc-url"
	DefaultMoneyRpcUrl     = "localhost:26866"
	DefaultTokensRpcUrl    = "localhost:28866"
	DefaultEvmRpcUrl       = "localhost:29866"
	PartitionCmdName       = "partition"
	PartitionRpcUrlCmdName = "partition-rpc-url"

	PasswordPromptUsage     = "password (interactive from prompt)"
	PasswordArgUsage        = "password (non-interactive from args)"
	SeedCmdName             = "seed"
	AddressCmdName          = "address"
	AmountCmdName           = "amount"
	PasswordPromptCmdName   = "password"
	PasswordArgCmdName      = "pn"
	WalletLocationCmdName   = "wallet-location"
	KeyCmdName              = "key"
	WaitForConfCmdName      = "wait-for-confirmation"
	TotalCmdName            = "total"
	QuietCmdName            = "quiet"
	ShowUnswappedCmdName    = "show-unswapped"
	BillIdCmdName           = "bill-id"
	SystemIdentifierCmdName = "system-identifier"
	ReferenceNumber         = "reference-number"
)

func BuildRpcUrl(url string) string {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + url
	}
	url = strings.TrimSuffix(url, "/")
	if !strings.HasSuffix(url, "/rpc") {
		url = url + "/rpc"
	}
	return url
}
