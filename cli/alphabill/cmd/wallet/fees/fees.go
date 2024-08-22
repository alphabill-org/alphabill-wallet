package fees

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/spf13/cobra"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	clitypes "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	cliaccount "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/util/account"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/args"
	"github.com/alphabill-org/alphabill-wallet/client"
	"github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/util"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	evmwallet "github.com/alphabill-org/alphabill-wallet/wallet/evm"
	"github.com/alphabill-org/alphabill-wallet/wallet/fees"
)

// NewFeesCmd creates a new cobra command for the wallet fees component.
func NewFeesCmd(walletConfig *clitypes.WalletConfig) *cobra.Command {
	var config = &feesConfig{
		walletConfig:        walletConfig,
		targetPartitionType: clitypes.MoneyType, // shows default value in help context
	}
	var cmd = &cobra.Command{
		Use:   "fees",
		Short: "cli for managing alphabill wallet fees",
		Run: func(cmd *cobra.Command, args []string) {
			walletConfig.Base.ConsoleWriter.Println("Error: must specify a subcommand")
		},
	}
	cmd.AddCommand(listFeesCmd(config))
	cmd.AddCommand(addFeeCreditCmd(config))
	cmd.AddCommand(reclaimFeeCreditCmd(config))
	cmd.AddCommand(lockFeeCreditCmd(config))
	cmd.AddCommand(unlockFeeCreditCmd(config))

	cmd.PersistentFlags().VarP(&config.targetPartitionType, args.PartitionCmdName, "n", "partition name for which to manage fees [money|tokens|evm]")
	cmd.PersistentFlags().StringVarP(&config.moneyPartitionNodeUrl, args.RpcUrl, "r", args.DefaultMoneyRpcUrl, "money rpc node url")

	usage := fmt.Sprintf("partition rpc node url for which to manage fees (default: [%s|%s|%s] based on --partition flag)", args.DefaultMoneyRpcUrl, args.DefaultTokensRpcUrl, args.DefaultEvmRpcUrl)
	cmd.PersistentFlags().StringVarP(&config.targetPartitionNodeUrl, args.PartitionRpcUrlCmdName, "m", "", usage)
	return cmd
}

func addFeeCreditCmd(config *feesConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "adds fee credit to the wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return addFeeCreditCmdExec(cmd, config)
		},
	}
	cmd.Flags().Uint64P(args.KeyCmdName, "k", 1, "specifies to which account to add the fee credit")
	cmd.Flags().StringP(args.AmountCmdName, "v", "1", "specifies how much fee credit to create in ALPHA")
	args.AddMaxFeeFlag(cmd, cmd.Flags())
	return cmd
}

func addFeeCreditCmdExec(cmd *cobra.Command, config *feesConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	amountString, err := cmd.Flags().GetString(args.AmountCmdName)
	if err != nil {
		return err
	}
	maxFee, err := args.ParseMaxFeeFlag(cmd)
	if err != nil {
		return err
	}

	walletConfig := config.walletConfig
	am, err := cliaccount.LoadExistingAccountManager(walletConfig)
	if err != nil {
		return fmt.Errorf("failed to load account manager: %w", err)
	}
	defer am.Close()

	feeManagerDB, err := fees.NewFeeManagerDB(walletConfig.WalletHomeDir)
	if err != nil {
		return fmt.Errorf("failed to create fee manager db: %w", err)
	}
	defer feeManagerDB.Close()
	fm, err := getFeeCreditManager(cmd.Context(), config, am, feeManagerDB, maxFee, walletConfig.Base.Logger)
	if err != nil {
		return fmt.Errorf("failed to create fee credit manager: %w", err)
	}
	defer fm.Close()

	return addFees(cmd.Context(), accountNumber, amountString, config, fm, walletConfig.Base.ConsoleWriter)
}

