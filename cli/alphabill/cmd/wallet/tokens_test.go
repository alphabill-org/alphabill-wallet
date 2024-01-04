package wallet

import (
	"bytes"
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/alphabill-org/alphabill/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	cmdtypes "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	test "github.com/alphabill-org/alphabill-wallet/internal/testutils"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/logger"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	testpartition "github.com/alphabill-org/alphabill-wallet/internal/testutils/partition"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/fees"
	tw "github.com/alphabill-org/alphabill-wallet/wallet/tokens"
	"github.com/alphabill-org/alphabill-wallet/wallet/tokens/backend"
	"github.com/alphabill-org/alphabill-wallet/wallet/tokens/client"
)

func TestListTokensCommandInputs(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		accountNumber uint64
		expectedKind  backend.Kind
		expectedPass  string
		expectedFlags []string
	}{
		{
			name:          "list all tokens",
			args:          []string{},
			accountNumber: 0, // all tokens
			expectedKind:  backend.Any,
		},
		{
			name:          "list all tokens with flags",
			args:          []string{"--with-all", "--with-type-name", "--with-token-uri", "--with-token-data"},
			expectedKind:  backend.Any,
			expectedFlags: []string{cmdFlagWithAll, cmdFlagWithTypeName, cmdFlagWithTokenURI, cmdFlagWithTokenData},
		},
		{
			name:          "list all tokens, encrypted wallet",
			args:          []string{"--pn", "some pass phrase"},
			accountNumber: 0, // all tokens
			expectedKind:  backend.Any,
			expectedPass:  "some pass phrase",
		},
		{
			name:          "list account tokens",
			args:          []string{"--key", "3"},
			accountNumber: 3,
			expectedKind:  backend.Any,
		},
		{
			name:          "list all fungible tokens",
			args:          []string{"fungible"},
			accountNumber: 0,
			expectedKind:  backend.Fungible,
		},
		{
			name:          "list account fungible tokens",
			args:          []string{"fungible", "--key", "4"},
			accountNumber: 4,
			expectedKind:  backend.Fungible,
		},
		{
			name:          "list account fungible tokens, encrypted wallet",
			args:          []string{"fungible", "--key", "4", "--pn", "some pass phrase"},
			accountNumber: 4,
			expectedKind:  backend.Fungible,
			expectedPass:  "some pass phrase",
		},
		{
			name:          "list all fungible tokens with falgs",
			args:          []string{"fungible", "--with-all", "--with-type-name"},
			expectedKind:  backend.Fungible,
			expectedFlags: []string{cmdFlagWithAll, cmdFlagWithTypeName},
		},
		{
			name:          "list all non-fungible tokens",
			args:          []string{"non-fungible"},
			accountNumber: 0,
			expectedKind:  backend.NonFungible,
		},
		{
			name:          "list all non-fungible tokens with flags",
			args:          []string{"non-fungible", "--with-all", "--with-type-name", "--with-token-uri", "--with-token-data"},
			expectedKind:  backend.NonFungible,
			expectedFlags: []string{cmdFlagWithAll, cmdFlagWithTypeName, cmdFlagWithTokenURI, cmdFlagWithTokenData},
		},
		{
			name:          "list account non-fungible tokens",
			args:          []string{"non-fungible", "--key", "5"},
			accountNumber: 5,
			expectedKind:  backend.NonFungible,
		},
		{
			name:          "list account non-fungible tokens, encrypted wallet",
			args:          []string{"non-fungible", "--key", "5", "--pn", "some pass phrase"},
			accountNumber: 5,
			expectedKind:  backend.NonFungible,
			expectedPass:  "some pass phrase",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := false
			cmd := tokenCmdList(&WalletConfig{}, func(cmd *cobra.Command, config *WalletConfig, accountNumber *uint64, kind backend.Kind) error {
				require.Equal(t, tt.accountNumber, *accountNumber)
				require.Equal(t, tt.expectedKind, kind)
				if len(tt.expectedPass) > 0 {
					passwordFromArg, err := cmd.Flags().GetString(passwordArgCmdName)
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
		expectedKind  backend.Kind
		expectedPass  string
	}{
		{
			name:         "list all tokens",
			args:         []string{},
			expectedKind: backend.Any,
		},
		{
			name:         "list all tokens, encrypted wallet",
			args:         []string{"--pn", "test pass phrase"},
			expectedKind: backend.Any,
			expectedPass: "test pass phrase",
		},
		{
			name:          "list all fungible tokens",
			args:          []string{"fungible", "-k", "0"},
			expectedKind:  backend.Fungible,
			expectedAccNr: 0,
		},
		{
			name:          "list all fungible tokens, encrypted wallet",
			args:          []string{"fungible", "--pn", "test pass phrase"},
			expectedKind:  backend.Fungible,
			expectedPass:  "test pass phrase",
			expectedAccNr: 0,
		},
		{
			name:          "list all non-fungible tokens",
			args:          []string{"non-fungible", "--key", "1"},
			expectedKind:  backend.NonFungible,
			expectedAccNr: 1,
		},
		{
			name:          "list all non-fungible tokens, encrypted wallet",
			args:          []string{"non-fungible", "--pn", "test pass phrase", "-k", "2"},
			expectedKind:  backend.NonFungible,
			expectedPass:  "test pass phrase",
			expectedAccNr: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := false
			cmd := tokenCmdListTypes(&WalletConfig{}, func(cmd *cobra.Command, config *WalletConfig, accountNumber *uint64, kind backend.Kind) error {
				require.Equal(t, tt.expectedAccNr, *accountNumber)
				require.Equal(t, tt.expectedKind, kind)
				if len(tt.expectedPass) != 0 {
					passwordFromArg, err := cmd.Flags().GetString(passwordArgCmdName)
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
	logF := observability.NewFactory(t)
	homedir := createNewTestWallet(t)
	// missing symbol parameter
	_, err := execCommand(logF, homedir, "token new-type fungible --decimals 3")
	require.ErrorContains(t, err, "required flag(s) \"symbol\" not set")
	// symbol parameter not set
	_, err = execCommand(logF, homedir, "token new-type fungible --symbol")
	require.EqualError(t, err, `flag needs an argument: --symbol`)
	// there currently are no restrictions on symbol length on CLI side
}

func TestWalletCreateFungibleTokenTypeCmd_TypeIdlFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	logF := observability.NewFactory(t)
	// hidden parameter type (not a mandatory parameter)
	_, err := execCommand(logF, homedir, "token new-type fungible --symbol \"@1\" --type")
	require.ErrorContains(t, err, "flag needs an argument: --type")
	_, err = execCommand(logF, homedir, "token new-type fungible --symbol \"@1\" --type 011")
	require.ErrorContains(t, err, "invalid argument \"011\" for \"--type\" flag: encoding/hex: odd length hex string")
	_, err = execCommand(logF, homedir, "token new-type fungible --symbol \"@1\" --type foo")
	require.ErrorContains(t, err, "invalid argument \"foo\" for \"--type\" flag")
	// there currently are no restrictions on type length on CLI side
}

func TestWalletCreateFungibleTokenTypeCmd_DecimalsFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	logF := observability.NewFactory(t)
	// hidden parameter type (not a mandatory parameter)
	_, err := execCommand(logF, homedir, "token new-type fungible --symbol \"@1\" --decimals")
	require.ErrorContains(t, err, "flag needs an argument: --decimals")
	_, err = execCommand(logF, homedir, "token new-type fungible --symbol \"@1\" --decimals foo")
	require.ErrorContains(t, err, "invalid argument \"foo\" for \"--decimals\" flag")
	_, err = execCommand(logF, homedir, "token new-type fungible --symbol \"@1\" --decimals -1")
	require.ErrorContains(t, err, "invalid argument \"-1\" for \"--decimals\"")
	_, err = execCommand(logF, homedir, "token new-type fungible --symbol \"@1\" --decimals 9")
	require.ErrorContains(t, err, "argument \"9\" for \"--decimals\" flag is out of range, max value 8")
}

func TestWalletCreateFungibleTokenCmd_TypeFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	logF := observability.NewFactory(t)
	_, err := execCommand(logF, homedir, "token new fungible --type A8B")
	require.ErrorContains(t, err, "invalid argument \"A8B\" for \"--type\" flag: encoding/hex: odd length hex string")
	_, err = execCommand(logF, homedir, "token new fungible --type nothex")
	require.ErrorContains(t, err, "invalid argument \"nothex\" for \"--type\" flag: encoding/hex: invalid byte")
	_, err = execCommand(logF, homedir, "token new fungible --amount 4")
	require.ErrorContains(t, err, "required flag(s) \"type\" not set")
}

func TestWalletCreateFungibleTokenCmd_AmountFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	_, err := execCommand(observability.NewFactory(t), homedir, "token new fungible --type A8BB")
	require.ErrorContains(t, err, "required flag(s) \"amount\" not set")
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
			args:       args{cmdParams: "token new non-fungible --data 12AB"},
			wantErrStr: "required flag(s) \"type\" not set",
		},
		{
			name:       "missing token type parameter has no value",
			args:       args{cmdParams: "token new non-fungible --type"},
			wantErrStr: "flag needs an argument: --type",
		},
		{
			name:       "type parameter is not hex encoded",
			args:       args{cmdParams: "token new non-fungible --type 11dummy"},
			wantErrStr: "invalid argument \"11dummy\" for \"--type\" flag",
		},
		{
			name:       "type parameter is odd length",
			args:       args{cmdParams: "token new non-fungible --type A8B08"},
			wantErrStr: "invalid argument \"A8B08\" for \"--type\" flag: encoding/hex: odd length hex string",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homedir := createNewTestWallet(t)
			_, err := execCommand(observability.NewFactory(t), homedir, tt.args.cmdParams)
			if len(tt.wantErrStr) != 0 {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWalletCreateNonFungibleTokenCmd_TokenIdFlag(t *testing.T) {
	//token-identifier parameter is odd length
	homedir := createNewTestWallet(t)
	logF := observability.NewFactory(t)
	_, err := execCommand(logF, homedir, "token new non-fungible --type A8B0 --token-identifier A8B09")
	require.ErrorContains(t, err, "invalid argument \"A8B09\" for \"--token-identifier\" flag: encoding/hex: odd length hex string")
	_, err = execCommand(logF, homedir, "token new non-fungible --type A8B0 --token-identifier nothex")
	require.ErrorContains(t, err, "invalid argument \"nothex\" for \"--token-identifier\" flag: encoding/hex: invalid byte")
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
			cmdParams:  "token new non-fungible --type 12AB --data 1122aabb --data-file=/tmp/test/foo.bin",
			wantErrStr: "if any flags in the group [data data-file] are set none of the others can be; [data data-file] were all set",
		},
		{
			name:       "data-file not found",
			cmdParams:  "token new non-fungible --type 12AB --data-file=/tmp/test/foo.bin",
			wantErrStr: "data-file read error: stat /tmp/test/foo.bin: no such file or directory",
		},
		{
			name:       "data-file too big",
			cmdParams:  "token new non-fungible --type 12AB --data-file=" + tmpfile.Name(),
			wantErrStr: "data-file read error: file size over 64KiB limit",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homedir := createNewTestWallet(t)
			_, err := execCommand(observability.NewFactory(t), homedir, tt.cmdParams)
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
			cmdParams:  "token update --token-identifier 12AB --data 1122aabb --data-file=/tmp/test/foo.bin",
			wantErrStr: "if any flags in the group [data data-file] are set none of the others can be; [data data-file] were all set",
		},
		{
			name:       "data-file not found",
			cmdParams:  "token update --token-identifier 12AB --data-file=/tmp/test/foo.bin",
			wantErrStr: "data-file read error: stat /tmp/test/foo.bin: no such file or directory",
		},
		{
			name:       "data-file too big",
			cmdParams:  "token update --token-identifier 12AB --data-file=" + tmpfile.Name(),
			wantErrStr: "data-file read error: file size over 64KiB limit",
		},
		{
			name:       "update nft: both data flags missing",
			cmdParams:  "token update --token-identifier 12AB",
			wantErrStr: "either of ['--data', '--data-file'] flags must be specified",
		},
		{
			name:       "update nft: token id missing",
			cmdParams:  "token update",
			wantErrStr: "required flag(s) \"token-identifier\" not set",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homedir := createNewTestWallet(t)
			_, err := execCommand(observability.NewFactory(t), homedir, tt.cmdParams)
			if len(tt.wantErrStr) != 0 {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// tokenID == nil means first token will be considered as success
func ensureTokenIndexed(t *testing.T, ctx context.Context, api *client.TokenBackend, ownerPubKey []byte, tokenID backend.TokenID) *backend.TokenUnit {
	var res *backend.TokenUnit
	require.Eventually(t, func() bool {
		offset := ""
		for {
			tokens, offset, err := api.GetTokens(ctx, backend.Any, ownerPubKey, offset, 0)
			require.NoError(t, err)
			for _, token := range tokens {
				if tokenID == nil {
					res = token
					return true
				}
				if bytes.Equal(token.ID, tokenID) {
					res = token
					return true
				}
			}
			if offset == "" {
				break
			}
		}
		return false
	}, 2*test.WaitDuration, test.WaitTick)
	return res
}

func ensureTokenTypeIndexed(t *testing.T, ctx context.Context, api *client.TokenBackend, creatorPubKey []byte, typeID backend.TokenTypeID) *backend.TokenUnitType {
	var res *backend.TokenUnitType
	require.Eventually(t, func() bool {
		offset := ""
		for {
			types, offset, err := api.GetTokenTypes(ctx, backend.Any, creatorPubKey, offset, 0)
			require.NoError(t, err)
			for _, t := range types {
				if bytes.Equal(t.ID, typeID) {
					res = t
					return true
				}
			}
			if offset == "" {
				break
			}
		}
		return false
	}, 2*test.WaitDuration, test.WaitTick)
	return res
}

func createTokensPartition(t *testing.T) *testpartition.NodePartition {
	tokensState := state.NewEmptyState()
	network, err := testpartition.NewPartition(t, 1,
		func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
			system, err := tokens.NewTxSystem(
				logger.New(t),
				tokens.WithState(tokensState),
				tokens.WithTrustBase(tb),
			)
			require.NoError(t, err)
			return system
		}, tokens.DefaultSystemIdentifier,
	)
	require.NoError(t, err)
	return network
}

func createNewTokenWallet(t *testing.T, addr string) (*tw.Wallet, string) {
	return createNewTokenWalletWithFeeManager(t, addr, nil)
}

func createNewTokenWalletWithFeeManager(t *testing.T, addr string, feeManager *fees.FeeManager) (*tw.Wallet, string) {
	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, "wallet")
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	require.NoError(t, am.CreateKeys(""))

	o := observability.NewFactory(t)
	clientURL, err := url.Parse(addr)
	require.NoError(t, err)
	backendClient := client.New(*clientURL, o.DefaultObserver())
	w, err := tw.New(tokens.DefaultSystemIdentifier, backendClient, am, false, feeManager, o.DefaultLogger())
	require.NoError(t, err)
	require.NotNil(t, w)

	return w, homeDir
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
	ccmd := tokenCmd(&WalletConfig{
		Base:          &cmdtypes.BaseConfiguration{HomeDir: homedir, ConsoleWriter: outputWriter, LogCfgFile: "logger-config.yaml", Observe: observability.Default(t)},
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

func verifyStdoutEventually(t *testing.T, exec func() *testutils.TestConsoleWriter, expectedLines ...string) {
	verifyStdoutEventuallyWithTimeout(t, exec, test.WaitDuration, test.WaitTick, expectedLines...)
}

func verifyStdoutEventuallyWithTimeout(t *testing.T, exec func() *testutils.TestConsoleWriter, waitFor time.Duration, tick time.Duration, expectedLines ...string) {
	require.Eventually(t, func() bool {
		joined := strings.Join(exec().Lines, "\n")
		res := true
		for _, expectedLine := range expectedLines {
			res = res && strings.Contains(joined, expectedLine)
		}
		return res
	}, waitFor, tick)
}
