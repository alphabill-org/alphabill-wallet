package fees

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	basetypes "github.com/alphabill-org/alphabill-go-base/types"
	clitypes "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	cliaccount "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/util/account"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/args"
	"github.com/alphabill-org/alphabill-wallet/client"
	"github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/util"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/fees"
	"github.com/spf13/cobra"
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

	cmd.PersistentFlags().StringVarP(&config.moneyPartitionNodeUrl, args.RpcUrl, "r", args.DefaultMoneyRpcUrl, "money rpc node url")
	cmd.PersistentFlags().VarP(&config.targetPartitionType, args.PartitionCmdName, "n", "partition name for which to manage fees [money|tokens|enterprise-tokens]")
	usage := fmt.Sprintf("partition rpc node url for which to manage fees (default: [%s|%s|%s|%s] based on --partition flag)", args.DefaultMoneyRpcUrl, args.DefaultTokensRpcUrl, args.DefaultEnterpriseTokensRpcUrl)
	cmd.PersistentFlags().StringVarP(&config.targetPartitionNodeUrl, args.PartitionRpcUrlCmdName, "m", "", usage)
	return cmd
}

func addFeeCreditCmd(config *feesConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "adds fee credit to the wallet (permissionless partitions only)",
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
	if config.targetPartitionType == clitypes.EnterpriseTokensType {
		return fmt.Errorf("adding fee credit is not supported for %s partition", config.targetPartitionType.String())
	}

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
	cmd.Flags().BoolP(args.FcrIdCmdName, "i", false, "include FCR IDs in output")
	return cmd
}

func listFeesCmdExec(cmd *cobra.Command, config *feesConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	listFcrIds, err := cmd.Flags().GetBool(args.FcrIdCmdName)
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

	return listFees(cmd.Context(), accountNumber, listFcrIds, am, config, fm, walletConfig.Base.ConsoleWriter)
}

func reclaimFeeCreditCmd(config *feesConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reclaim",
		Short: "reclaims fee credit of the wallet (permissionless partitions only)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return reclaimFeeCreditCmdExec(cmd, config)
		},
	}
	cmd.Flags().Uint64P(args.KeyCmdName, "k", 1, "specifies to which account to reclaim the fee credit")
	args.AddMaxFeeFlag(cmd, cmd.Flags())
	return cmd
}

