package cmd

import (
	"context"
	"fmt"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/spf13/cobra"
)

func newNodeIdentifierCmd(ctx context.Context) *cobra.Command {
	var file string
	var cmd = &cobra.Command{
		Use:   "identifier",
		Short: "Returns the ID of the node",
		RunE: func(cmd *cobra.Command, args []string) error {
			return identifierRunFun(ctx, file)
		},
	}
	cmd.Flags().StringVarP(&file, keyFileCmd, "k", "", "path to the key file")
	err := cmd.MarkFlagRequired(keyFileCmd)
	if err != nil {
		panic(err)
	}
	return cmd
}

func identifierRunFun(_ context.Context, file string) error {
	keys, err := LoadKeys(file, false)
	if err != nil {
		return errors.Wrapf(err, "failed to load keys %v", file)
	}
	id, err := peer.IDFromPublicKey(keys.EncryptionPrivateKey.GetPublic())
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", id)
	return nil

}