func listFeesCmd(config *feesConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "lists fee credit of the wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listFeesCmdExec(cmd, config)
		},
	}
	cmd.Flags().Uint64P(args.KeyCmdName, "k", 0, "specifies which account fee bills to list (default: all accounts)")
	return cmd
}

func listFeesCmdExec(cmd *cobra.Command, config *feesConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	walletConfig := config.walletConfig
	am, err := cliaccount.LoadExistingAccountManager(walletConfig)
	if err != nil {
		return fmt.Errorf("failed to load account manager: %w", err)
	}
	defer am.Close()

	feeManagerDB, err := fees.NewFeeManagerDB(walletConfig.WalletHomeDir)
	if err != nil {
		return fmt.Errorf("failed to create fee manager db: %w", err)
	}
	defer feeManagerDB.Close()

	fm, err := getFeeCreditManager(cmd.Context(), config, am, feeManagerDB, 0, walletConfig.Base.Logger)
	if err != nil {
		return err
	}
	defer fm.Close()

	return listFees(cmd.Context(), accountNumber, am, config, fm, walletConfig.Base.ConsoleWriter)
}

func reclaimFeeCreditCmd(config *feesConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reclaim",
		Short: "reclaims fee credit of the wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return reclaimFeeCreditCmdExec(cmd, config)
		},
	}
	cmd.Flags().Uint64P(args.KeyCmdName, "k", 1, "specifies to which account to reclaim the fee credit")
	args.AddMaxFeeFlag(cmd, cmd.Flags())
	return cmd
}

func reclaimFeeCreditCmdExec(cmd *cobra.Command, config *feesConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	maxFee, err := args.ParseMaxFeeFlag(cmd)
	if err != nil {
		return err
	}

	walletConfig := config.walletConfig
	am, err := cliaccount.LoadExistingAccountManager(walletConfig)
	if err != nil {
		return fmt.Errorf("failed to load account manager: %w", err)
	}
	defer am.Close()

	feeManagerDB, err := fees.NewFeeManagerDB(walletConfig.WalletHomeDir)
	if err != nil {
		return fmt.Errorf("failed to create fee manager db: %w", err)
	}
	defer feeManagerDB.Close()

	fm, err := getFeeCreditManager(cmd.Context(), config, am, feeManagerDB, maxFee, walletConfig.Base.Logger)
	if err != nil {
		return err
	}
	defer fm.Close()

	return reclaimFees(cmd.Context(), accountNumber, config, fm, walletConfig.Base.ConsoleWriter)
}

func lockFeeCreditCmd(config *feesConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lock",
		Short: "locks fee credit of the wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return lockFeeCreditCmdExec(cmd, config)
		},
	}
	cmd.Flags().Uint64P(args.KeyCmdName, "k", 0, "specifies which account fee credit record to lock")
	args.AddMaxFeeFlag(cmd, cmd.Flags())
	_ = cmd.MarkFlagRequired(args.KeyCmdName)
	return cmd
}

func lockFeeCreditCmdExec(cmd *cobra.Command, config *feesConfig) error {
	if config.targetPartitionType == clitypes.EvmType {
		return errors.New("locking fee credit is not supported for EVM partition")
	}
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	if accountNumber == 0 {
		return errors.New("account number must be greater than zero")
	}
	maxFee, err := args.ParseMaxFeeFlag(cmd)
	if err != nil {
		return err
	}

	walletConfig := config.walletConfig
	am, err := cliaccount.LoadExistingAccountManager(walletConfig)
	if err != nil {
		return fmt.Errorf("failed to load account manager: %w", err)
	}
	defer am.Close()

	feeManagerDB, err := fees.NewFeeManagerDB(walletConfig.WalletHomeDir)
	if err != nil {
		return fmt.Errorf("failed to create fee manager db: %w", err)
	}
	defer feeManagerDB.Close()

	fm, err := getFeeCreditManager(cmd.Context(), config, am, feeManagerDB, maxFee, walletConfig.Base.Logger)
	if err != nil {
		return err
	}
	defer fm.Close()

	_, err = fm.LockFeeCredit(cmd.Context(), fees.LockFeeCreditCmd{AccountIndex: accountNumber - 1, LockStatus: wallet.LockReasonManual})
	if err != nil {
		return fmt.Errorf("failed to lock fee credit: %w", err)
	}
	walletConfig.Base.ConsoleWriter.Println("Fee credit record locked successfully.")
	return nil
}

