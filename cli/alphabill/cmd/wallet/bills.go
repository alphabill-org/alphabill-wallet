package wallet

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	moneytx "github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
	"github.com/spf13/cobra"

	cmdtypes "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/money"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/backend"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/backend/client"
	txbuilder "github.com/alphabill-org/alphabill-wallet/wallet/money/tx_builder"
)

type (
	// TrustBase json schema for trust base file.
	TrustBase struct {
		RootValidators []*genesis.PublicKeyInfo `json:"root_validators"`
	}
)

// NewWalletBillsCmd creates a new cobra command for the wallet bills component.
func NewWalletBillsCmd(config *WalletConfig) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "bills",
		Short: "cli for managing alphabill wallet bills and proofs",
	}
	cmd.AddCommand(listCmd(config))
	cmd.AddCommand(lockCmd(config))
	cmd.AddCommand(unlockCmd(config))
	return cmd
}

func listCmd(config *WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "lists bill ids and values",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execListCmd(cmd, config)
		},
	}
	cmd.Flags().StringP(alphabillApiURLCmdName, "r", defaultAlphabillApiURL, "alphabill API uri to connect to")
	cmd.Flags().Uint64P(keyCmdName, "k", 0, "specifies which account bills to list (default: all accounts)")
	cmd.Flags().BoolP(showUnswappedCmdName, "s", false, "includes unswapped dust bills in output")
	return cmd
}

func execListCmd(cmd *cobra.Command, config *WalletConfig) error {
	uri, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}
	restClient, err := client.New(uri, config.Base.Observe)
	if err != nil {
		return err
	}
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	showUnswapped, err := cmd.Flags().GetBool(showUnswappedCmdName)
	if err != nil {
		return err
	}

	am, err := LoadExistingAccountManager(config)
	if err != nil {
		return err
	}
	defer am.Close()

	type accountBillGroup struct {
		accountIndex uint64
		pubKey       []byte
		bills        *backend.ListBillsResponse
	}
	var accountBillGroups []*accountBillGroup
	if accountNumber == 0 {
		pubKeys, err := am.GetPublicKeys()
		if err != nil {
			return err
		}
		for accountIndex, pubKey := range pubKeys {
			bills, err := restClient.ListBills(cmd.Context(), pubKey, showUnswapped, "", 100)
			if err != nil {
				return err
			}
			accountBillGroups = append(accountBillGroups, &accountBillGroup{pubKey: pubKey, accountIndex: uint64(accountIndex), bills: bills})
		}
	} else {
		accountIndex := accountNumber - 1
		pubKey, err := am.GetPublicKey(accountIndex)
		if err != nil {
			return err
		}
		accountBills, err := restClient.ListBills(cmd.Context(), pubKey, showUnswapped, "", 100)
		if err != nil {
			return err
		}
		accountBillGroups = append(accountBillGroups, &accountBillGroup{pubKey: pubKey, accountIndex: accountIndex, bills: accountBills})
	}

	for _, group := range accountBillGroups {
		if len(group.bills.Bills) == 0 {
			config.Base.ConsoleWriter.Println(fmt.Sprintf("Account #%d - empty", group.accountIndex+1))
		} else {
			config.Base.ConsoleWriter.Println(fmt.Sprintf("Account #%d", group.accountIndex+1))
		}
		for j, bill := range group.bills.Bills {
			billValueStr := util.AmountToString(bill.Value, 8)
			config.Base.ConsoleWriter.Println(fmt.Sprintf("#%d 0x%X %s%s", j+1, bill.Id, billValueStr, getLockedReasonString(bill)))
		}
	}
	return nil
}

func lockCmd(config *WalletConfig) *cobra.Command {
	var billID cmdtypes.BytesHex
	var systemID uint32
	cmd := &cobra.Command{
		Use:   "lock",
		Short: "locks specific bill",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execLockCmd(cmd, config, billID, types.SystemID(systemID))
		},
	}
	cmd.Flags().StringP(alphabillApiURLCmdName, "r", defaultAlphabillApiURL, "alphabill API uri to connect to")
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "account number of the bill to lock")
	cmd.Flags().Var(&billID, billIdCmdName, "id of the bill to lock")
	cmd.Flags().Uint32Var(&systemID, systemIdentifierCmdName, uint32(moneytx.DefaultSystemIdentifier), "system identifier")
	return cmd
}

