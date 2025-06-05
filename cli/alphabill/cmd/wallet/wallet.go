package wallet

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/spf13/cobra"
	"github.com/tyler-smith/go-bip39"

	sdkmoney "github.com/alphabill-org/alphabill-go-base/txsystem/money"
	sdktypes "github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	cliaccount "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/util/account"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/args"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/bills"
	clifees "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/fees"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/orchestration"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/permissioned"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/tokens"
	"github.com/alphabill-org/alphabill-wallet/client"
	"github.com/alphabill-org/alphabill-wallet/util"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/fees"
	"github.com/alphabill-org/alphabill-wallet/wallet/money"
)

// NewWalletCmd creates a new cobra command for the wallet component.
func NewWalletCmd(baseConfig *types.BaseConfiguration) *cobra.Command {
	config := &types.WalletConfig{Base: baseConfig}
	var walletCmd = &cobra.Command{
		Use:   "wallet",
		Short: "cli for managing alphabill wallet",
		PersistentPreRunE: func(ccmd *cobra.Command, args []string) error {
			// initialize config so that baseConf.HomeDir gets configured
			if err := types.InitializeConfig(ccmd, baseConfig); err != nil {
				return fmt.Errorf("initializing base configuration: %w", err)
			}

			if err := InitWalletConfig(ccmd, config); err != nil {
				return fmt.Errorf("initializing wallet configuration: %w", err)
			}
			return nil
		},
	}
	walletCmd.AddCommand(bills.NewBillsCmd(config))
	walletCmd.AddCommand(clifees.NewFeesCmd(config))
	walletCmd.AddCommand(CreateCmd(config))
	walletCmd.AddCommand(SendCmd(config))
	walletCmd.AddCommand(GetPubKeysCmd(config))
	walletCmd.AddCommand(GetBalanceCmd(config))
	walletCmd.AddCommand(CollectDustCmd(config))
	walletCmd.AddCommand(AddKeyCmd(config))
	walletCmd.AddCommand(tokens.NewTokenCmd(config))
	walletCmd.AddCommand(orchestration.NewCmd(config))
	walletCmd.AddCommand(permissioned.NewCmd(config))
	// add passwords flags for (encrypted)wallet
	//walletCmd.PersistentFlags().BoolP(passwordPromptCmdName, "p", false, passwordPromptUsage)
	//walletCmd.PersistentFlags().String(passwordArgCmdName, "", passwordArgUsage)
	walletCmd.PersistentFlags().BoolVarP(&config.PromptPassword, args.PasswordPromptCmdName, "p", false, args.PasswordPromptUsage)
	walletCmd.PersistentFlags().StringVar(&config.PasswordFromArg, args.PasswordArgCmdName, "", args.PasswordArgUsage)
	walletCmd.PersistentFlags().StringVarP(&config.WalletHomeDir, args.WalletLocationCmdName, "l", "", "wallet home directory (default $AB_HOME/wallet)")
	return walletCmd
}

func CreateCmd(config *types.WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "create",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ExecCreateCmd(cmd, config)
		},
	}
	cmd.Flags().StringP(args.SeedCmdName, "s", "", "mnemonic seed, the number of words should be 12, 15, 18, 21 or 24")
	return cmd
}

