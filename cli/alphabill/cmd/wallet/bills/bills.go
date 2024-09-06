package bills

import (
	"errors"
	"fmt"
	"strings"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	clitypes "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	cliaccount "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/util/account"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/args"
	"github.com/alphabill-org/alphabill-wallet/client"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/util"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/spf13/cobra"
)

// NewBillsCmd creates a new cobra command for the wallet bills component.
func NewBillsCmd(walletConfig *clitypes.WalletConfig) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "bills",
		Short: "cli for managing alphabill wallet bills and proofs",
	}
	cmd.AddCommand(listCmd(walletConfig))
	cmd.AddCommand(lockCmd(walletConfig))
	cmd.AddCommand(unlockCmd(walletConfig))
	return cmd
}

func listCmd(walletConfig *clitypes.WalletConfig) *cobra.Command {
	config := &clitypes.BillsConfig{WalletConfig: walletConfig}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "lists bill ids and values",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execListCmd(cmd, config)
		},
	}
	cmd.Flags().StringVarP(&config.RpcUrl, args.RpcUrl, "r", args.DefaultMoneyRpcUrl, "rpc node url")
	cmd.Flags().Uint64VarP(&config.Key, args.KeyCmdName, "k", 0, "specifies which account bills to list (default: all accounts)")
	cmd.Flags().BoolVarP(&config.ShowUnswapped, args.ShowUnswappedCmdName, "s", false, "includes unswapped dust bills in output")
	_ = cmd.Flags().MarkHidden(args.ShowUnswappedCmdName)
	return cmd
}

func execListCmd(cmd *cobra.Command, config *clitypes.BillsConfig) error {
	moneyClient, err := client.NewMoneyPartitionClient(cmd.Context(), config.GetRpcUrl())
	if err != nil {
		return fmt.Errorf("failed to dial money rpc: %w", err)
	}

	accountNumber := config.Key
	// TODO unswapped bills are not available from node
	// showUnswapped := config.ShowUnswapped

	am, err := cliaccount.LoadExistingAccountManager(config.WalletConfig)
	if err != nil {
		return err
	}
	defer am.Close()

	type accountBillGroup struct {
		accountIndex uint64
		pubKey       []byte
		bills        []*sdktypes.Bill
	}
	var accountBillGroups []*accountBillGroup
	if accountNumber == 0 {
		accountKeys, err := am.GetAccountKeys()
		if err != nil {
			return fmt.Errorf("failed to load account keys: %w", err)
		}
		for accountIndex, accountKey := range accountKeys {
			pubKey := accountKey.PubKey
			bills, err := moneyClient.GetBills(cmd.Context(), accountKey.PubKeyHash.Sha256)
			if err != nil {
				return fmt.Errorf("failed to fetch bills: %w", err)
			}
			accountBillGroups = append(accountBillGroups, &accountBillGroup{pubKey: pubKey, accountIndex: uint64(accountIndex), bills: bills})
		}
	} else {
		accountIndex := accountNumber - 1
		accountKey, err := am.GetAccountKey(accountIndex)
		if err != nil {
			return fmt.Errorf("failed to load account key: %w", err)
		}
		accountBills, err := moneyClient.GetBills(cmd.Context(), accountKey.PubKeyHash.Sha256)
		if err != nil {
			return fmt.Errorf("failed to fetch bills: %w", err)
		}
		accountBillGroups = append(accountBillGroups, &accountBillGroup{pubKey: accountKey.PubKey, accountIndex: accountIndex, bills: accountBills})
	}

	for _, group := range accountBillGroups {
		if len(group.bills) == 0 {
			config.WalletConfig.Base.ConsoleWriter.Println(fmt.Sprintf("Account #%d - empty", group.accountIndex+1))
		} else {
			config.WalletConfig.Base.ConsoleWriter.Println(fmt.Sprintf("Account #%d", group.accountIndex+1))
		}
		for j, bill := range group.bills {
			billValueStr := util.AmountToString(bill.Value, 8)
			config.WalletConfig.Base.ConsoleWriter.Println(fmt.Sprintf("#%d 0x%s %s%s", j+1, bill.ID.String(), billValueStr, getLockedReasonString(bill)))
		}
	}
	return nil
}

