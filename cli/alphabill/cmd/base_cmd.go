package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/money"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/tokens"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet"
)

type (
	WalletApp struct {
		baseCmd  *cobra.Command
		baseConf *types.BaseConfiguration
	}

	Factory interface {
		Logger(cfg *logger.LogConfiguration) (*slog.Logger, error)
		Observability(metrics, traces string) (observability.MeterAndTracer, error)
	}
)

// New creates a new Alphabill wallet application
func New(obsF Factory, opts ...interface{}) *WalletApp {
	baseCmd, baseConfig := newBaseCmd(obsF)
	app := &WalletApp{baseCmd: baseCmd, baseConf: baseConfig}
	app.AddSubcommands(obsF, opts)
	return app
}

// Execute runs the application
func (a *WalletApp) Execute(ctx context.Context) (err error) {
	defer func() {
		if a.baseConf.Observe != nil {
			err = errors.Join(err, a.baseConf.Observe.Shutdown())
		}
	}()

	return a.baseCmd.ExecuteContext(ctx)
}

func (a *WalletApp) AddSubcommands(obsF Factory, opts []interface{}) {
	a.baseCmd.AddCommand(wallet.NewWalletCmd(a.baseConf, obsF))
	a.baseCmd.AddCommand(money.NewMoneyBackendCmd(a.baseConf))
	a.baseCmd.AddCommand(tokens.NewTokensBackendCmd(a.baseConf))
}

func newBaseCmd(obsF Factory) (*cobra.Command, *types.BaseConfiguration) {
	config := &types.BaseConfiguration{}
	// BaseCmd represents the base command when called without any subcommands
	var baseCmd = &cobra.Command{
		Use:           "abwallet",
		Short:         "The alphabill wallet CLI",
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// You can bind cobra and viper in a few locations, but PersistencePreRunE on the base command works well
			// If subcommand does not define PersistentPreRunE, the one from base cmd is used.
			if err := types.InitializeConfig(cmd, config, obsF); err != nil {
				return fmt.Errorf("failed to initialize configuration: %w", err)
			}
			return nil
		},
	}
	config.AddConfigurationFlags(baseCmd)
	return baseCmd, config
}