func reclaimFeeCreditCmdExec(cmd *cobra.Command, config *feesConfig) error {
	if config.targetPartitionType == clitypes.EnterpriseTokensType {
		return fmt.Errorf("reclaiming fee credit is not supported for %s partition", config.targetPartitionType.String())
	}
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
		Short: "locks fee credit of the wallet (permissionless partitions only)",
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
	if config.targetPartitionType == clitypes.EnterpriseTokensType {
		return fmt.Errorf("locking fee credit is not supported for %s partition", config.targetPartitionType.String())
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

	_, err = fm.LockFeeCredit(cmd.Context(), fees.LockFeeCreditCmd{AccountIndex: accountNumber - 1})
	if err != nil {
		return fmt.Errorf("failed to lock fee credit: %w", err)
	}
	walletConfig.Base.ConsoleWriter.Println("Fee credit record locked successfully.")
	return nil
}

func unlockFeeCreditCmd(config *feesConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unlock",
		Short: "unlocks fee credit of the wallet (permissionless partitions only)",
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
	if config.targetPartitionType == clitypes.EnterpriseTokensType {
		return fmt.Errorf("locking fee credit is not supported for %s partition", config.targetPartitionType.String())
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
	LockFeeCredit(ctx context.Context, cmd fees.LockFeeCreditCmd) (*basetypes.TxRecordProof, error)
	UnlockFeeCredit(ctx context.Context, cmd fees.UnlockFeeCreditCmd) (*basetypes.TxRecordProof, error)
	MinAddFeeAmount() uint64
	MinReclaimFeeAmount() uint64
	Close()
}

func listFees(ctx context.Context, accountNumber uint64, listFcrIds bool, am account.Manager, c *feesConfig, w FeeCreditManager, consoleWriter clitypes.ConsoleWrapper) error {
	consoleWriter.Println("Partition: " + c.targetPartitionType)
	if accountNumber == 0 {
		pubKeys, err := am.GetPublicKeys()
		if err != nil {
			return err
		}
		for accountIndex := range pubKeys {
			accountInfo, err := getAccountInfo(uint64(accountIndex), listFcrIds, ctx, w)
			if err != nil {
				return err
			}
			consoleWriter.Println(accountInfo.String())
		}
		return nil
	}
	accountIndex := accountNumber - 1
	accountInfo, err := getAccountInfo(accountIndex, listFcrIds, ctx, w)
	if err != nil {
		return err
	}
	consoleWriter.Println(accountInfo.String())
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
		DisableLocking: false,
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
	case clitypes.EnterpriseTokensType:
		return args.DefaultEnterpriseTokensRpcUrl
	default:
		panic("invalid \"partition\" flag value: " + c.targetPartitionType)
	}
}

// Creates a fees.FeeManager that needs to be closed with the Close() method.
// Does not close the account.Manager passed as an argument.
func getFeeCreditManager(ctx context.Context, c *feesConfig, am account.Manager, feeManagerDB fees.FeeManagerDB, maxFee uint64, logger *slog.Logger) (*fees.FeeManager, error) {
	switch c.targetPartitionType {
	case clitypes.MoneyType:
		moneyClient, err := client.NewMoneyPartitionClient(ctx, c.getMoneyRpcUrl())
		if err != nil {
			return nil, fmt.Errorf("failed to create money rpc client: %w", err)
		}
		pdr, err := moneyClient.PartitionDescription(ctx)
		if err != nil {
			return nil, fmt.Errorf("loading PDR: %w", err)
		}
		fcrGenerator := func(shard basetypes.ShardID, pubKey []byte, latestAdditionTime uint64) (basetypes.UnitID, error) {
			return money.NewFeeCreditRecordIDFromPublicKey(pdr, shard, pubKey, latestAdditionTime)
		}
		return fees.NewFeeManager(
			pdr.NetworkID,
			am,
			feeManagerDB,
			pdr.PartitionID,
			moneyClient,
			fcrGenerator,
			pdr.PartitionID,
			moneyClient,
			fcrGenerator,
			maxFee,
			logger,
		), nil
	case clitypes.TokensType:
		tokensClient, err := client.NewTokensPartitionClient(ctx, c.getTargetPartitionRpcUrl())
		if err != nil {
			return nil, fmt.Errorf("failed to dial tokens rpc url: %w", err)
		}
		tokenPDR, err := tokensClient.PartitionDescription(ctx)
		if err != nil {
			return nil, fmt.Errorf("loading tokens PDR: %w", err)
		}
		moneyClient, err := client.NewMoneyPartitionClient(ctx, c.getMoneyRpcUrl())
		if err != nil {
			return nil, fmt.Errorf("failed to create money rpc client: %w", err)
		}
		moneyPDR, err := moneyClient.PartitionDescription(ctx)
		if err != nil {
			return nil, fmt.Errorf("loading money PDR: %w", err)
		}
		if moneyPDR.NetworkID != tokenPDR.NetworkID {
			return nil, errors.New("money and tokens rpc clients must be in the same network")
		}
		return fees.NewFeeManager(
			moneyPDR.NetworkID,
			am,
			feeManagerDB,
			moneyPDR.PartitionID,
			moneyClient,
			func(shard basetypes.ShardID, pubKey []byte, latestAdditionTime uint64) (basetypes.UnitID, error) {
				return money.NewFeeCreditRecordIDFromPublicKey(moneyPDR, shard, pubKey, latestAdditionTime)
			},
			tokenPDR.PartitionID,
			tokensClient,
			func(shard basetypes.ShardID, pubKey []byte, latestAdditionTime uint64) (basetypes.UnitID, error) {
				return tokens.NewFeeCreditRecordIDFromPublicKey(tokenPDR, shard, pubKey, latestAdditionTime)
			},
			maxFee,
			logger,
		), nil
	case clitypes.EnterpriseTokensType:
		tokensRpcUrl := c.getTargetPartitionRpcUrl()
		tokensClient, err := client.NewTokensPartitionClient(ctx, tokensRpcUrl)
		if err != nil {
			return nil, fmt.Errorf("failed to dial tokens rpc url: %w", err)
		}
		pdr, err := tokensClient.PartitionDescription(ctx)
		if err != nil {
			return nil, fmt.Errorf("loading PDR: %w", err)
		}
		return fees.NewFeeManager(
			pdr.NetworkID,
			am,
			feeManagerDB,
			0,
			nil,
			nil,
			pdr.PartitionID,
			tokensClient,
			func(shard basetypes.ShardID, pubKey []byte, latestAdditionTime uint64) (basetypes.UnitID, error) {
				return tokens.NewFeeCreditRecordIDFromPublicKey(pdr, shard, pubKey, latestAdditionTime)
			},
			maxFee,
			logger,
		), nil
	default:
		panic(`invalid "partition" flag value: ` + c.targetPartitionType)
	}
}

func getAccountInfo(accountIndex uint64, showFcrId bool, ctx context.Context, w FeeCreditManager) (*AccountInfoWrapper, error) {
	fcr, err := w.GetFeeCredit(ctx, fees.GetFeeCreditCmd{AccountIndex: accountIndex})
	if err != nil {
		return nil, err
	}
	var balance uint64
	var fcrId basetypes.UnitID
	if fcr != nil {
		balance = fcr.Balance
		if showFcrId {
			fcrId = fcr.ID
		}
	}
	return &AccountInfoWrapper{
		AccountNumber: accountIndex + 1,
		FcrId:         fcrId,
		Balance:       balance,
		LockedReason:  getLockedReasonString(fcr),
	}, nil
}

func getLockedReasonString(fcr *types.FeeCreditRecord) string {
	if fcr == nil || fcr.StateLockTx == nil {
		return ""
	}
	return fmt.Sprintf(" locked='0x%X'", fcr.StateLockTx)
}

type AccountInfoWrapper struct {
	AccountNumber uint64
	FcrId         basetypes.UnitID
	Balance       uint64
	LockedReason  string
}

func (a AccountInfoWrapper) String() string {
	accountAmount := util.AmountToString(a.Balance, 8)
	if a.FcrId == nil {
		return fmt.Sprintf("Account #%d %s%s", a.AccountNumber, accountAmount, a.LockedReason)
	} else {
		return fmt.Sprintf("Account #%d 0x%s %s%s", a.AccountNumber, a.FcrId, accountAmount, a.LockedReason)
	}
}