func ExecCreateCmd(cmd *cobra.Command, config *types.WalletConfig) (err error) {
	mnemonic := ""
	if cmd.Flags().Changed(args.SeedCmdName) {
		// when user omits value for "s" flag, ie by executing
		// wallet create -s --wallet-location some/path
		// then Cobra eats next param name (--wallet-location) as value for "s". So we validate the mnemonic here to
		// catch this case as otherwise we most likely get error about creating wallet db which is confusing
		if mnemonic, err = cmd.Flags().GetString(args.SeedCmdName); err != nil {
			return fmt.Errorf("failed to read the value of the %q flag: %w", args.SeedCmdName, err)
		}
		if !bip39.IsMnemonicValid(mnemonic) {
			return fmt.Errorf("invalid value %q for flag %q (mnemonic)", mnemonic, args.SeedCmdName)
		}
	}

	password, err := cliaccount.CreatePassphrase(config)
	if err != nil {
		return err
	}

	am, err := account.NewManager(config.WalletHomeDir, password, true)
	if err != nil {
		return fmt.Errorf("failed to create account manager: %w", err)
	}
	defer am.Close()

	if err := money.GenerateKeys(am, mnemonic); err != nil {
		return fmt.Errorf("failed to generate keys: %w", err)
	}

	if mnemonic == "" {
		mnemonicSeed, err := am.GetMnemonic()
		if err != nil {
			return fmt.Errorf("failed to read mnemonic created for the wallet: %w", err)
		}
		config.Base.ConsoleWriter.Println("The following mnemonic key can be used to recover your wallet. Please write it down now, and keep it in a safe, offline place.")
		config.Base.ConsoleWriter.Println("mnemonic key: " + mnemonicSeed)
	}
	return nil
}

func SendCmd(config *types.WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "send",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ExecSendCmd(cmd.Context(), cmd, config)
		},
	}
	cmd.Flags().StringSliceP(args.AddressCmdName, "a", nil, "compressed secp256k1 public key(s) of "+
		"the receiver(s) in hexadecimal format, must start with 0x and be 68 characters in length, must match with "+
		"amounts")
	cmd.Flags().StringSliceP(args.AmountCmdName, "v", nil, "the amount(s) to send to the "+
		"receiver(s), must match with addresses")
	cmd.Flags().String(args.ReferenceNumber, "", `user defined "reference number" of the transfer, up to 32 bytes. Prefix the value with "0x" `+
		"to pass hex encoded binary data, without it the value will be treated as (UTF-8 encoded) string and used as-is. "+
		"If the command results in more than one transaction all of them use the same reference number")
	cmd.Flags().StringP(args.RpcUrl, "r", args.DefaultMoneyRpcUrl, "rpc node url")
	cmd.Flags().Uint64P(args.KeyCmdName, "k", 1, "which key to use for sending the transaction")
	args.AddWaitForProofFlags(cmd, cmd.Flags())
	args.AddMaxFeeFlag(cmd, cmd.Flags())

	if err := cmd.MarkFlagRequired(args.AddressCmdName); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired(args.AmountCmdName); err != nil {
		panic(err)
	}
	return cmd
}

