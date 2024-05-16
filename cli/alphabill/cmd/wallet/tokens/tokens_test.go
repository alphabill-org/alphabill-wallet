package tokens

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/args"
	"github.com/alphabill-org/alphabill-wallet/wallet/tokens"
)

func TestListTokensCommandInputs(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		accountNumber uint64
		expectedKind  tokens.Kind
		expectedPass  string
		expectedFlags []string
	}{
		{
			name:          "list all tokens",
			args:          []string{},
			accountNumber: 0, // all tokens
			expectedKind:  tokens.Any,
		},
		{
			name:          "list all tokens with flags",
			args:          []string{"--with-all", "--with-type-name", "--with-token-uri", "--with-token-data"},
			expectedKind:  tokens.Any,
			expectedFlags: []string{cmdFlagWithAll, cmdFlagWithTypeName, cmdFlagWithTokenURI, cmdFlagWithTokenData},
		},
		{
			name:          "list all tokens, encrypted wallet",
			args:          []string{"--pn", "some pass phrase"},
			accountNumber: 0, // all tokens
			expectedKind:  tokens.Any,
			expectedPass:  "some pass phrase",
		},
		{
			name:          "list account tokens",
			args:          []string{"--key", "3"},
			accountNumber: 3,
			expectedKind:  tokens.Any,
		},
		{
			name:          "list all fungible tokens",
			args:          []string{"fungible"},
			accountNumber: 0,
			expectedKind:  tokens.Fungible,
		},
		{
			name:          "list account fungible tokens",
			args:          []string{"fungible", "--key", "4"},
			accountNumber: 4,
			expectedKind:  tokens.Fungible,
		},
		{
			name:          "list account fungible tokens, encrypted wallet",
			args:          []string{"fungible", "--key", "4", "--pn", "some pass phrase"},
			accountNumber: 4,
			expectedKind:  tokens.Fungible,
			expectedPass:  "some pass phrase",
		},
		{
			name:          "list all fungible tokens with flags",
			args:          []string{"fungible", "--with-all", "--with-type-name"},
			expectedKind:  tokens.Fungible,
			expectedFlags: []string{cmdFlagWithAll, cmdFlagWithTypeName},
		},
		{
			name:          "list all non-fungible tokens",
			args:          []string{"non-fungible"},
			accountNumber: 0,
			expectedKind:  tokens.NonFungible,
		},
		{
			name:          "list all non-fungible tokens with flags",
			args:          []string{"non-fungible", "--with-all", "--with-type-name", "--with-token-uri", "--with-token-data"},
			expectedKind:  tokens.NonFungible,
			expectedFlags: []string{cmdFlagWithAll, cmdFlagWithTypeName, cmdFlagWithTokenURI, cmdFlagWithTokenData},
		},
		{
			name:          "list account non-fungible tokens",
			args:          []string{"non-fungible", "--key", "5"},
			accountNumber: 5,
			expectedKind:  tokens.NonFungible,
		},
		{
			name:          "list account non-fungible tokens, encrypted wallet",
			args:          []string{"non-fungible", "--key", "5", "--pn", "some pass phrase"},
			accountNumber: 5,
			expectedKind:  tokens.NonFungible,
			expectedPass:  "some pass phrase",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := false
			cmd := tokenCmdList(&types.WalletConfig{}, func(cmd *cobra.Command, config *types.WalletConfig, accountNumber *uint64, kind tokens.Kind) error {
				require.Equal(t, tt.accountNumber, *accountNumber)
				require.Equal(t, tt.expectedKind, kind)
				if len(tt.expectedPass) > 0 {
					passwordFromArg, err := cmd.Flags().GetString(args.PasswordArgCmdName)
					require.NoError(t, err)
					require.Equal(t, tt.expectedPass, passwordFromArg)
				}
				if len(tt.expectedFlags) > 0 {
					for _, flag := range tt.expectedFlags {
						flagValue, err := cmd.Flags().GetBool(flag)
						require.NoError(t, err)
						require.True(t, flagValue)
					}
				}
				exec = true
				return nil
			})
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			require.NoError(t, err)
			require.True(t, exec)
		})
	}
}