func execLockCmd(cmd *cobra.Command, config *WalletConfig, billID []byte, systemID types.SystemID) error {
	uri, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}
	restClient, err := client.New(uri, config.Base.Observe)
	if err != nil {
		return err
	}
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	am, err := LoadExistingAccountManager(config)
	if err != nil {
		return fmt.Errorf("failed to load account manager: %w", err)
	}
	defer am.Close()
	accountKey, err := am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return fmt.Errorf("failed to load account key: %w", err)
	}
	infoResponse, err := restClient.GetInfo(cmd.Context())
	if err != nil {
		return err
	}
	moneyTypeVar := cmdtypes.MoneyType
	if !strings.HasPrefix(infoResponse.Name, moneyTypeVar.String()) {
		return errors.New("invalid wallet backend API URL provided for money partition")
	}
	fcrID := money.FeeCreditRecordIDFormPublicKey(nil, accountKey.PubKey)
	fcb, err := restClient.GetFeeCreditBill(cmd.Context(), fcrID)
	if err != nil {
		return fmt.Errorf("failed to fetch fee credit bill: %w", err)
	}
	if fcb.GetValue() < txbuilder.MaxFee {
		return errors.New("not enough fee credit in wallet")
	}
	bill, err := fetchBillByID(cmd.Context(), billID, restClient, accountKey)
	if err != nil {
		return fmt.Errorf("failed to fetch bill by id: %w", err)
	}
	if bill.IsLocked() {
		return errors.New("bill is already locked")
	}
	rnr, err := restClient.GetRoundNumber(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to fetch round number: %w", err)
	}
	tx, err := txbuilder.NewLockTx(accountKey, systemID, bill.Id, bill.TxHash, wallet.LockReasonManual, rnr.RoundNumber+10)
	if err != nil {
		return fmt.Errorf("failed to create lock tx: %w", err)
	}
	moneyTxPublisher := money.NewTxPublisher(restClient, config.Base.Observe.Logger())
	_, err = moneyTxPublisher.SendTx(cmd.Context(), tx, accountKey.PubKey)
	if err != nil {
		return fmt.Errorf("failed to send lock tx: %w", err)
	}
	config.Base.ConsoleWriter.Println("Bill locked successfully.")
	return nil
}

func unlockCmd(config *WalletConfig) *cobra.Command {
	var billID cmdtypes.BytesHex
	var systemID uint32
	cmd := &cobra.Command{
		Use:   "unlock",
		Short: "unlocks specific bill",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execUnlockCmd(cmd, config, billID, types.SystemID(systemID))
		},
	}
	cmd.Flags().StringP(alphabillApiURLCmdName, "r", defaultAlphabillApiURL, "alphabill API uri to connect to")
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "account number of the bill to unlock")
	cmd.Flags().Var(&billID, billIdCmdName, "id of the bill to unlock")
	cmd.Flags().Uint32Var(&systemID, systemIdentifierCmdName, uint32(moneytx.DefaultSystemIdentifier), "system identifier")
	return cmd
}

func execUnlockCmd(cmd *cobra.Command, config *WalletConfig, billID []byte, systemID types.SystemID) error {
	uri, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}
	restClient, err := client.New(uri, config.Base.Observe)
	if err != nil {
		return err
	}
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	am, err := LoadExistingAccountManager(config)
	if err != nil {
		return fmt.Errorf("failed to load account manager: %w", err)
	}
	defer am.Close()

	infoResponse, err := restClient.GetInfo(cmd.Context())
	if err != nil {
		return err
	}
	moneyTypeVar := cmdtypes.MoneyType
	if !strings.HasPrefix(infoResponse.Name, moneyTypeVar.String()) {
		return errors.New("invalid wallet backend API URL provided for money partition")
	}
	accountKey, err := am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return fmt.Errorf("failed to load account key: %w", err)
	}
	fcrID := money.FeeCreditRecordIDFormPublicKey(nil, accountKey.PubKey)
	fcb, err := restClient.GetFeeCreditBill(cmd.Context(), fcrID)
	if err != nil {
		return fmt.Errorf("failed to fetch fee credit bill: %w", err)
	}
	if fcb.GetValue() < txbuilder.MaxFee {
		return errors.New("not enough fee credit in wallet")
	}
	bill, err := fetchBillByID(cmd.Context(), billID, restClient, accountKey)
	if err != nil {
		return fmt.Errorf("failed to fetch bill by id: %w", err)
	}
	if !bill.IsLocked() {
		return errors.New("bill is already unlocked")
	}
	rnr, err := restClient.GetRoundNumber(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to fetch round number: %w", err)
	}
	tx, err := txbuilder.NewUnlockTx(accountKey, systemID, bill, rnr.RoundNumber+10)
	if err != nil {
		return fmt.Errorf("failed to create unlock tx: %w", err)
	}
	moneyTxPublisher := money.NewTxPublisher(restClient, config.Base.Observe.Logger())
	_, err = moneyTxPublisher.SendTx(cmd.Context(), tx, accountKey.PubKey)
	if err != nil {
		return fmt.Errorf("failed to send unlock tx: %w", err)
	}
	config.Base.ConsoleWriter.Println("Bill unlocked successfully.")
	return nil
}

func fetchBillByID(ctx context.Context, billID []byte, restClient *client.MoneyBackendClient, accountKey *account.AccountKey) (*wallet.Bill, error) {
	bills, err := restClient.GetBills(ctx, accountKey.PubKey)
	if err != nil {
		return nil, err
	}
	for _, b := range bills {
		if bytes.Equal(b.Id, billID) {
			return b, nil
		}
	}
	return nil, errors.New("bill not found")
}

func getLockedReasonString(bill *wallet.Bill) string {
	if bill.IsLocked() {
		return fmt.Sprintf(" (%s)", bill.Locked.String())
	}
	return ""
}