func ExecSendCmd(ctx context.Context, cmd *cobra.Command, config *types.WalletConfig) error {
	rpcUrl, err := cmd.Flags().GetString(args.RpcUrl)
	if err != nil {
		return err
	}
	moneyClient, err := client.NewMoneyPartitionClient(ctx, args.BuildRpcUrl(rpcUrl))
	if err != nil {
		return fmt.Errorf("failed to dial rpc url: %w", err)
	}
	defer moneyClient.Close()

	am, err := cliaccount.LoadExistingAccountManager(config)
	if err != nil {
		return err
	}
	feeManagerDB, err := fees.NewFeeManagerDB(config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer feeManagerDB.Close()

	maxFee, err := args.ParseMaxFeeFlag(cmd)
	if err != nil {
		return err
	}

	w, err := money.NewWallet(cmd.Context(), am, feeManagerDB, moneyClient, maxFee, config.Base.Logger)
	if err != nil {
		return err
	}
	defer w.Close()

	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	if accountNumber == 0 {
		return fmt.Errorf("invalid parameter for flag %q: 0 is not a valid account key", args.KeyCmdName)
	}

	waitForConf, proofFile, err := args.WaitForProofArg(cmd)
	if err != nil {
		return err
	}
	receiverPubKeys, err := cmd.Flags().GetStringSlice(args.AddressCmdName)
	if err != nil {
		return err
	}
	receiverAmounts, err := cmd.Flags().GetStringSlice(args.AmountCmdName)
	if err != nil {
		return err
	}
	receivers, err := groupPubKeysAndAmounts(receiverPubKeys, receiverAmounts)
	if err != nil {
		return err
	}
	refNumber, err := parseReferenceNumberArg(cmd)
	if err != nil {
		return err
	}
	proofs, err := w.Send(ctx, money.SendCmd{Receivers: receivers, WaitForConfirmation: waitForConf, AccountIndex: accountNumber - 1, ReferenceNumber: refNumber, MaxFee: maxFee})
	if err != nil {
		return err
	}
	if waitForConf {
		config.Base.ConsoleWriter.Println("Successfully confirmed transaction(s)")

		var feeSum uint64
		for _, proof := range proofs {
			feeSum += proof.TxRecord.ServerMetadata.GetActualFee()
		}
		config.Base.ConsoleWriter.Println("Paid", util.AmountToString(feeSum, 8), "fees for transaction(s).")
		if proofFile != "" {
			w, err := os.Create(filepath.Clean(proofFile))
			if err != nil {
				return fmt.Errorf("creating file for transaction proof: %w", err)
			}
			if err := sdktypes.Cbor.Encode(w, proofs); err != nil {
				return fmt.Errorf("encoding transaction proofs as CBOR: %w", err)
			}
			config.Base.ConsoleWriter.Println("Transaction proof(s) saved to file:" + proofFile)
		}
	} else {
		config.Base.ConsoleWriter.Println("Successfully sent transaction(s)")
	}
	return nil
}

func GetBalanceCmd(config *types.WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "get-balance",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ExecGetBalanceCmd(cmd, config)
		},
	}
	cmd.Flags().StringP(args.RpcUrl, "r", args.DefaultMoneyRpcUrl, "rpc node url")
	cmd.Flags().Uint64P(args.KeyCmdName, "k", 0, "specifies which key balance to query "+
		"(by default returns all key balances including total balance over all keys)")
	cmd.Flags().BoolP(args.TotalCmdName, "t", false,
		"if specified shows only total balance over all accounts")
	cmd.Flags().BoolP(args.QuietCmdName, "q", false, "hides info irrelevant for scripting, "+
		"e.g. account key numbers, can only be used together with key or total flag")
	cmd.Flags().BoolP(args.ShowUnswappedCmdName, "s", false, "includes unswapped dust bills in balance output")
	_ = cmd.Flags().MarkHidden(args.ShowUnswappedCmdName)
	return cmd
}

func ExecGetBalanceCmd(cmd *cobra.Command, config *types.WalletConfig) error {
	rpcUrl, err := cmd.Flags().GetString(args.RpcUrl)
	if err != nil {
		return err
	}
	moneyClient, err := client.NewMoneyPartitionClient(cmd.Context(), args.BuildRpcUrl(rpcUrl))
	if err != nil {
		return fmt.Errorf("failed to dial rpc url: %w", err)
	}
	defer moneyClient.Close()

	am, err := cliaccount.LoadExistingAccountManager(config)
	if err != nil {
		return err
	}
	defer am.Close()

	feeManagerDB, err := fees.NewFeeManagerDB(config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer feeManagerDB.Close()

	w, err := money.NewWallet(cmd.Context(), am, feeManagerDB, moneyClient, 0, config.Base.Logger)
	if err != nil {
		return err
	}
	defer w.Close()

	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	total, err := cmd.Flags().GetBool(args.TotalCmdName)
	if err != nil {
		return err
	}
	quiet, err := cmd.Flags().GetBool(args.QuietCmdName)
	if err != nil {
		return err
	}
	showUnswapped, err := cmd.Flags().GetBool(args.ShowUnswappedCmdName)
	if err != nil {
		return err
	}
	if !total && accountNumber == 0 {
		quiet = false // quiet is supposed to work only when total or key flag is provided
	}
	if accountNumber == 0 {
		totals, sum, err := w.GetBalances(cmd.Context(), money.GetBalanceCmd{CountDCBills: showUnswapped})
		if err != nil {
			return err
		}
		if !total {
			for i, v := range totals {
				config.Base.ConsoleWriter.Println(fmt.Sprintf("#%d %s", i+1, util.AmountToString(v, 8)))
			}
		}
		sumStr := util.AmountToString(sum, 8)
		if quiet {
			config.Base.ConsoleWriter.Println(sumStr)
		} else {
			config.Base.ConsoleWriter.Println(fmt.Sprintf("Total %s", sumStr))
		}
	} else {
		balance, err := w.GetBalance(cmd.Context(), money.GetBalanceCmd{AccountIndex: accountNumber - 1, CountDCBills: showUnswapped})
		if err != nil {
			return err
		}
		balanceStr := util.AmountToString(balance, 8)
		if quiet {
			config.Base.ConsoleWriter.Println(balanceStr)
		} else {
			config.Base.ConsoleWriter.Println(fmt.Sprintf("#%d %s", accountNumber, balanceStr))
		}
	}
	return nil
}

func GetPubKeysCmd(config *types.WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "get-pubkeys",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ExecGetPubKeysCmd(cmd, config)
		},
	}
	cmd.Flags().BoolP(args.QuietCmdName, "q", false, "hides info irrelevant for scripting, e.g. account key numbers")
	return cmd
}