func TestListTokensTypesCommandInputs(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedAccNr uint64
		expectedKind  tokens.Kind
		expectedPass  string
	}{
		{
			name:         "list all tokens",
			args:         []string{},
			expectedKind: tokens.Any,
		},
		{
			name:         "list all tokens, encrypted wallet",
			args:         []string{"--pn", "test pass phrase"},
			expectedKind: tokens.Any,
			expectedPass: "test pass phrase",
		},
		{
			name:          "list all fungible tokens",
			args:          []string{"fungible", "-k", "0"},
			expectedKind:  tokens.Fungible,
			expectedAccNr: 0,
		},
		{
			name:          "list all fungible tokens, encrypted wallet",
			args:          []string{"fungible", "--pn", "test pass phrase"},
			expectedKind:  tokens.Fungible,
			expectedPass:  "test pass phrase",
			expectedAccNr: 0,
		},
		{
			name:          "list all non-fungible tokens",
			args:          []string{"non-fungible", "--key", "1"},
			expectedKind:  tokens.NonFungible,
			expectedAccNr: 1,
		},
		{
			name:          "list all non-fungible tokens, encrypted wallet",
			args:          []string{"non-fungible", "--pn", "test pass phrase", "-k", "2"},
			expectedKind:  tokens.NonFungible,
			expectedPass:  "test pass phrase",
			expectedAccNr: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := false
			cmd := tokenCmdListTypes(&types.WalletConfig{}, func(cmd *cobra.Command, config *types.WalletConfig, accountNumber *uint64, kind tokens.Kind) error {
				require.Equal(t, tt.expectedAccNr, *accountNumber)
				require.Equal(t, tt.expectedKind, kind)
				if len(tt.expectedPass) != 0 {
					passwordFromArg, err := cmd.Flags().GetString(args.PasswordArgCmdName)
					require.NoError(t, err)
					require.Equal(t, tt.expectedPass, passwordFromArg)
				}
				exec = true
				return nil
			})
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			require.NoError(t, err)
			require.True(t, exec)
		})
	}
}

func TestWalletCreateFungibleTokenTypeCmd_SymbolFlag(t *testing.T) {
	tokensCmd := testutils.NewSubCmdExecutor(NewTokenCmd, "new-type", "fungible")
	// missing symbol parameter
	tokensCmd.ExecWithError(t, "required flag(s) \"symbol\" not set",
		"--decimals", "3")
	// symbol parameter not set
	tokensCmd.ExecWithError(t, "flag needs an argument: --symbol",
		"--symbol")
}

func TestWalletCreateFungibleTokenTypeCmd_TypeIdlFlag(t *testing.T) {
	tokensCmd := testutils.NewSubCmdExecutor(NewTokenCmd, "new-type", "fungible", "--symbol", "\"@1\"")
	tokensCmd.ExecWithError(t, "flag needs an argument: --type",
		"--type")
	tokensCmd.ExecWithError(t, "invalid argument \"011\" for \"--type\" flag: encoding/hex: odd length hex string",
		"--type", "011")
	tokensCmd.ExecWithError(t, "invalid argument \"foo\" for \"--type\" flag",
		"--type", "foo")
}

func TestWalletCreateFungibleTokenTypeCmd_DecimalsFlag(t *testing.T) {
	// homedir := testutils.CreateNewTestWallet(t)
	tokensCmd := testutils.NewSubCmdExecutor(NewTokenCmd, "new-type", "fungible", "--symbol", "\"@1\"")
	tokensCmd.ExecWithError(t, "flag needs an argument: --decimals",
		"--decimals")
	tokensCmd.ExecWithError(t, "invalid argument \"foo\" for \"--decimals\" flag",
		"--decimals", "foo")
	tokensCmd.ExecWithError(t, "invalid argument \"-1\" for \"--decimals\"",
		"--decimals", "-1")
	tokensCmd.ExecWithError(t, "argument \"9\" for \"--decimals\" flag is out of range, max value 8",
		"--decimals", "9")
}