func unlockFeeCreditCmd(config *feesConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unlock",
		Short: "unlocks fee credit of the wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return unlockFeeCreditCmdExec(cmd, config)
		},
	}
	cmd.Flags().Uint64P(args.KeyCmdName, "k", 0, "specifies which account fee credit record to unlock")
	args.AddMaxFeeFlag(cmd, cmd.Flags())
	_ = cmd.MarkFlagRequired(args.KeyCmdName)
	return cmd
}

func unlockFeeCreditCmdExec(cmd *cobra.Command, config *feesConfig) error {
	if config.targetPartitionType == clitypes.EvmType {
		return errors.New("locking fee credit is not supported for EVM partition")
	}
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	if accountNumber == 0 {
		return errors.New("account number must be greater than zero")
	}
	maxFee, err := args.ParseMaxFeeFlag(cmd)
	if err != nil {
		return err
	}

	walletConfig := config.walletConfig
	am, err := cliaccount.LoadExistingAccountManager(walletConfig)
	if err != nil {
		return fmt.Errorf("failed to load account manager: %w", err)
	}
	defer am.Close()

	feeManagerDB, err := fees.NewFeeManagerDB(walletConfig.WalletHomeDir)
	if err != nil {
		return fmt.Errorf("failed to create fee manager db: %w", err)
	}
	defer feeManagerDB.Close()

	fm, err := getFeeCreditManager(cmd.Context(), config, am, feeManagerDB, maxFee, walletConfig.Base.Logger)
	if err != nil {
		return err
	}
	defer fm.Close()

	_, err = fm.UnlockFeeCredit(cmd.Context(), fees.UnlockFeeCreditCmd{AccountIndex: accountNumber - 1})
	if err != nil {
		return fmt.Errorf("failed to unlock fee credit: %w", err)
	}
	walletConfig.Base.ConsoleWriter.Println("Fee credit record unlocked successfully.")
	return nil
}

type FeeCreditManager interface {
	GetFeeCredit(ctx context.Context, cmd fees.GetFeeCreditCmd) (*types.FeeCreditRecord, error)
	AddFeeCredit(ctx context.Context, cmd fees.AddFeeCmd) (*fees.AddFeeCmdResponse, error)
	ReclaimFeeCredit(ctx context.Context, cmd fees.ReclaimFeeCmd) (*fees.ReclaimFeeCmdResponse, error)
	LockFeeCredit(ctx context.Context, cmd fees.LockFeeCreditCmd) (*types.Proof, error)
	UnlockFeeCredit(ctx context.Context, cmd fees.UnlockFeeCreditCmd) (*types.Proof, error)
	MinAddFeeAmount() uint64
	MinReclaimFeeAmount() uint64
	Close()
}

func listFees(ctx context.Context, accountNumber uint64, am account.Manager, c *feesConfig, w FeeCreditManager, consoleWriter clitypes.ConsoleWrapper) error {
	if accountNumber == 0 {
		pubKeys, err := am.GetPublicKeys()
		if err != nil {
			return err
		}
		consoleWriter.Println("Partition: " + c.targetPartitionType)
		for accountIndex := range pubKeys {
			fcr, err := w.GetFeeCredit(ctx, fees.GetFeeCreditCmd{AccountIndex: uint64(accountIndex)})
			if err != nil {
				return err
			}
			accNum := accountIndex + 1
			balance := uint64(0)
			if fcr != nil {
				balance = fcr.Balance
			}
			amountString := util.AmountToString(balance, 8)
			consoleWriter.Println(fmt.Sprintf("Account #%d %s%s", accNum, amountString, getLockedReasonString(fcr)))
		}
	} else {
		accountIndex := accountNumber - 1
		fcr, err := w.GetFeeCredit(ctx, fees.GetFeeCreditCmd{AccountIndex: accountIndex})
		if err != nil {
			return err
		}
		amountString := util.AmountToString(fcr.Balance, 8)
		consoleWriter.Println("Partition: " + c.targetPartitionType)
		consoleWriter.Println(fmt.Sprintf("Account #%d %s%s", accountNumber, amountString, getLockedReasonString(fcr)))
	}
	return nil
}