func ExecGetPubKeysCmd(cmd *cobra.Command, config *types.WalletConfig) error {
	am, err := cliaccount.LoadExistingAccountManager(config)
	if err != nil {
		return err
	}
	defer am.Close()

	pubKeys, err := am.GetPublicKeys()
	if err != nil {
		return err
	}
	hideKeyNumber, _ := cmd.Flags().GetBool(args.QuietCmdName)
	for accIdx, accPubKey := range pubKeys {
		if hideKeyNumber {
			config.Base.ConsoleWriter.Println(hexutil.Encode(accPubKey))
		} else {
			config.Base.ConsoleWriter.Println(fmt.Sprintf("#%d %s", accIdx+1, hexutil.Encode(accPubKey)))
		}
	}
	return nil
}

func CollectDustCmd(config *types.WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collect-dust",
		Short: "consolidates bills",
		Long:  "consolidates all bills into a single bill",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ExecCollectDust(cmd, config)
		},
	}
	cmd.Flags().StringP(args.RpcUrl, "r", args.DefaultMoneyRpcUrl, "rpc node url")
	cmd.Flags().Uint64P(args.KeyCmdName, "k", 0, "which key to use for dust collection, 0 for all bills from all accounts")
	args.AddMaxFeeFlag(cmd, cmd.Flags())
	return cmd
}

func ExecCollectDust(cmd *cobra.Command, config *types.WalletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}

	rpcUrl, err := cmd.Flags().GetString(args.RpcUrl)
	if err != nil {
		return err
	}
	moneyClient, err := client.NewMoneyPartitionClient(cmd.Context(), args.BuildRpcUrl(rpcUrl))
	if err != nil {
		return fmt.Errorf("failed to dial rpc url: %w", err)
	}
	defer moneyClient.Close()

	am, err := cliaccount.LoadExistingAccountManager(config)
	if err != nil {
		return err
	}
	defer am.Close()

	feeManagerDB, err := fees.NewFeeManagerDB(config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer feeManagerDB.Close()

	maxFee, err := args.ParseMaxFeeFlag(cmd)
	if err != nil {
		return err
	}

	w, err := money.NewWallet(cmd.Context(), am, feeManagerDB, moneyClient, maxFee, config.Base.Logger)
	if err != nil {
		return err
	}
	defer w.Close()

	config.Base.ConsoleWriter.Println("Starting dust collection, this may take a while...")
	dcResults, err := w.CollectDust(cmd.Context(), accountNumber)
	if err != nil {
		config.Base.ConsoleWriter.Println("Failed to collect dust: " + err.Error())
		return err
	}
	for _, dcResult := range dcResults {
		if dcResult.DustCollectionResult != nil {
			swapTx, err := dcResult.DustCollectionResult.SwapProof.GetTransactionOrderV1()
			if err != nil {
				return fmt.Errorf("failed to get swap transaction order: %w", err)
			}
			attr := &sdkmoney.SwapDCAttributes{}
			err = swapTx.UnmarshalAttributes(attr)
			if err != nil {
				return fmt.Errorf("failed to unmarshal swap tx proof: %w", err)
			}
			feeSum, swapAmount, err := dcResult.DustCollectionResult.GetFeeSumAndSwapAmount()
			if err != nil {
				return fmt.Errorf("failed to calculate fee sum: %w", err)
			}
			config.Base.ConsoleWriter.Println(fmt.Sprintf(
				"Dust collection finished successfully on account #%d. Joined %d bills with total value of %s "+
					"ALPHA into an existing target bill with unit identifier 0x%s. Paid %s fees for transaction(s).",
				dcResult.AccountIndex+1,
				len(attr.DustTransferProofs),
				util.AmountToString(swapAmount, 8),
				swapTx.GetUnitID(),
				util.AmountToString(feeSum, 8),
			))
		} else {
			config.Base.ConsoleWriter.Println(fmt.Sprintf("Nothing to swap on account #%d", dcResult.AccountIndex+1))
		}
	}
	return nil
}

