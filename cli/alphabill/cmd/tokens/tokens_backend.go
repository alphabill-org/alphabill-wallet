package tokens

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/alphabill-org/alphabill/types"
	"github.com/spf13/cobra"

	cmdtypes "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/tokens/backend"
)

const (
	serverAddrCmdName       = "server-addr"
	dbFileCmdName           = "db"
	systemIdentifierCmdName = "system-identifier"
	defaultServerAddr       = "localhost:9735"
	defaultAlphabillNodeURL = "localhost:9543"
	alphabillNodeURLCmdName = "alphabill-uri"
)

func NewTokensBackendCmd(baseConfig *cmdtypes.BaseConfiguration) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "tokens-backend",
		Short: "Starts tokens backend service",
		Long:  "Starts tokens backend service, indexes all transactions by owner predicates, starts http server",
	}
	cmd.AddCommand(buildCmdStartTokensBackend(baseConfig))
	return cmd
}

func buildCmdStartTokensBackend(config *cmdtypes.BaseConfiguration) *cobra.Command {
	var systemID uint32
	cmd := &cobra.Command{
		Use: "start",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokensBackendStartCmd(cmd.Context(), cmd, config, types.SystemID(systemID))
		},
	}
	cmd.Flags().StringP(alphabillNodeURLCmdName, "u", defaultAlphabillNodeURL, "alphabill node url")
	cmd.Flags().StringP(serverAddrCmdName, "s", defaultServerAddr, "server address")
	cmd.Flags().StringP(dbFileCmdName, "f", "", "path to the database file")
	cmd.Flags().Uint32Var(&systemID, systemIdentifierCmdName, uint32(tokens.DefaultSystemIdentifier), "system identifier in hex format")
	return cmd
}

func execTokensBackendStartCmd(ctx context.Context, cmd *cobra.Command, config *cmdtypes.BaseConfiguration, systemID types.SystemID) error {
	abURL, err := cmd.Flags().GetString(alphabillNodeURLCmdName)
	if err != nil {
		return fmt.Errorf("failed to get %q flag value: %w", alphabillNodeURLCmdName, err)
	}
	srvAddr, err := cmd.Flags().GetString(serverAddrCmdName)
	if err != nil {
		return fmt.Errorf("failed to get %q flag value: %w", serverAddrCmdName, err)
	}
	dbFile, err := filenameEnsureDir(dbFileCmdName, cmd, config.HomeDir, "tokens-backend", "tokens.db")
	if err != nil {
		return fmt.Errorf("failed to get path for database: %w", err)
	}
	return backend.Run(ctx, backend.NewConfig(systemID, srvAddr, abURL, dbFile, config.Observe))
}

/*
filenameEnsureDir assumes that "flagName" is a filename parameter in "cmd" and returns it's value
or name constructed from defaultPathElements (when the flag value is empty). It also ensures that
the directory of the file exists (ie creates it when it doesn't exist already).
*/
func filenameEnsureDir(flagName string, cmd *cobra.Command, defaultPathElements ...string) (string, error) {
	fileName, err := cmd.Flags().GetString(flagName)
	if err != nil {
		return "", fmt.Errorf("failed to get %q flag value: %w", flagName, err)
	}
	if fileName == "" {
		fileName = filepath.Join(defaultPathElements...)
	}
	if err := os.MkdirAll(filepath.Dir(fileName), 0700); err != nil {
		return "", fmt.Errorf("failed to create directory path: %w", err)
	}
	return fileName, nil
}