func addFees(ctx context.Context, accountNumber uint64, amountString string, c *feesConfig, w FeeCreditManager, consoleWriter clitypes.ConsoleWrapper) error {
	amount, err := util.StringToAmount(amountString, 8)
	if err != nil {
		return err
	}
	rsp, err := w.AddFeeCredit(ctx, fees.AddFeeCmd{
		Amount:         amount,
		AccountIndex:   accountNumber - 1,
		DisableLocking: c.targetPartitionType == clitypes.EvmType,
	})
	if err != nil {
		if errors.Is(err, fees.ErrMinimumFeeAmount) {
			return fmt.Errorf("minimum fee credit amount to add is %s", util.AmountToString(w.MinAddFeeAmount(), 8))
		}
		if errors.Is(err, fees.ErrInsufficientBalance) {
			return fmt.Errorf("insufficient balance for transaction. Bills smaller than the minimum amount (%s) are not counted", util.AmountToString(w.MinAddFeeAmount(), 8))
		}
		if errors.Is(err, fees.ErrInvalidPartition) {
			return fmt.Errorf("pending fee process exists for another partition, run the command for the correct partition: %w", err)
		}
		return err
	}
	var feeSum uint64
	for _, proof := range rsp.Proofs {
		feeSum += proof.GetFees()
	}
	consoleWriter.Println("Successfully created", amountString, "fee credits on", c.targetPartitionType, "partition.")
	consoleWriter.Println("Paid", util.AmountToString(feeSum, 8), "ALPHA fee for transactions.")
	return nil
}

func reclaimFees(ctx context.Context, accountNumber uint64, c *feesConfig, w FeeCreditManager, consoleWriter clitypes.ConsoleWrapper) error {
	rsp, err := w.ReclaimFeeCredit(ctx, fees.ReclaimFeeCmd{
		AccountIndex: accountNumber - 1,
	})
	if err != nil {
		if errors.Is(err, fees.ErrMinimumFeeAmount) {
			return fmt.Errorf("insufficient fee credit balance. Minimum amount is %s", util.AmountToString(w.MinReclaimFeeAmount(), 8))
		}
		if errors.Is(err, fees.ErrInvalidPartition) {
			return fmt.Errorf("wallet contains locked bill for different partition, run the command for the correct partition: %w", err)
		}
		return err
	}
	consoleWriter.Println("Successfully reclaimed fee credits on", c.targetPartitionType, "partition.")
	consoleWriter.Println("Paid", util.AmountToString(rsp.Proofs.GetFees(), 8), "ALPHA fee for transactions.")
	return nil
}

type feesConfig struct {
	walletConfig           *clitypes.WalletConfig
	moneyPartitionNodeUrl  string
	targetPartitionType    clitypes.PartitionType
	targetPartitionNodeUrl string
}

func (c *feesConfig) getMoneyRpcUrl() string {
	return args.BuildRpcUrl(c.moneyPartitionNodeUrl)
}

func (c *feesConfig) getTargetPartitionRpcUrl() string {
	return args.BuildRpcUrl(c.getTargetPartitionUrl())
}

