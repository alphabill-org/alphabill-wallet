package evm

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"

	"github.com/alphabill-org/alphabill/txsystem/evm"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/util/account"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/args"
	"github.com/alphabill-org/alphabill-wallet/util"
	evmwallet "github.com/alphabill-org/alphabill-wallet/wallet/evm"
	evmclient "github.com/alphabill-org/alphabill-wallet/wallet/evm/client"
)

const (
	DataCmdName       = "data"
	MaxGasCmdName     = "max-gas"
	ValueCmdName      = "value"
	ScSizeLimit24Kb   = 24 * 1024
	DefaultEvmAddrLen = 20
	DefaultCallMaxGas = 50000000

	AlphabillApiURLCmdName = "alphabill-api-uri"
	DefaultEvmNodeRestURL  = "localhost:29866"
)

func NewEvmCmd(config *types.WalletConfig) *cobra.Command {
	evmConfig := &types.EvmConfig{WalletConfig: config}
	cmd := &cobra.Command{
		Use:   "evm",
		Short: "interact with alphabill EVM partition",
	}
	cmd.AddCommand(evmCmdDeploy(evmConfig))
	cmd.AddCommand(evmCmdExecute(evmConfig))
	cmd.AddCommand(evmCmdCall(evmConfig))
	cmd.AddCommand(evmCmdBalance(evmConfig))
	cmd.PersistentFlags().StringVarP(&evmConfig.NodeURL, AlphabillApiURLCmdName, "r", DefaultEvmNodeRestURL, "alphabill EVM partition node REST URI to connect to")
	return cmd
}

func evmCmdDeploy(config *types.EvmConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "deploys a new smart contract on evm partition by sending a transaction on the block chain",
		Long: "Executes smart contract deployment by sending a transaction on the block chain." +
			"On success the new smart contract address is printed as result and it can be used to execute/call smart contract functions",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execEvmCmdDeploy(cmd, config)
		},
	}
	// account from which to call - pay for the transaction
	cmd.Flags().Uint64P(args.KeyCmdName, "k", 1, "which key to use for sending the transaction")
	// data - smart contract code
	cmd.Flags().String(DataCmdName, "", "contract code as hex string")
	// max-gas
	cmd.Flags().Uint64(MaxGasCmdName, 0, "maximum amount of gas user is willing to spend")
	if err := cmd.MarkFlagRequired(DataCmdName); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired(MaxGasCmdName); err != nil {
		panic(err)
	}
	return cmd
}

func evmCmdExecute(config *types.EvmConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "execute",
		Short: "executes smart contract call by sending a transaction on the block chain",
		Long: "Executes smart contract call by sending a transaction on the block chain." +
			"State changes are persisted and result is stored in block chain",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execEvmCmdExecute(cmd, config)
		},
	}
	// account from which to call - pay for the transaction
	cmd.Flags().Uint64P(args.KeyCmdName, "k", 1, "which key to use for sending the transaction")
	// to address - smart contract to call
	cmd.Flags().String(args.AddressCmdName, "", "smart contract address in hexadecimal format, must start with 0x and be 20 characters in length")
	// data - function ID + parameter
	cmd.Flags().String(DataCmdName, "", "4 byte function ID and optionally argument in hex")
	// max amount of gas user is willing to spend
	cmd.Flags().Uint64(MaxGasCmdName, 0, "maximum amount of gas user is willing to spend")
	if err := cmd.MarkFlagRequired(args.AddressCmdName); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired(DataCmdName); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired(MaxGasCmdName); err != nil {
		panic(err)
	}
	return cmd
}

func evmCmdCall(config *types.EvmConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "call",
		Short: "executes a smart contract call immediately without creating a transaction on the block chain",
		Long: "Executes a smart contract call immediately without creating a transaction on the block chain." +
			"State changes are not persisted and nothing is added to the block. Often used for executing read-only smart contract functions.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execEvmCmdCall(cmd, config)
		},
	}
	// account from which to call - pay for the transaction
	cmd.Flags().Uint64P(args.KeyCmdName, "k", 1, "which key to use for from address in evm call")
	// to address - smart contract to call
	cmd.Flags().String(args.AddressCmdName, "", "to address in hexadecimal format, must be 20 characters in length")
	// data
	cmd.Flags().String(DataCmdName, "", "data as hex string")
	// max amount of gas user is willing to spend
	cmd.Flags().Uint64(MaxGasCmdName, DefaultCallMaxGas, "(optional) maximum amount of gas user is willing to spend")
	// value, default 0
	cmd.Flags().Uint64(ValueCmdName, 0, "(optional) value to transfer")
	_ = cmd.Flags().MarkHidden(ValueCmdName)
	if err := cmd.MarkFlagRequired(args.AddressCmdName); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired(DataCmdName); err != nil {
		panic(err)
	}
	return cmd
}