func lockCmd(walletConfig *clitypes.WalletConfig) *cobra.Command {
	config := &clitypes.BillsConfig{WalletConfig: walletConfig}
	cmd := &cobra.Command{
		Use:   "lock",
		Short: "locks specific bill",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execLockCmd(cmd, config)
		},
	}
	cmd.Flags().StringVarP(&config.RpcUrl, args.RpcUrl, "r", args.DefaultMoneyRpcUrl, "rpc node url")
	cmd.Flags().Uint64VarP(&config.Key, args.KeyCmdName, "k", 1, "account number of the bill to lock")
	cmd.Flags().Var(&config.BillID, args.BillIdCmdName, "id of the bill to lock")
	cmd.Flags().Uint32Var(&config.SystemID, args.SystemIdentifierCmdName, uint32(money.DefaultSystemID), "system identifier")
	args.AddMaxFeeFlag(cmd, cmd.Flags())
	return cmd
}

func execLockCmd(cmd *cobra.Command, config *clitypes.BillsConfig) error {
	accountNumber := config.Key
	am, err := cliaccount.LoadExistingAccountManager(config.WalletConfig)
	if err != nil {
		return fmt.Errorf("failed to load account manager: %w", err)
	}
	defer am.Close()
	accountKey, err := am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return fmt.Errorf("failed to load account key: %w", err)
	}

	moneyClient, err := client.NewMoneyPartitionClient(cmd.Context(), config.GetRpcUrl())
	if err != nil {
		return fmt.Errorf("failed to dial money rpc: %w", err)
	}

	infoResponse, err := moneyClient.GetNodeInfo(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to fetch node info rpc: %w", err)
	}
	moneyTypeVar := clitypes.MoneyType
	if !strings.HasPrefix(infoResponse.Name, moneyTypeVar.String()) {
		return errors.New("invalid rpc url provided for money partition")
	}
	maxFee, err := args.ParseMaxFeeFlag(cmd)
	if err != nil {
		return fmt.Errorf("failed to parse maxFee parameter: %w", err)
	}
	fcr, err := moneyClient.GetFeeCreditRecordByOwnerID(cmd.Context(), accountKey.PubKeyHash.Sha256)
	if err != nil {
		return fmt.Errorf("failed to fetch fee credit record: %w", err)
	}
	if fcr == nil || fcr.Balance < maxFee {
		return errors.New("not enough fee credit in wallet")
	}
	bill, err := moneyClient.GetBill(cmd.Context(), types.UnitID(config.BillID))
	if err != nil {
		return fmt.Errorf("failed to fetch bill: %w", err)
	}
	if bill == nil {
		return fmt.Errorf("bill not found")
	}
	if bill.LockStatus != 0 {
		return errors.New("bill is already locked")
	}
	roundNumber, err := moneyClient.GetRoundNumber(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to fetch round number: %w", err)
	}
	tx, err := bill.Lock(wallet.LockReasonManual,
		sdktypes.WithTimeout(roundNumber+10),
		sdktypes.WithFeeCreditRecordID(fcr.ID),
		sdktypes.WithMaxFee(maxFee),
	)
	if err != nil {
		return fmt.Errorf("failed to create lock tx: %w", err)
	}
	txSigner, err := sdktypes.NewMoneyTxSignerFromKey(accountKey.PrivKey)
	if err != nil {
		return fmt.Errorf("failed to create money tx signer: %w", err)
	}
	if err = txSigner.SignTx(tx); err != nil {
		return fmt.Errorf("failed to sign tx: %w", err)
	}

	_, err = moneyClient.ConfirmTransaction(cmd.Context(), tx, config.WalletConfig.Base.Logger)
	if err != nil {
		return fmt.Errorf("failed to send lock tx: %w", err)
	}

	config.WalletConfig.Base.ConsoleWriter.Println("Bill locked successfully.")
	return nil
}

