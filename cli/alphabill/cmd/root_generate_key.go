package cmd

import (
	"context"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"
	"github.com/spf13/cobra"
	"os"
	"path"
)

const (
	rootKeyFileName = "root-key.json"
	rootKeysDirName = "root-keys"
)

type generateKeyConfig struct {
	rootGenesisConfig *rootGenesisConfig

	// path to output directory where key file will be created (default $ABHOME/root-keys)
	OutputDir string
}

// getOutputDir returns custom outputdir if provided, otherwise $ABHOME/root-keys, and creates parent directories.
// Must be called after base command PersistentPreRunE function has been called, so that $ABHOME is initialized.
func (c *generateKeyConfig) getOutputDir() string {
	if c.OutputDir != "" {
		return c.OutputDir
	}
	defaultOutputDir := path.Join(c.rootGenesisConfig.Base.HomeDir, rootKeysDirName)
	err := os.MkdirAll(defaultOutputDir, 0700) // -rwx------
	if err != nil {
		panic(err)
	}
	return defaultOutputDir
}

// newGenerateKeyCmd creates a new cobra command for the root-genesis generate-key component.
func newGenerateKeyCmd(ctx context.Context, rootGenesisConfig *rootGenesisConfig) *cobra.Command {
	config := &generateKeyConfig{rootGenesisConfig: rootGenesisConfig}
	var cmd = &cobra.Command{
		Use:   "generate-key",
		Short: "Generates root key file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return generateKeysRunFunc(ctx, config)
		},
	}
	cmd.Flags().StringVarP(&config.OutputDir, "output-dir", "o", "", "path to output directory (default: $ABHOME/root-keys)")
	return cmd
}

func generateKeysRunFunc(_ context.Context, config *generateKeyConfig) error {
	rk, err := newRootKey()
	if err != nil {
		return err
	}
	return createRootKeyFile(rk, config.getOutputDir())
}

func createRootKeyFile(rk *rootKey, outputDir string) error {
	outputFile := path.Join(outputDir, rootKeyFileName)
	return util.WriteJsonFile(outputFile, rk)
}