func evmCmdBalance(config *types.EvmConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "balance",
		Short:  "get account balance",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return execEvmCmdBalance(cmd, config)
		},
	}
	// account from which to call - pay for the transaction
	cmd.Flags().Uint64P(args.KeyCmdName, "k", 1, "which key to use for balance")
	return cmd
}

func initEvmWallet(cobraCmd *cobra.Command, config *types.EvmConfig) (*evmwallet.Wallet, error) {
	uri, err := cobraCmd.Flags().GetString(AlphabillApiURLCmdName)
	if err != nil {
		return nil, err
	}
	am, err := account.LoadExistingAccountManager(config.WalletConfig)
	if err != nil {
		return nil, err
	}
	wallet, err := evmwallet.New(evm.DefaultEvmTxSystemIdentifier, uri, am)
	if err != nil {
		return nil, err
	}
	return wallet, nil
}

// readHexFlag returns nil in case array is empty (weird behaviour by cobra)
func readHexFlag(cmd *cobra.Command, flag string) ([]byte, error) {
	str, err := cmd.Flags().GetString(flag)
	if err != nil {
		return nil, err
	}
	if len(str) == 0 {
		return nil, fmt.Errorf("argument is empty")
	}
	res, err := hex.DecodeString(str)
	if err != nil {
		return nil, fmt.Errorf("hex decode error: %w", err)
	}
	return res, err
}

func execEvmCmdDeploy(cmd *cobra.Command, config *types.EvmConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return fmt.Errorf("key parameter read failed: %w", err)
	}
	w, err := initEvmWallet(cmd, config)
	if err != nil {
		return fmt.Errorf("evm wallet init failed: %w", err)
	}
	defer w.Shutdown()
	code, err := readHexFlag(cmd, DataCmdName)
	if err != nil {
		return fmt.Errorf("failed to read '%s' parameter: %w", DataCmdName, err)
	}
	if len(code) > ScSizeLimit24Kb {
		return fmt.Errorf("contract code too big, maximum size is 24Kb")
	}
	maxGas, err := cmd.Flags().GetUint64(MaxGasCmdName)
	if err != nil {
		return fmt.Errorf("failed to read '%s' parameter: %w", MaxGasCmdName, err)
	}
	attributes := &evmclient.TxAttributes{
		Data: code,
		Gas:  maxGas,
	}
	result, err := w.SendEvmTx(cmd.Context(), accountNumber, attributes)
	if err != nil {
		if errors.Is(err, evmclient.ErrNotFound) {
			return fmt.Errorf("no evm fee credit for account %d, please add", accountNumber)
		}
		return fmt.Errorf("deploy failed, %w", err)
	}
	printResult(config.WalletConfig.Base.ConsoleWriter, result)
	return nil
}

func execEvmCmdExecute(cmd *cobra.Command, config *types.EvmConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return fmt.Errorf("key parameter read failed: %w", err)
	}
	w, err := initEvmWallet(cmd, config)
	if err != nil {
		return fmt.Errorf("evm wallet init failed: %w", err)
	}
	defer w.Shutdown()
	// get to address
	toAddr, err := readHexFlag(cmd, args.AddressCmdName)
	if err != nil {
		return fmt.Errorf("failed to read '%s' parameter: %w", args.AddressCmdName, err)
	}
	if len(toAddr) != DefaultEvmAddrLen {
		return fmt.Errorf("invalid address %x, address must be 20 bytes", toAddr)
	}
	// read binary contract file
	fnIDAndArg, err := readHexFlag(cmd, DataCmdName)
	if err != nil {
		return fmt.Errorf("failed to read '%s' parameter: %w", DataCmdName, err)
	}
	maxGas, err := cmd.Flags().GetUint64(MaxGasCmdName)
	if err != nil {
		return fmt.Errorf("failed to read '%s' parameter: %w", MaxGasCmdName, err)
	}
	attributes := &evmclient.TxAttributes{
		To:   toAddr,
		Data: fnIDAndArg,
		Gas:  maxGas,
	}
	result, err := w.SendEvmTx(cmd.Context(), accountNumber, attributes)
	if err != nil {
		if errors.Is(err, evmclient.ErrNotFound) {
			return fmt.Errorf("no evm fee credit for account %d, please add", accountNumber)
		}
		return fmt.Errorf("excution failed, %w", err)
	}
	printResult(config.WalletConfig.Base.ConsoleWriter, result)
	return nil
}