func unlockCmd(walletConfig *clitypes.WalletConfig) *cobra.Command {
	config := &clitypes.BillsConfig{WalletConfig: walletConfig}
	cmd := &cobra.Command{
		Use:   "unlock",
		Short: "unlocks specific bill",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execUnlockCmd(cmd, config)
		},
	}
	cmd.Flags().StringVarP(&config.RpcUrl, args.RpcUrl, "r", args.DefaultMoneyRpcUrl, "rpc node url")
	cmd.Flags().Uint64VarP(&config.Key, args.KeyCmdName, "k", 1, "account number of the bill to unlock")
	cmd.Flags().Var(&config.BillID, args.BillIdCmdName, "id of the bill to unlock")
	cmd.Flags().Uint32Var(&config.SystemID, args.SystemIdentifierCmdName, uint32(money.DefaultSystemID), "system identifier")
	args.AddMaxFeeFlag(cmd, cmd.Flags())
	return cmd
}

func execUnlockCmd(cmd *cobra.Command, config *clitypes.BillsConfig) error {
	accountNumber := config.Key
	am, err := cliaccount.LoadExistingAccountManager(config.WalletConfig)
	if err != nil {
		return fmt.Errorf("failed to load account manager: %w", err)
	}
	defer am.Close()
	accountKey, err := am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return fmt.Errorf("failed to load account key: %w", err)
	}

	moneyClient, err := client.NewMoneyPartitionClient(cmd.Context(), config.GetRpcUrl())
	if err != nil {
		return fmt.Errorf("failed to dial money rpc: %w", err)
	}

	infoResponse, err := moneyClient.GetNodeInfo(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to fetch node info rpc: %w", err)
	}
	moneyTypeVar := clitypes.MoneyType
	if !strings.HasPrefix(infoResponse.Name, moneyTypeVar.String()) {
		return errors.New("invalid rpc url provided for money partition")
	}
	maxFee, err := args.ParseMaxFeeFlag(cmd)
	if err != nil {
		return fmt.Errorf("failed to parse maxFee parameter: %w", err)
	}

	fcr, err := moneyClient.GetFeeCreditRecordByOwnerID(cmd.Context(), accountKey.PubKeyHash.Sha256)
	if err != nil {
		return fmt.Errorf("failed to fetch fee credit record: %w", err)
	}
	if fcr == nil || fcr.Balance < maxFee {
		return errors.New("not enough fee credit in wallet")
	}

	bill, err := moneyClient.GetBill(cmd.Context(), types.UnitID(config.BillID))
	if err != nil {
		return fmt.Errorf("failed to fetch bill: %w", err)
	}
	if bill == nil {
		return fmt.Errorf("bill not found")
	}
	if bill.LockStatus == 0 {
		return errors.New("bill is already unlocked")
	}

	roundNumber, err := moneyClient.GetRoundNumber(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to fetch round number: %w", err)
	}
	tx, err := bill.Unlock(
		sdktypes.WithTimeout(roundNumber+10),
		sdktypes.WithFeeCreditRecordID(fcr.ID),
		sdktypes.WithMaxFee(maxFee),
	)
	if err != nil {
		return fmt.Errorf("failed to create unlock tx: %w", err)
	}
	txSigner, err := sdktypes.NewMoneyTxSignerFromKey(accountKey.PrivKey)
	if err != nil {
		return fmt.Errorf("failed to create money tx signer: %w", err)
	}
	if err = txSigner.SignTx(tx); err != nil {
		return fmt.Errorf("failed to sign tx: %w", err)
	}

	_, err = moneyClient.ConfirmTransaction(cmd.Context(), tx, config.WalletConfig.Base.Logger)
	if err != nil {
		return fmt.Errorf("failed to send unlock tx: %w", err)
	}

	config.WalletConfig.Base.ConsoleWriter.Println("Bill unlocked successfully.")
	return nil
}

func getLockedReasonString(bill *sdktypes.Bill) string {
	if bill.LockStatus != 0 {
		return fmt.Sprintf(" (%s)", wallet.LockReason(bill.LockStatus).String())
	}
	return ""
}
