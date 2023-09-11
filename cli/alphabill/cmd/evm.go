package cmd

import (
	"context"
	"fmt"
	"math/big"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/txsystem/evm"
	"github.com/alphabill-org/alphabill/internal/txsystem/evm/api"
	"github.com/fxamacker/cbor/v2"
	"github.com/spf13/cobra"
)

type (
	evmConfiguration struct {
		baseNodeConfiguration
		Node       *startNodeConfiguration
		RPCServer  *grpcServerConfiguration
		RESTServer *restServerConfiguration
	}
)

func newEvmNodeCmd(baseConfig *baseConfiguration) *cobra.Command {
	config := &evmConfiguration{
		baseNodeConfiguration: baseNodeConfiguration{
			Base: baseConfig,
		},
		Node:       &startNodeConfiguration{},
		RPCServer:  &grpcServerConfiguration{},
		RESTServer: &restServerConfiguration{},
	}

	var nodeCmd = &cobra.Command{
		Use:   "evm",
		Short: "Starts an evm partition node",
		Long:  `Starts an evm partition node, binding to the network address provided by configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEvmNode(cmd.Context(), config)
		},
	}

	addCommonNodeConfigurationFlags(nodeCmd, config.Node, "evm")

	config.RPCServer.addConfigurationFlags(nodeCmd)
	config.RESTServer.addConfigurationFlags(nodeCmd)
	return nodeCmd
}

func runEvmNode(ctx context.Context, cfg *evmConfiguration) error {
	pg, err := loadPartitionGenesis(cfg.Node.Genesis)
	if err != nil {
		return err
	}
	params := &genesis.EvmPartitionParams{}
	err = cbor.Unmarshal(pg.Params, params)
	if err != nil {
		return fmt.Errorf("failed to unmarshal evm partition params: %w", err)
	}
	systemIdentifier := pg.SystemDescriptionRecord.GetSystemIdentifier()
	txs, err := evm.NewEVMTxSystem(
		systemIdentifier,
		evm.WithBlockGasLimit(params.BlockGasLimit),
		evm.WithGasPrice(params.GasUnitPrice),
	)
	if err != nil {
		return err
	}
	cfg.RESTServer.router = api.NewAPI(
		txs.GetState(),
		systemIdentifier,
		big.NewInt(0).SetUint64(params.BlockGasLimit),
		params.GasUnitPrice,
	)
	return defaultNodeRunFunc(ctx, "evm node", txs, cfg.Node, cfg.RPCServer, cfg.RESTServer)
}
