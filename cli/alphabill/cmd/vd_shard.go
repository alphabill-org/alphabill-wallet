package cmd

import (
	"context"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/verifiable_data"
	"github.com/spf13/cobra"
	"os"
	"path"
)

type (
	vdShardConfiguration struct {
		baseShardConfiguration
		// trust base public keys, in compressed secp256k1 (33 bytes each) hex format
		UnicityTrustBase []string
	}

	vdShardTxConverter struct{}
)

func newVDShardCmd(ctx context.Context, rootConfig *rootConfiguration) *cobra.Command {
	config := &vdShardConfiguration{
		baseShardConfiguration: baseShardConfiguration{
			Root:   rootConfig,
			Server: &grpcServerConfiguration{},
		},
	}
	// shardCmd represents the shard command
	var shardCmd = &cobra.Command{
		Use:   "vd-shard",
		Short: "Starts a Verifiable Data partition's shard node",
		Long:  `Starts a Verifiable Data partition's shard node, binding to the network address provided by configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return defaultVDShardRunFunc(ctx, config)
		},
	}

	shardCmd.Flags().StringSliceVar(&config.UnicityTrustBase, "trust-base", []string{}, "public key used as trust base, in compressed (33 bytes) hex format.")
	config.Server.addConfigurationFlags(shardCmd)

	return shardCmd
}

func (r *vdShardTxConverter) Convert(tx *transaction.Transaction) (transaction.GenericTransaction, error) {
	return transaction.NewVerifiableDataTx(tx)
}

func defaultVDShardRunFunc(ctx context.Context, cfg *vdShardConfiguration) error {
	state, err := verifiable_data.NewVDSchemeState(cfg.UnicityTrustBase)
	if err != nil {
		return err
	}

	err = os.MkdirAll(cfg.Root.HomeDir+"/vd", 0700) // -rwx------
	if err != nil {
		return err
	}
	blockStoreFile := path.Join(cfg.Root.HomeDir, "vd", partition.BoltBlockStoreFileName)
	blockStore, err := partition.NewBoltBlockStore(blockStoreFile)
	if err != nil {
		return err
	}

	return defaultShardRunFunc(ctx, &cfg.baseShardConfiguration, &vdShardTxConverter{}, state, blockStore)
}