func TestWalletCreateFungibleTokenCmd_TypeFlag(t *testing.T) {
	tokensCmd := testutils.NewSubCmdExecutor(NewTokenCmd, "new", "fungible")
	tokensCmd.ExecWithError(t, "invalid argument \"A8B\" for \"--type\" flag: encoding/hex: odd length hex string",
		"--type", "A8B")
	tokensCmd.ExecWithError(t, "invalid argument \"nothex\" for \"--type\" flag: encoding/hex: invalid byte",
		"--type", "nothex")
	tokensCmd.ExecWithError(t, "required flag(s) \"type\" not set",
		"--amount", "4")
}

func TestWalletCreateFungibleTokenCmd_AmountFlag(t *testing.T) {
	tokensCmd := testutils.NewSubCmdExecutor(NewTokenCmd, "new", "fungible")
	tokensCmd.ExecWithError(t, "required flag(s) \"amount\" not set",
		"--type", "A8BB")
}

func TestWalletCreateNonFungibleTokenCmd_TypeFlag(t *testing.T) {
	tokensCmd := testutils.NewSubCmdExecutor(NewTokenCmd, "new", "non-fungible")
	tokensCmd.ExecWithError(t, "required flag(s) \"type\" not set",
		"--data", "12AB")
	tokensCmd.ExecWithError(t, "flag needs an argument: --type",
		"--type")
	tokensCmd.ExecWithError(t, "invalid argument \"11dummy\" for \"--type\" flag",
		"--type", "11dummy")
	tokensCmd.ExecWithError(t, "invalid argument \"A8B08\" for \"--type\" flag: encoding/hex: odd length hex string",
		"--type", "A8B08")
}

func TestWalletCreateNonFungibleTokenCmd_DataFileFlag(t *testing.T) {
	data := make([]byte, maxBinaryFile64KiB+1)
	tmpfile, err := os.CreateTemp(t.TempDir(), "test")
	require.NoError(t, err)
	_, err = tmpfile.Write(data)
	require.NoError(t, err)

	tokensCmd := testutils.NewSubCmdExecutor(NewTokenCmd, "new", "non-fungible", "--type", "12AB")
	tokensCmd.ExecWithError(t, "if any flags in the group [data data-file] are set none of the others can be; [data data-file] were all set",
		"--data", "1122aabb",
		"--data-file", "/tmp/test/foo.bin")
	tokensCmd.ExecWithError(t, "data-file read error: stat /tmp/test/foo.bin: no such file or directory",
		"--data-file", "/tmp/test/foo.bin")
	tokensCmd.ExecWithError(t, "data-file read error: file size over 64KiB limit",
		"--data-file", tmpfile.Name())
}

func TestWalletUpdateNonFungibleTokenDataCmd_Flags(t *testing.T) {
	data := make([]byte, maxBinaryFile64KiB+1)
	tmpfile, err := os.CreateTemp(t.TempDir(), "test")
	require.NoError(t, err)
	_, err = tmpfile.Write(data)
	require.NoError(t, err)

	tokensCmd := testutils.NewSubCmdExecutor(NewTokenCmd, "update")

	tokensCmd.ExecWithError(t, "if any flags in the group [data data-file] are set none of the others can be; [data data-file] were all set",
		"--token-identifier", "12AB",
		"--data", "1122aabb",
		"--data-file", "/tmp/test/foo.bin")
	tokensCmd.ExecWithError(t, "data-file read error: stat /tmp/test/foo.bin: no such file or directory",
		"--token-identifier", "12AB",
		"--data-file", "/tmp/test/foo.bin")
	tokensCmd.ExecWithError(t, "data-file read error: file size over 64KiB limit",
		"--token-identifier", "12AB",
		"--data-file", tmpfile.Name())
	tokensCmd.ExecWithError(t, "either of ['--data', '--data-file'] flags must be specified",
		"--token-identifier", "12AB")
	tokensCmd.ExecWithError(t, "required flag(s) \"token-identifier\" not set")
}