func execEvmCmdCall(cmd *cobra.Command, config *types.EvmConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return fmt.Errorf("key parameter read failed: %w", err)
	}
	w, err := initEvmWallet(cmd, config)
	if err != nil {
		return fmt.Errorf("evm wallet init failed: %w", err)
	}
	defer w.Shutdown()
	// get to address
	toAddr, err := readHexFlag(cmd, args.AddressCmdName)
	if err != nil {
		return fmt.Errorf("failed to read '%s' parameter: %w", args.AddressCmdName, err)
	}
	if len(toAddr) != DefaultEvmAddrLen {
		return fmt.Errorf("invalid address %x, address must be 20 bytes", toAddr)
	}
	// data
	data, err := readHexFlag(cmd, DataCmdName)
	if err != nil {
		return fmt.Errorf("failed to read '%s' parameter: %w", DataCmdName, err)
	}
	if len(data) > ScSizeLimit24Kb {
		return fmt.Errorf("")
	}
	maxGas, err := cmd.Flags().GetUint64(MaxGasCmdName)
	if err != nil {
		return fmt.Errorf("failed to read '%s' parameter: %w", MaxGasCmdName, err)
	}
	value, err := cmd.Flags().GetUint64(ValueCmdName)
	if err != nil {
		return fmt.Errorf("failed to read '%s' parameter: %w", ValueCmdName, err)
	}
	attributes := &evmclient.CallAttributes{
		To:    toAddr,
		Data:  data,
		Value: new(big.Int).SetUint64(value),
		Gas:   maxGas,
	}
	result, err := w.EvmCall(cmd.Context(), accountNumber, attributes)
	if err != nil {
		return fmt.Errorf("call failed, %w", err)
	}
	printResult(config.WalletConfig.Base.ConsoleWriter, result)
	return nil
}

func execEvmCmdBalance(cmd *cobra.Command, config *types.EvmConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return fmt.Errorf("key parameter read failed: %w", err)
	}
	w, err := initEvmWallet(cmd, config)
	if err != nil {
		return fmt.Errorf("evm wallet init failed: %w", err)
	}
	defer w.Shutdown()
	balance, err := w.GetBalance(cmd.Context(), accountNumber)
	if err != nil {
		return fmt.Errorf("get balance failed, %w", err)
	}
	inAlpha := evmwallet.ConvertBalanceToAlpha(balance)
	balanceStr := util.AmountToString(inAlpha, 8)
	balanceEthStr := util.AmountToString(balance.Uint64(), 18)
	config.WalletConfig.Base.ConsoleWriter.Println(fmt.Sprintf("#%d %s (eth: %s)", accountNumber, balanceStr, balanceEthStr))
	return nil
}

func printResult(consoleWriter types.ConsoleWrapper, result *evmclient.Result) {
	if !result.Success {
		consoleWriter.Println(fmt.Sprintf("Evm transaction failed: %s", result.Details.ErrorDetails))
		consoleWriter.Println(fmt.Sprintf("Evm transaction processing fee: %v", util.AmountToString(result.ActualFee, 8)))
		return
	}
	consoleWriter.Println("Evm transaction succeeded")
	consoleWriter.Println(fmt.Sprintf("Evm transaction processing fee: %v", util.AmountToString(result.ActualFee, 8)))
	noContract := common.Address{} // content if no contract is deployed
	if result.Details.ContractAddr != noContract {
		consoleWriter.Println(fmt.Sprintf("Deployed smart contract address: %x", result.Details.ContractAddr))
	}
	for i, l := range result.Details.Logs {
		consoleWriter.Println(fmt.Sprintf("Evm log %v : %v", i, l))
	}
	if len(result.Details.ReturnData) > 0 {
		consoleWriter.Println(fmt.Sprintf("Evm execution returned: %X", result.Details.ReturnData))
	}
}
