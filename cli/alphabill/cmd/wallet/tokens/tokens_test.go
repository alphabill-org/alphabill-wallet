package tokens

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	clitypes "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/args"
	"github.com/alphabill-org/alphabill-wallet/client/rpc"
	test "github.com/alphabill-org/alphabill-wallet/internal/testutils"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/logger"
	tokenswallet "github.com/alphabill-org/alphabill-wallet/wallet/tokens"
)

func TestListTokensCommandInputs(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		accountNumber uint64
		expectedKind  tokenswallet.Kind
		expectedPass  string
		expectedFlags []string
	}{
		{
			name:          "list all tokens",
			args:          []string{},
			accountNumber: 0, // all tokens
			expectedKind:  tokenswallet.Any,
		},
		{
			name:          "list all tokens with flags",
			args:          []string{"--with-all", "--with-type-name", "--with-token-uri", "--with-token-data"},
			expectedKind:  tokenswallet.Any,
			expectedFlags: []string{cmdFlagWithAll, cmdFlagWithTypeName, cmdFlagWithTokenURI, cmdFlagWithTokenData},
		},
		{
			name:          "list all tokens, encrypted wallet",
			args:          []string{"--pn", "some pass phrase"},
			accountNumber: 0, // all tokens
			expectedKind:  tokenswallet.Any,
			expectedPass:  "some pass phrase",
		},
		{
			name:          "list account tokens",
			args:          []string{"--key", "3"},
			accountNumber: 3,
			expectedKind:  tokenswallet.Any,
		},
		{
			name:          "list all fungible tokens",
			args:          []string{"fungible"},
			accountNumber: 0,
			expectedKind:  tokenswallet.Fungible,
		},
		{
			name:          "list account fungible tokens",
			args:          []string{"fungible", "--key", "4"},
			accountNumber: 4,
			expectedKind:  tokenswallet.Fungible,
		},
		{
			name:          "list account fungible tokens, encrypted wallet",
			args:          []string{"fungible", "--key", "4", "--pn", "some pass phrase"},
			accountNumber: 4,
			expectedKind:  tokenswallet.Fungible,
			expectedPass:  "some pass phrase",
		},
		{
			name:          "list all fungible tokens with falgs",
			args:          []string{"fungible", "--with-all", "--with-type-name"},
			expectedKind:  tokenswallet.Fungible,
			expectedFlags: []string{cmdFlagWithAll, cmdFlagWithTypeName},
		},
		{
			name:          "list all non-fungible tokens",
			args:          []string{"non-fungible"},
			accountNumber: 0,
			expectedKind:  tokenswallet.NonFungible,
		},
		{
			name:          "list all non-fungible tokens with flags",
			args:          []string{"non-fungible", "--with-all", "--with-type-name", "--with-token-uri", "--with-token-data"},
			expectedKind:  tokenswallet.NonFungible,
			expectedFlags: []string{cmdFlagWithAll, cmdFlagWithTypeName, cmdFlagWithTokenURI, cmdFlagWithTokenData},
		},
		{
			name:          "list account non-fungible tokens",
			args:          []string{"non-fungible", "--key", "5"},
			accountNumber: 5,
			expectedKind:  tokenswallet.NonFungible,
		},
		{
			name:          "list account non-fungible tokens, encrypted wallet",
			args:          []string{"non-fungible", "--key", "5", "--pn", "some pass phrase"},
			accountNumber: 5,
			expectedKind:  tokenswallet.NonFungible,
			expectedPass:  "some pass phrase",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := false
			cmd := tokenCmdList(&clitypes.WalletConfig{}, func(cmd *cobra.Command, config *clitypes.WalletConfig, accountNumber *uint64, kind tokenswallet.Kind) error {
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
		expectedKind  tokenswallet.Kind
		expectedPass  string
	}{
		{
			name:         "list all tokens",
			args:         []string{},
			expectedKind: tokenswallet.Any,
		},
		{
			name:         "list all tokens, encrypted wallet",
			args:         []string{"--pn", "test pass phrase"},
			expectedKind: tokenswallet.Any,
			expectedPass: "test pass phrase",
		},
		{
			name:          "list all fungible tokens",
			args:          []string{"fungible", "-k", "0"},
			expectedKind:  tokenswallet.Fungible,
			expectedAccNr: 0,
		},
		{
			name:          "list all fungible tokens, encrypted wallet",
			args:          []string{"fungible", "--pn", "test pass phrase"},
			expectedKind:  tokenswallet.Fungible,
			expectedPass:  "test pass phrase",
			expectedAccNr: 0,
		},
		{
			name:          "list all non-fungible tokens",
			args:          []string{"non-fungible", "--key", "1"},
			expectedKind:  tokenswallet.NonFungible,
			expectedAccNr: 1,
		},
		{
			name:          "list all non-fungible tokens, encrypted wallet",
			args:          []string{"non-fungible", "--pn", "test pass phrase", "-k", "2"},
			expectedKind:  tokenswallet.NonFungible,
			expectedPass:  "test pass phrase",
			expectedAccNr: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := false
			cmd := tokenCmdListTypes(&clitypes.WalletConfig{}, func(cmd *cobra.Command, config *clitypes.WalletConfig, accountNumber *uint64, kind tokenswallet.Kind) error {
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
	homedir := testutils.CreateNewTestWallet(t)
	// missing symbol parameter
	tokenCmdNewTypeFungible(&clitypes.WalletConfig{
		Base:          &clitypes.BaseConfiguration{HomeDir: homedir},
		WalletHomeDir: filepath.Join(homedir, "wallet"),
	})
	execTokensCmdWithError(t, homedir, "new-type fungible --decimals 3", "required flag(s) \"symbol\" not set")
	// symbol parameter not set
	execTokensCmdWithError(t, homedir, "new-type fungible --symbol", "flag needs an argument: --symbol")
}

func TestWalletCreateFungibleTokenTypeCmd_TypeIdlFlag(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	execTokensCmdWithError(t, homedir, "new-type fungible --symbol \"@1\" --type", "flag needs an argument: --type")
	execTokensCmdWithError(t, homedir, "new-type fungible --symbol \"@1\" --type 011", "invalid argument \"011\" for \"--type\" flag: encoding/hex: odd length hex string")
	execTokensCmdWithError(t, homedir, "new-type fungible --symbol \"@1\" --type foo", "invalid argument \"foo\" for \"--type\" flag")
}

func TestWalletCreateFungibleTokenTypeCmd_DecimalsFlag(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	execTokensCmdWithError(t, homedir, "new-type fungible --symbol \"@1\" --decimals", "flag needs an argument: --decimals")
	execTokensCmdWithError(t, homedir, "new-type fungible --symbol \"@1\" --decimals foo", "invalid argument \"foo\" for \"--decimals\" flag")
	execTokensCmdWithError(t, homedir, "new-type fungible --symbol \"@1\" --decimals -1", "invalid argument \"-1\" for \"--decimals\"")
	execTokensCmdWithError(t, homedir, "new-type fungible --symbol \"@1\" --decimals 9", "argument \"9\" for \"--decimals\" flag is out of range, max value 8")
}

func TestWalletCreateFungibleTokenCmd_TypeFlag(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	execTokensCmdWithError(t, homedir, "new fungible --type A8B", "invalid argument \"A8B\" for \"--type\" flag: encoding/hex: odd length hex string")
	execTokensCmdWithError(t, homedir, "new fungible --type nothex", "invalid argument \"nothex\" for \"--type\" flag: encoding/hex: invalid byte")
	execTokensCmdWithError(t, homedir, "new fungible --amount 4", "required flag(s) \"type\" not set")
}

func TestWalletCreateFungibleTokenCmd_AmountFlag(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	execTokensCmdWithError(t, homedir, "new fungible --type A8BB", "required flag(s) \"amount\" not set")
}

func TestWalletCreateNonFungibleTokenCmd_TypeFlag(t *testing.T) {
	type args struct {
		cmdParams string
	}
	tests := []struct {
		name       string
		args       args
		want       []byte
		wantErrStr string
	}{
		{
			name:       "missing token type parameter",
			args:       args{cmdParams: "new non-fungible --data 12AB"},
			wantErrStr: "required flag(s) \"type\" not set",
		},
		{
			name:       "missing token type parameter has no value",
			args:       args{cmdParams: "new non-fungible --type"},
			wantErrStr: "flag needs an argument: --type",
		},
		{
			name:       "type parameter is not hex encoded",
			args:       args{cmdParams: "new non-fungible --type 11dummy"},
			wantErrStr: "invalid argument \"11dummy\" for \"--type\" flag",
		},
		{
			name:       "type parameter is odd length",
			args:       args{cmdParams: "new non-fungible --type A8B08"},
			wantErrStr: "invalid argument \"A8B08\" for \"--type\" flag: encoding/hex: odd length hex string",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homedir := testutils.CreateNewTestWallet(t)
			_, err := doExecTokensCmd(t, homedir, tt.args.cmdParams)
			if len(tt.wantErrStr) != 0 {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWalletCreateNonFungibleTokenCmd_TokenIdFlag(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	execTokensCmdWithError(t, homedir, "new non-fungible --type A8B0 --token-identifier A8B09", "invalid argument \"A8B09\" for \"--token-identifier\" flag: encoding/hex: odd length hex string")
	execTokensCmdWithError(t, homedir, "new non-fungible --type A8B0 --token-identifier nothex", "invalid argument \"nothex\" for \"--token-identifier\" flag: encoding/hex: invalid byte")
}

func TestWalletCreateNonFungibleTokenCmd_DataFileFlag(t *testing.T) {
	data := make([]byte, maxBinaryFile64KiB+1)
	tmpfile, err := os.CreateTemp(t.TempDir(), "test")
	require.NoError(t, err)
	_, err = tmpfile.Write(data)
	require.NoError(t, err)

	tests := []struct {
		name       string
		cmdParams  string
		want       []byte
		wantErrStr string
	}{
		{
			name:       "both data and data-file specified",
			cmdParams:  "new non-fungible --type 12AB --data 1122aabb --data-file=/tmp/test/foo.bin",
			wantErrStr: "if any flags in the group [data data-file] are set none of the others can be; [data data-file] were all set",
		},
		{
			name:       "data-file not found",
			cmdParams:  "new non-fungible --type 12AB --data-file=/tmp/test/foo.bin",
			wantErrStr: "data-file read error: stat /tmp/test/foo.bin: no such file or directory",
		},
		{
			name:       "data-file too big",
			cmdParams:  "new non-fungible --type 12AB --data-file=" + tmpfile.Name(),
			wantErrStr: "data-file read error: file size over 64KiB limit",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homedir := testutils.CreateNewTestWallet(t)
			_, err := doExecTokensCmd(t, homedir, tt.cmdParams)
			if len(tt.wantErrStr) != 0 {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWalletUpdateNonFungibleTokenDataCmd_Flags(t *testing.T) {
	data := make([]byte, maxBinaryFile64KiB+1)
	tmpfile, err := os.CreateTemp(t.TempDir(), "test")
	require.NoError(t, err)
	_, err = tmpfile.Write(data)
	require.NoError(t, err)

	tests := []struct {
		name       string
		cmdParams  string
		want       []byte
		wantErrStr string
	}{
		{
			name:       "both data and data-file specified",
			cmdParams:  "update --token-identifier 12AB --data 1122aabb --data-file=/tmp/test/foo.bin",
			wantErrStr: "if any flags in the group [data data-file] are set none of the others can be; [data data-file] were all set",
		},
		{
			name:       "data-file not found",
			cmdParams:  "update --token-identifier 12AB --data-file=/tmp/test/foo.bin",
			wantErrStr: "data-file read error: stat /tmp/test/foo.bin: no such file or directory",
		},
		{
			name:       "data-file too big",
			cmdParams:  "update --token-identifier 12AB --data-file=" + tmpfile.Name(),
			wantErrStr: "data-file read error: file size over 64KiB limit",
		},
		{
			name:       "update nft: both data flags missing",
			cmdParams:  "update --token-identifier 12AB",
			wantErrStr: "either of ['--data', '--data-file'] flags must be specified",
		},
		{
			name:       "update nft: token id missing",
			cmdParams:  "update",
			wantErrStr: "required flag(s) \"token-identifier\" not set",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homedir := testutils.CreateNewTestWallet(t)
			_, err := doExecTokensCmd(t, homedir, tt.cmdParams)
			if len(tt.wantErrStr) != 0 {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// tokenID == nil means first token will be considered as success
func ensureTokenIndexed(t *testing.T, ctx context.Context, api *rpc.TokensClient, ownerID []byte, tokenID tokenswallet.TokenID) *tokenswallet.TokenUnit {
	var res *tokenswallet.TokenUnit
	require.Eventually(t, func() bool {
		tokenz, err := api.GetTokens(ctx, tokenswallet.Any, ownerID)
		require.NoError(t, err)
		for _, token := range tokenz {
			if tokenID == nil {
				res = token
				return true
			}
			if tokenID.Eq(token.ID) {
				res = token
				return true
			}
		}
		return false
	}, 2*test.WaitDuration, test.WaitTick)
	return res
}

func execTokensCmdWithError(t *testing.T, homedir string, command string, expectedError string) {
	_, err := doExecTokensCmd(t, homedir, command)
	require.ErrorContains(t, err, expectedError)
}

func execTokensCmd(t *testing.T, homedir string, command string) *testutils.TestConsoleWriter {
	outputWriter, err := doExecTokensCmd(t, homedir, command)
	require.NoError(t, err)
	return outputWriter
}

func doExecTokensCmd(t *testing.T, homedir string, command string) (*testutils.TestConsoleWriter, error) {
	outputWriter := &testutils.TestConsoleWriter{}
	ccmd := NewTokenCmd(&clitypes.WalletConfig{
		Base: &clitypes.BaseConfiguration{
			HomeDir:       homedir,
			ConsoleWriter: outputWriter,
			Logger:        logger.New(t),
		},
		WalletHomeDir: filepath.Join(homedir, "wallet"),
	})
	ccmd.SetArgs(strings.Split(command, " "))
	return outputWriter, ccmd.Execute()
}

func randomFungibleTokenTypeID(t *testing.T) types.UnitID {
	unitID, err := tokens.NewRandomFungibleTokenTypeID(nil)
	require.NoError(t, err)
	return unitID
}

func randomNonFungibleTokenTypeID(t *testing.T) types.UnitID {
	unitID, err := tokens.NewRandomNonFungibleTokenTypeID(nil)
	require.NoError(t, err)
	return unitID
}

func randomNonFungibleTokenID(t *testing.T) types.UnitID {
	unitID, err := tokens.NewRandomNonFungibleTokenID(nil)
	require.NoError(t, err)
	return unitID
}