func (c *feesConfig) getTargetPartitionUrl() string {
	if c.targetPartitionNodeUrl != "" {
		return c.targetPartitionNodeUrl
	}
	switch c.targetPartitionType {
	case clitypes.MoneyType:
		return args.DefaultMoneyRpcUrl
	case clitypes.TokensType:
		return args.DefaultTokensRpcUrl
	case clitypes.EvmType:
		return args.DefaultEvmRpcUrl
	default:
		panic("invalid \"partition\" flag value: " + c.targetPartitionType)
	}
}

// Creates a fees.FeeManager that needs to be closed with the Close() method.
// Does not close the account.Manager passed as an argument.
func getFeeCreditManager(ctx context.Context, c *feesConfig, am account.Manager, feeManagerDB fees.FeeManagerDB, maxFee uint64, logger *slog.Logger) (*fees.FeeManager, error) {
	moneyClient, err := client.NewMoneyPartitionClient(ctx, c.getMoneyRpcUrl())
	if err != nil {
		return nil, fmt.Errorf("failed to dial money rpc url: %w", err)
	}
	moneyInfo, err := moneyClient.GetNodeInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch money system info: %w", err)
	}
	moneyTypeVar := clitypes.MoneyType
	if !strings.HasPrefix(moneyInfo.Name, moneyTypeVar.String()) {
		return nil, errors.New("invalid rpc url provided for money partition")
	}

	switch c.targetPartitionType {
	case clitypes.MoneyType:
		return fees.NewFeeManager(
			am,
			feeManagerDB,
			moneyInfo.SystemID,
			moneyClient,
			money.NewFeeCreditRecordIDFromPublicKey,
			moneyInfo.SystemID,
			moneyClient,
			money.NewFeeCreditRecordIDFromPublicKey,
			maxFee,
			logger,
		), nil
	case clitypes.TokensType:
		tokensRpcUrl := c.getTargetPartitionRpcUrl()
		tokensClient, err := client.NewTokensPartitionClient(ctx, tokensRpcUrl)
		if err != nil {
			return nil, fmt.Errorf("failed to dial tokens rpc url: %w", err)
		}
		tokenInfo, err := tokensClient.GetNodeInfo(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch tokens system info: %w", err)
		}
		tokenTypeVar := clitypes.TokensType
		if !strings.HasPrefix(tokenInfo.Name, tokenTypeVar.String()) {
			return nil, errors.New("invalid rpc url provided for tokens partition")
		}
		return fees.NewFeeManager(
			am,
			feeManagerDB,
			moneyInfo.SystemID,
			moneyClient,
			money.NewFeeCreditRecordIDFromPublicKey,
			tokenInfo.SystemID,
			tokensClient,
			tokens.NewFeeCreditRecordIDFromPublicKey,
			maxFee,
			logger,
		), nil
	case clitypes.EvmType:
		evmRpcUrl := c.getTargetPartitionRpcUrl()
		evmClient, err := client.NewEvmPartitionClient(ctx, evmRpcUrl)
		if err != nil {
			return nil, fmt.Errorf("failed to dial evm rpc url: %w", err)
		}
		evmInfo, err := evmClient.GetNodeInfo(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch evm system info: %w", err)
		}
		evmTypeVar := clitypes.EvmType
		if !strings.HasPrefix(evmInfo.Name, evmTypeVar.String()) {
			return nil, errors.New("invalid validator node URL provided for evm partition")
		}
		return fees.NewFeeManager(
			am,
			feeManagerDB,
			moneyInfo.SystemID,
			moneyClient,
			money.NewFeeCreditRecordIDFromPublicKey,
			evmInfo.SystemID,
			evmClient,
			evmwallet.NewFeeCreditRecordIDFromPublicKey,
			maxFee,
			logger,
		), nil
	default:
		panic(`invalid "partition" flag value: ` + c.targetPartitionType)
	}
}

func getLockedReasonString(fcr *types.FeeCreditRecord) string {
	if fcr != nil && fcr.LockStatus != 0 {
		return fmt.Sprintf(" (%s)", wallet.LockReason(fcr.LockStatus).String())
	}
	return ""
}
