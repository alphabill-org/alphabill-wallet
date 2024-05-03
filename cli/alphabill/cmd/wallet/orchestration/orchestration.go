package orchestration

import (
	"crypto"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"

	clitypes "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	cliaccount "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/util/account"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/args"
	"github.com/alphabill-org/alphabill-wallet/client/rpc"
	"github.com/alphabill-org/alphabill-wallet/wallet/orchestration/txbuilder"
	"github.com/alphabill-org/alphabill-wallet/wallet/txpublisher"
)

const (
	cmdFlagPartitionID = "partition-id"
	cmdFlagShardID     = "shard-id"
	cmdFlagVarFilePath = "var-file"

	txTimeoutBlockCount = 10
)

func NewCmd(config *clitypes.WalletConfig) *cobra.Command {
	orchestrationConfig := &clitypes.OrchestrationConfig{WalletConfig: config}
	cmd := &cobra.Command{
		Use:   "orchestration",
		Short: "tools to manage orchestration partition",
	}
	cmd.AddCommand(addVarCmd(orchestrationConfig))
	cmd.PersistentFlags().StringVarP(&orchestrationConfig.RpcUrl, args.RpcUrl, "r", args.DefaultOrchestrationRpcUrl, "rpc node url")
	cmd.PersistentFlags().Uint64VarP(&orchestrationConfig.Key, args.KeyCmdName, "k", 1, "account number of the proof-of-authority key")
	cmd.PersistentFlags().Uint32VarP(&orchestrationConfig.SystemID, args.SystemIdentifierCmdName, "s", uint32(orchestration.DefaultSystemID), "system identifier of the orchestration partition")
	return cmd
}

func addVarCmd(orchestrationConfig *clitypes.OrchestrationConfig) *cobra.Command {
	config := &clitypes.AddVarCmdConfig{OrchestrationConfig: orchestrationConfig}
	cmd := &cobra.Command{
		Use:   "add-var",
		Short: "adds validator assignment record",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execAddVarCmd(cmd, config)
		},
	}
	cmd.Flags().Uint32Var(&config.PartitionID, cmdFlagPartitionID, 0, "partition id (system identifier) of the managed partition")
	_ = cmd.MarkFlagRequired(cmdFlagPartitionID)

	cmd.Flags().Var(&config.ShardID, cmdFlagShardID, "shard id (nil if only one shard) of the managed shard")

	cmd.Flags().StringVar(&config.VarFilePath, cmdFlagVarFilePath, "", "path to validator assignment record json file")
	_ = cmd.MarkFlagRequired(cmdFlagVarFilePath)

	return cmd
}

func execAddVarCmd(cmd *cobra.Command, config *clitypes.AddVarCmdConfig) error {
	// load account manager (it is expected that accounts.db exists in wallet home dir)
	am, err := cliaccount.LoadExistingAccountManager(config.OrchestrationConfig.WalletConfig)
	if err != nil {
		return fmt.Errorf("failed to load account manager: %w", err)
	}
	ac, err := am.GetAccountKey(config.OrchestrationConfig.Key - 1)
	if err != nil {
		return fmt.Errorf("failed to load account key: %w", err)
	}

	// create rpc client
	rpcUrl := args.BuildRpcUrl(config.OrchestrationConfig.RpcUrl)
	rpcClient, err := rpc.DialContext(cmd.Context(), rpcUrl)
	if err != nil {
		return fmt.Errorf("failed to create rpc client: %w", err)
	}

	// load var file
	varFile, err := util.ReadJsonFile(config.VarFilePath, &orchestration.ValidatorAssignmentRecord{})
	if err != nil {
		return fmt.Errorf("failed to load var file: %w", err)
	}

	// create 'addVar' tx
	unitPart := hash.Sum(crypto.SHA256, util.Uint32ToBytes(config.PartitionID), config.ShardID)
	unitID := orchestration.NewVarID(config.ShardID, unitPart)
	roundNumber, err := rpcClient.GetRoundNumber(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to fetch round number: %w", err)
	}
	timeout := roundNumber + txTimeoutBlockCount
	txo, err := txbuilder.NewAddVarTx(*varFile, types.SystemID(config.OrchestrationConfig.SystemID), unitID, timeout, ac)
	if err != nil {
		return fmt.Errorf("failed to create 'addVar' tx: %w", err)
	}

	// send 'addVar' tx
	slog := config.OrchestrationConfig.WalletConfig.Base.Logger
	txPublisher := txpublisher.NewTxPublisher(rpcClient, slog)
	_, err = txPublisher.SendTx(cmd.Context(), txo)
	if err != nil {
		return fmt.Errorf("failed to send tx: %w", err)
	}
	config.OrchestrationConfig.WalletConfig.Base.ConsoleWriter.Println("Validator Assignment Record added successfully.")
	return nil
}
