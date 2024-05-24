package args

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	RpcUrl                     = "rpc-url"
	DefaultMoneyRpcUrl         = "localhost:26866"
	DefaultTokensRpcUrl        = "localhost:28866"
	DefaultEvmRpcUrl           = "localhost:29866"
	DefaultOrchestrationRpcUrl = "localhost:30866"
	PartitionCmdName           = "partition"
	PartitionRpcUrlCmdName     = "partition-rpc-url"

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
	proofOutputFlagName     = "proof-output"
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

/*
AddWaitForProofFlags adds "wait-for-confirmation" and "proof-output" flags to the flagset.
*/
func AddWaitForProofFlags(cmd *cobra.Command, flags *pflag.FlagSet) {
	// use string instead of boolean as boolean requires equals sign between name and value e.g. w=[true|false]
	flags.StringP(WaitForConfCmdName, "w", "true", "waits for transaction confirmation "+
		"on the blockchain, otherwise just broadcasts the transaction")
	flags.String(proofOutputFlagName, "", `save transaction proof to the file (if the file already exists `+
		`it will be overwritten). This flag implicitly sets "`+WaitForConfCmdName+`" to "true"`)
	cmd.MarkFlagsMutuallyExclusive(WaitForConfCmdName, proofOutputFlagName)
}

/*
WaitForProofArg returns values of the "wait-for-confirmation" and "proof-output" flags.
Returns:
  - wait: true if "wait-for-confirmation" was either explicitly or implicitly (by setting
    the "proof-output" flag) set to "true";
  - filename: the absolute path of the file into which user wants the proof to be saved;
*/
func WaitForProofArg(cmd *cobra.Command) (wait bool, filename string, _ error) {
	waitForConfStr, err := cmd.Flags().GetString(WaitForConfCmdName)
	if err != nil {
		return false, "", fmt.Errorf("reading %q flag: %w", WaitForConfCmdName, err)
	}
	if wait, err = strconv.ParseBool(waitForConfStr); err != nil {
		return false, "", fmt.Errorf("parsing %q flag: %w", WaitForConfCmdName, err)
	}

	if cmd.Flags().Changed(proofOutputFlagName) {
		outputProof, err := cmd.Flags().GetString(proofOutputFlagName)
		if err != nil {
			return false, "", fmt.Errorf("reading %q flag: %w", proofOutputFlagName, err)
		}
		if filename, err = filepath.Abs(outputProof); err != nil {
			return false, "", fmt.Errorf("parsing %q flag value as file name: %w", proofOutputFlagName, err)
		}
	}

	return wait || filename != "", filename, nil
}
