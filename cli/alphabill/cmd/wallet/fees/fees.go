package fees

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
	"github.com/spf13/cobra"

	clitypes "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	cliaccount "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/util/account"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/args"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	evmwallet "github.com/alphabill-org/alphabill-wallet/wallet/evm"
	evmclient "github.com/alphabill-org/alphabill-wallet/wallet/evm/client"
	"github.com/alphabill-org/alphabill-wallet/wallet/fees"
	moneywallet "github.com/alphabill-org/alphabill-wallet/wallet/money"
	moneyclient "github.com/alphabill-org/alphabill-wallet/wallet/money/backend/client"
	tokenswallet "github.com/alphabill-org/alphabill-wallet/wallet/tokens"
	tokensclient "github.com/alphabill-org/alphabill-wallet/wallet/tokens/client"
)

// NewFeesCmd creates a new cobra command for the wallet fees component.
func NewFeesCmd(walletConfig *clitypes.WalletConfig) *cobra.Command {
	var config = &feesConfig{
		walletConfig:  walletConfig,
		partitionType: clitypes.MoneyType, // shows default value in help context
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

	cmd.PersistentFlags().VarP(&config.partitionType, args.PartitionCmdName, "n", "partition name for which to manage fees [money|tokens|evm]")
	cmd.PersistentFlags().StringVarP(&config.moneyBackendURL, args.AlphabillApiURLCmdName, "r", args.DefaultAlphabillApiURL, "wallet backend API URL")

	usage := fmt.Sprintf("partition backend url for which to manage fees (default: [%s|%s|%s] based on --partition flag)", args.DefaultAlphabillApiURL, args.DefaultTokensBackendApiURL, args.DefaultEvmNodeRestURL)
	cmd.PersistentFlags().StringVarP(&config.partitionBackendURL, args.PartitionBackendUrlCmdName, "m", "", usage)
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

	fm, err := getFeeCreditManager(cmd.Context(), config, am, feeManagerDB, config.moneyBackendURL, walletConfig.Base.Observe)
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

	fm, err := getFeeCreditManager(cmd.Context(), config, am, feeManagerDB, config.moneyBackendURL, walletConfig.Base.Observe)
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
	return cmd
}

func reclaimFeeCreditCmdExec(cmd *cobra.Command, config *feesConfig) error {
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

	fm, err := getFeeCreditManager(cmd.Context(), config, am, feeManagerDB, config.moneyBackendURL, walletConfig.Base.Observe)
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
	_ = cmd.MarkFlagRequired(args.KeyCmdName)
	return cmd
}

func lockFeeCreditCmdExec(cmd *cobra.Command, config *feesConfig) error {
	if config.partitionType == clitypes.EvmType {
		return errors.New("locking fee credit is not supported for EVM partition")
	}
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	if accountNumber == 0 {
		return errors.New("account number must be greater than zero")
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

	fm, err := getFeeCreditManager(cmd.Context(), config, am, feeManagerDB, config.moneyBackendURL, walletConfig.Base.Observe)
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
	_ = cmd.MarkFlagRequired(args.KeyCmdName)
	return cmd
}

func unlockFeeCreditCmdExec(cmd *cobra.Command, config *feesConfig) error {
	if config.partitionType == clitypes.EvmType {
		return errors.New("locking fee credit is not supported for EVM partition")
	}
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	if accountNumber == 0 {
		return errors.New("account number must be greater than zero")
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

	fm, err := getFeeCreditManager(cmd.Context(), config, am, feeManagerDB, config.moneyBackendURL, walletConfig.Base.Observe)
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
	GetFeeCredit(ctx context.Context, cmd fees.GetFeeCreditCmd) (*wallet.Bill, error)
	AddFeeCredit(ctx context.Context, cmd fees.AddFeeCmd) (*fees.AddFeeCmdResponse, error)
	ReclaimFeeCredit(ctx context.Context, cmd fees.ReclaimFeeCmd) (*fees.ReclaimFeeCmdResponse, error)
	LockFeeCredit(ctx context.Context, cmd fees.LockFeeCreditCmd) (*wallet.Proof, error)
	UnlockFeeCredit(ctx context.Context, cmd fees.UnlockFeeCreditCmd) (*wallet.Proof, error)
	Close()
}

func listFees(ctx context.Context, accountNumber uint64, am account.Manager, c *feesConfig, w FeeCreditManager, consoleWriter clitypes.ConsoleWrapper) error {
	if accountNumber == 0 {
		pubKeys, err := am.GetPublicKeys()
		if err != nil {
			return err
		}
		consoleWriter.Println("Partition: " + c.partitionType)
		for accountIndex := range pubKeys {
			fcb, err := w.GetFeeCredit(ctx, fees.GetFeeCreditCmd{AccountIndex: uint64(accountIndex)})
			if err != nil {
				return err
			}
			accNum := accountIndex + 1
			amountString := util.AmountToString(fcb.GetValue(), 8)
			consoleWriter.Println(fmt.Sprintf("Account #%d %s%s", accNum, amountString, getLockedReasonString(fcb)))
		}
	} else {
		accountIndex := accountNumber - 1
		fcb, err := w.GetFeeCredit(ctx, fees.GetFeeCreditCmd{AccountIndex: accountIndex})
		if err != nil {
			return err
		}
		amountString := util.AmountToString(fcb.GetValue(), 8)
		consoleWriter.Println("Partition: " + c.partitionType)
		consoleWriter.Println(fmt.Sprintf("Account #%d %s%s", accountNumber, amountString, getLockedReasonString(fcb)))
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
		DisableLocking: c.partitionType == clitypes.EvmType,
	})
	if err != nil {
		if errors.Is(err, fees.ErrMinimumFeeAmount) {
			return fmt.Errorf("minimum fee credit amount to add is %s", util.AmountToString(fees.MinimumFeeAmount, 8))
		}
		if errors.Is(err, fees.ErrInsufficientBalance) {
			return fmt.Errorf("insufficient balance for transaction. Bills smaller than the minimum amount (%s) are not counted", util.AmountToString(fees.MinimumFeeAmount, 8))
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
	consoleWriter.Println("Successfully created", amountString, "fee credits on", c.partitionType, "partition.")
	consoleWriter.Println("Paid", util.AmountToString(feeSum, 8), "ALPHA fee for transactions.")
	return nil
}

func reclaimFees(ctx context.Context, accountNumber uint64, c *feesConfig, w FeeCreditManager, consoleWriter clitypes.ConsoleWrapper) error {
	rsp, err := w.ReclaimFeeCredit(ctx, fees.ReclaimFeeCmd{
		AccountIndex: accountNumber - 1,
	})
	if err != nil {
		if errors.Is(err, fees.ErrMinimumFeeAmount) {
			return fmt.Errorf("insufficient fee credit balance. Minimum amount is %s", util.AmountToString(fees.MinimumFeeAmount, 8))
		}
		if errors.Is(err, fees.ErrInvalidPartition) {
			return fmt.Errorf("wallet contains locked bill for different partition, run the command for the correct partition: %w", err)
		}
		return err
	}
	consoleWriter.Println("Successfully reclaimed fee credits on", c.partitionType, "partition.")
	consoleWriter.Println("Paid", util.AmountToString(rsp.Proofs.GetFees(), 8), "ALPHA fee for transactions.")
	return nil
}

type feesConfig struct {
	walletConfig        *clitypes.WalletConfig
	moneyBackendURL     string
	partitionType       clitypes.PartitionType
	partitionBackendURL string
}

func (c *feesConfig) parsePartitionBackendURL() (*url.URL, error) {
	backendURL := c.getPartitionBackendURL()
	if !strings.HasPrefix(backendURL, "http://") && !strings.HasPrefix(backendURL, "https://") {
		backendURL = "http://" + backendURL
	}
	return url.Parse(backendURL)
}

func (c *feesConfig) getPartitionBackendURL() string {
	if c.partitionBackendURL != "" {
		return c.partitionBackendURL
	}
	switch c.partitionType {
	case clitypes.MoneyType:
		return args.DefaultAlphabillApiURL
	case clitypes.TokensType:
		return args.DefaultTokensBackendApiURL
	case clitypes.EvmType:
		return args.DefaultEvmNodeRestURL // evm does not use backend and instead talks to an actual evm node
	default:
		panic("invalid \"partition\" flag value: " + c.partitionType)
	}
}

// Creates a fees.FeeManager that needs to be closed with the Close() method.
// Does not close the account.Manager passed as an argument.
func getFeeCreditManager(ctx context.Context, c *feesConfig, am account.Manager, feeManagerDB fees.FeeManagerDB, moneyBackendURL string, obs clitypes.Observability) (FeeCreditManager, error) {
	moneyBackendClient, err := moneyclient.New(moneyBackendURL, obs)
	if err != nil {
		return nil, fmt.Errorf("failed to create money backend client: %w", err)
	}
	moneySystemInfo, err := moneyBackendClient.GetInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch money system info: %w", err)
	}
	moneyTypeVar := clitypes.MoneyType
	if !strings.HasPrefix(moneySystemInfo.Name, moneyTypeVar.String()) {
		return nil, errors.New("invalid wallet backend API URL provided for money partition")
	}
	// the info response is expected to have system ID as hex, max 4 bytes
	mSysID, err := strconv.ParseUint(moneySystemInfo.SystemID, 16, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to decode money system identifier hex: %w", err)
	}
	moneySystemID := types.SystemID(mSysID)
	moneyTxPublisher := moneywallet.NewTxPublisher(moneyBackendClient, obs.Logger())

	switch c.partitionType {
	case clitypes.MoneyType:
		return fees.NewFeeManager(
			am,
			feeManagerDB,
			moneySystemID,
			moneyTxPublisher,
			moneyBackendClient,
			moneywallet.FeeCreditRecordIDFormPublicKey,
			moneySystemID,
			moneyTxPublisher,
			moneyBackendClient,
			moneywallet.FeeCreditRecordIDFormPublicKey,
			obs.Logger(),
		), nil
	case clitypes.TokensType:
		backendURL, err := c.parsePartitionBackendURL()
		if err != nil {
			return nil, fmt.Errorf("failed to parse partition backend url: %w", err)
		}
		tokenBackendClient := tokensclient.New(*backendURL, obs)
		tokenTxPublisher := tokenswallet.NewTxPublisher(tokenBackendClient, obs.Logger())
		tokenInfo, err := tokenBackendClient.GetInfo(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch tokens system info: %w", err)
		}
		tokenTypeVar := clitypes.TokensType
		if !strings.HasPrefix(tokenInfo.Name, tokenTypeVar.String()) {
			return nil, errors.New("invalid wallet backend API URL provided for tokens partition")
		}
		tokenSystemID, err := strconv.ParseUint(tokenInfo.SystemID, 16, 32)
		if err != nil {
			return nil, fmt.Errorf("failed to decode tokens system identifier hex: %w", err)
		}
		return fees.NewFeeManager(
			am,
			feeManagerDB,
			moneySystemID,
			moneyTxPublisher,
			moneyBackendClient,
			moneywallet.FeeCreditRecordIDFormPublicKey,
			types.SystemID(tokenSystemID),
			tokenTxPublisher,
			tokenBackendClient,
			tokenswallet.FeeCreditRecordIDFromPublicKey,
			obs.Logger(),
		), nil
	case clitypes.EvmType:
		evmNodeURL, err := c.parsePartitionBackendURL()
		if err != nil {
			return nil, err
		}
		evmClient := evmclient.New(*evmNodeURL)
		evmTxPublisher := evmwallet.NewTxPublisher(evmClient)
		evmInfo, err := evmClient.GetInfo(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch evm system info: %w", err)
		}
		evmTypeVar := clitypes.EvmType
		if !strings.HasPrefix(evmInfo.Name, evmTypeVar.String()) {
			return nil, errors.New("invalid validator node URL provided for evm partition")
		}
		evmSystemID, err := strconv.ParseUint(evmInfo.SystemID, 16, 32)
		if err != nil {
			return nil, fmt.Errorf("failed to decode evm system identifier hex: %w", err)
		}
		return fees.NewFeeManager(
			am,
			feeManagerDB,
			moneySystemID,
			moneyTxPublisher,
			moneyBackendClient,
			moneywallet.FeeCreditRecordIDFormPublicKey,
			types.SystemID(evmSystemID),
			evmTxPublisher,
			evmClient,
			evmwallet.FeeCreditRecordIDFromPublicKey,
			obs.Logger(),
		), nil
	default:
		panic(`invalid "partition" flag value: ` + c.partitionType)
	}
}

func getLockedReasonString(bill *wallet.Bill) string {
	if bill.IsLocked() {
		return fmt.Sprintf(" (%s)", bill.Locked.String())
	}
	return ""
}