func AddKeyCmd(config *types.WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-key",
		Short: "adds the next key in the series to the wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ExecAddKeyCmd(cmd, config)
		},
	}
	return cmd
}

func ExecAddKeyCmd(cmd *cobra.Command, config *types.WalletConfig) error {
	am, err := cliaccount.LoadExistingAccountManager(config)
	if err != nil {
		return err
	}
	defer am.Close()

	accIdx, accPubKey, err := am.AddAccount()
	if err != nil {
		return err
	}
	config.Base.ConsoleWriter.Println(fmt.Sprintf("Added key #%d %s", accIdx+1, hexutil.Encode(accPubKey)))
	return nil
}

func InitWalletConfig(cmd *cobra.Command, config *types.WalletConfig) error {
	walletLocation, err := cmd.Flags().GetString(args.WalletLocationCmdName)
	if err != nil {
		return err
	}
	if walletLocation != "" {
		config.WalletHomeDir = walletLocation
	} else {
		config.WalletHomeDir = filepath.Join(config.Base.HomeDir, "wallet")
	}
	return nil
}

func groupPubKeysAndAmounts(pubKeys []string, amounts []string) ([]money.ReceiverData, error) {
	if len(pubKeys) != len(amounts) {
		return nil, fmt.Errorf("must specify the same amount of addresses and amounts (got %d vs %d)", len(pubKeys), len(amounts))
	}
	var receivers []money.ReceiverData
	for i := 0; i < len(pubKeys); i++ {
		amount, err := util.StringToAmount(amounts[i], 8)
		if err != nil {
			return nil, fmt.Errorf("invalid amount: %w", err)
		}
		pubKeyBytes, err := hexutil.Decode(pubKeys[i])
		if err != nil {
			return nil, fmt.Errorf("invalid address format: %s", pubKeys[i])
		}
		receivers = append(receivers, money.ReceiverData{
			Amount: amount,
			PubKey: pubKeyBytes,
		})
	}
	return receivers, nil
}

func parseReferenceNumberArg(cmd *cobra.Command) ([]byte, error) {
	input, err := cmd.Flags().GetString(args.ReferenceNumber)
	if err != nil {
		return nil, fmt.Errorf("reading %q flag: %w", args.ReferenceNumber, err)
	}

	return parseReferenceNumber(input)
}

func parseReferenceNumber(input string) (ref []byte, err error) {
	if strings.HasPrefix(input, "0x") {
		if ref, err = hex.DecodeString(input[2:]); err != nil {
			return nil, fmt.Errorf("decoding reference number from hex string to binary: %w", err)
		}
	} else {
		ref = []byte(input)
	}

	if n := len(ref); n > 32 {
		return nil, fmt.Errorf("maximum allowed length of the reference number is 32 bytes, argument is %d bytes", n)
	}
	return ref, nil
}
