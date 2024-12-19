package permissioned

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc/permissioned"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/spf13/cobra"

	clitypes "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	cliaccount "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/util/account"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/args"
	"github.com/alphabill-org/alphabill-wallet/client"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/util"
)

const txTimeoutBlockCount = 10

// NewCmd creates a new cobra command for managing permissioned partitions.
func NewCmd(walletConfig *clitypes.WalletConfig) *cobra.Command {
	var config = &config{
		walletConfig: walletConfig,
	}
	var cmd = &cobra.Command{
		Use:   "permissioned",
		Short: "cli for managing permissioned partitions",
		Run: func(cmd *cobra.Command, args []string) {
			walletConfig.Base.ConsoleWriter.Println("Error: must specify a subcommand")
		},
	}
	cmd.AddCommand(addFeeCreditCmd(config))
	cmd.AddCommand(deleteFeeCreditCmd(config))

	cmd.PersistentFlags().StringVarP(&config.rpcUrl, args.RpcUrl, "r", "", "RPC URL of a partition node")
	return cmd
}

func addFeeCreditCmd(config *config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-credit",
		Short: "adds fee credit to a fee credit record owned by the specified owner predicate (admin only command)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return addFeeCreditCmdExec(cmd, config)
		},
	}

	var hexFlag clitypes.BytesHex
	cmd.Flags().VarP(&hexFlag, args.TargetPubkeyFlagName, "t", "pubkey of the fee credit record owner")
	err := cmd.MarkFlagRequired(args.TargetPubkeyFlagName)
	if err != nil {
		return nil
	}
	cmd.Flags().Uint64P(args.KeyCmdName, "k", 1, "key used to sign the transaction")
	cmd.Flags().StringP(args.AmountCmdName, "v", "1", "specifies how much fee credit to add in ALPHA")
	return cmd
}

func addFeeCreditCmdExec(cmd *cobra.Command, config *config) error {
	amountString, err := cmd.Flags().GetString(args.AmountCmdName)
	if err != nil {
		return err
	}

	amount, err := util.StringToAmount(amountString, 8)
	if err != nil {
		return err
	}

	targetPubkey := *cmd.Flag(args.TargetPubkeyFlagName).Value.(*clitypes.BytesHex)

	rpcUrl, err := cmd.Flags().GetString(args.RpcUrl)
	if err != nil {
		return err
	}
	tokensClient, err := client.NewTokensPartitionClient(cmd.Context(), args.BuildRpcUrl(rpcUrl))
	if err != nil {
		return fmt.Errorf("failed to dial rpc url: %w", err)
	}
	defer tokensClient.Close()

	nodeInfo, err := tokensClient.GetNodeInfo(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get node info: %w", err)
	}
	if !nodeInfo.PermissionedMode {
		return fmt.Errorf("cannot add fee credit, partition not in permissioned mode")
	}

	pdr, err := tokensClient.PartitionDescription(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get PDR: %w", err)
	}

	am, err := cliaccount.LoadExistingAccountManager(config.walletConfig)
	if err != nil {
		return fmt.Errorf("failed to load account manager: %w", err)
	}
	defer am.Close()

	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	if accountNumber == 0 {
		return fmt.Errorf("invalid parameter for flag %q: 0 is not a valid account key", args.KeyCmdName)
	}
	accountKey, err := am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return fmt.Errorf("failed to get account key for account %d", accountNumber)
	}

	ownerID := hash.Sum256(targetPubkey)
	fcr, err := tokensClient.GetFeeCreditRecordByOwnerID(cmd.Context(), ownerID)
	if err != nil {
		return fmt.Errorf("failed to fetch fee credit record: %w", err)
	}

	round, err := tokensClient.GetRoundNumber(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get current round number: %w", err)
	}
	timeout := round + txTimeoutBlockCount

	ownerPredicate := templates.NewP2pkh256BytesFromKeyHash(ownerID)
	if fcr == nil {
		fcrID, err := tokens.NewFeeCreditRecordIDFromOwnerPredicate(pdr, types.ShardID{}, ownerPredicate, timeout)
		if err != nil {
			return fmt.Errorf("failed to create fee credit record ID: %w", err)
		}
		fcr = &sdktypes.FeeCreditRecord{
			NetworkID:   nodeInfo.NetworkID,
			PartitionID: nodeInfo.PartitionID,
			ID:          fcrID,
		}
	}

	setFCTx, err := fcr.SetFeeCredit(ownerPredicate, amount, sdktypes.WithTimeout(timeout))
	if err != nil {
		return fmt.Errorf("failed to create setFC transaction: %w", err)
	}

	adminProof, err := sdktypes.NewP2pkhAuthProofSignatureFromKey(setFCTx, accountKey.PrivKey)
	if err != nil {
		return fmt.Errorf("failed to create owner predicate signature: %w", err)
	}
	err = setFCTx.SetAuthProof(permissioned.SetFeeCreditAuthProof{OwnerProof: adminProof})
	if err != nil {
		return fmt.Errorf("failed to set transaction auth proof: %w", err)
	}

	_, err = tokensClient.ConfirmTransaction(cmd.Context(), setFCTx, config.walletConfig.Base.Logger)
	if err != nil {
		return fmt.Errorf("failed to send transaction: %w", err)
	}
	config.walletConfig.Base.ConsoleWriter.Println("Fee credit added successfully")
	config.walletConfig.Base.ConsoleWriter.Println(fmt.Sprintf("FCR ID 0x%s", setFCTx.Payload.UnitID))
	return nil
}

