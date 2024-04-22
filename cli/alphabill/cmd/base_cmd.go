package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/tools"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet"
)

type (
	WalletApp struct {
		baseCmd  *cobra.Command
		baseConf *types.BaseConfiguration
	}
)

// New creates a new Alphabill wallet application
func New(opts ...interface{}) *WalletApp {
	baseCmd, baseConfig := newBaseCmd()
	app := &WalletApp{baseCmd: baseCmd, baseConf: baseConfig}
	app.AddSubcommands(opts)
	return app
}

// Execute runs the application
func (a *WalletApp) Execute(ctx context.Context) (err error) {
	return a.baseCmd.ExecuteContext(ctx)
}

func (a *WalletApp) AddSubcommands(opts []interface{}) {
	a.baseCmd.AddCommand(wallet.NewWalletCmd(a.baseConf))
	a.baseCmd.AddCommand(tools.NewToolsCmd())
}

func newBaseCmd() (*cobra.Command, *types.BaseConfiguration) {
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
			if err := types.InitializeConfig(cmd, config); err != nil {
				return fmt.Errorf("failed to initialize configuration: %w", err)
			}
			return nil
		},
	}
	config.AddConfigurationFlags(baseCmd)
	return baseCmd, config
}