func deleteFeeCreditCmd(config *config) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete-credit",
		Short: "deletes fee credit record owned by the specified owner predicate (admin only command)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return deleteFeeCreditCmdExec(cmd, config)
		},
	}

	var hexFlag clitypes.BytesHex
	cmd.Flags().VarP(&hexFlag, args.TargetPubkeyFlagName, "t", "pubkey of the fee credit record owner")
	err := cmd.MarkFlagRequired(args.TargetPubkeyFlagName)
	if err != nil {
		return nil
	}
	cmd.Flags().Uint64P(args.KeyCmdName, "k", 1, "key used to sign the transaction")

	return cmd
}

func deleteFeeCreditCmdExec(cmd *cobra.Command, config *config) error {
	targetPubkey := *cmd.Flag(args.TargetPubkeyFlagName).Value.(*clitypes.BytesHex)

	rpcUrl, err := cmd.Flags().GetString(args.RpcUrl)
	if err != nil {
		return err
	}
	tokensClient, err := client.NewTokensPartitionClient(cmd.Context(), args.BuildRpcUrl(rpcUrl))
	if err != nil {
		return fmt.Errorf("failed to dial rpc url: %w", err)
	}
	defer tokensClient.Close()

	nodeInfo, err := tokensClient.GetNodeInfo(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get node info: %w", err)
	}
	if !nodeInfo.PermissionedMode {
		return fmt.Errorf("cannot delete fee credit, partition not in permissioned mode")
	}

	am, err := cliaccount.LoadExistingAccountManager(config.walletConfig)
	if err != nil {
		return fmt.Errorf("failed to load account manager: %w", err)
	}
	defer am.Close()

	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	if accountNumber == 0 {
		return fmt.Errorf("invalid parameter for flag %q: 0 is not a valid account key", args.KeyCmdName)
	}
	accountKey, err := am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return fmt.Errorf("failed to get account key for account %d", accountNumber)
	}

	ownerID := hash.Sum256(targetPubkey)
	fcr, err := tokensClient.GetFeeCreditRecordByOwnerID(cmd.Context(), ownerID)
	if err != nil {
		return fmt.Errorf("failed to fetch fee credit record: %w", err)
	}
	if fcr == nil {
		return fmt.Errorf("fee credit record not found")
	}

	round, err := tokensClient.GetRoundNumber(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to get current round number: %w", err)
	}

	setFCTx, err := fcr.DeleteFeeCredit(sdktypes.WithTimeout(round + txTimeoutBlockCount))
	if err != nil {
		return fmt.Errorf("failed to create deleteFC transaction: %w", err)
	}

	adminProof, err := sdktypes.NewP2pkhAuthProofSignatureFromKey(setFCTx, accountKey.PrivKey)
	if err != nil {
		return fmt.Errorf("failed to create owner predicate signature: %w", err)
	}
	err = setFCTx.SetAuthProof(permissioned.SetFeeCreditAuthProof{OwnerProof: adminProof})
	if err != nil {
		return fmt.Errorf("failed to set transaction auth proof: %w", err)
	}

	_, err = tokensClient.ConfirmTransaction(cmd.Context(), setFCTx, config.walletConfig.Base.Logger)
	if err != nil {
		return fmt.Errorf("failed to send transaction: %w", err)
	}
	config.walletConfig.Base.ConsoleWriter.Println("Fee credit deleted successfully")
	return nil
}

type config struct {
	walletConfig *clitypes.WalletConfig
	rpcUrl       string
}
