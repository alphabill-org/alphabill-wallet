package wallet

import (
	"path/filepath"
	"strings"
	"testing"

	abrpc "github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	"github.com/alphabill-org/alphabill-wallet/client/rpc/mocksrv"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
)

func TestWalletCreateCmd(t *testing.T) {
	outputWriter := &testutils.TestConsoleWriter{}
	homeDir := testutils.SetupTestHomeDir(t, "wallet-test")
	obsF := observability.NewFactory(t)
	wcmd := NewWalletCmd(&types.BaseConfiguration{HomeDir: homeDir, ConsoleWriter: outputWriter, LogCfgFile: "logger-config.yaml"}, obsF)
	wcmd.SetArgs([]string{"create"})
	err := wcmd.Execute()
	require.NoError(t, err)
	require.True(t, util.FileExists(filepath.Join(homeDir, "wallet", "accounts.db")))
	testutils.VerifyStdout(t, outputWriter,
		"The following mnemonic key can be used to recover your wallet. Please write it down now, and keep it in a safe, offline place.")
}

func TestWalletCreateCmd_encrypt(t *testing.T) {
	outputWriter := &testutils.TestConsoleWriter{}
	homeDir := testutils.SetupTestHomeDir(t, "wallet-test")
	obsF := observability.NewFactory(t)
	wcmd := NewWalletCmd(&types.BaseConfiguration{HomeDir: homeDir, ConsoleWriter: outputWriter, LogCfgFile: "logger-config.yaml"}, obsF)
	pw := "123456"
	wcmd.SetArgs([]string{"create", "--pn", pw})
	err := wcmd.Execute()
	require.NoError(t, err)
	require.True(t, util.FileExists(filepath.Join(homeDir, "wallet", "accounts.db")))
	testutils.VerifyStdout(t, outputWriter,
		"The following mnemonic key can be used to recover your wallet. Please write it down now, and keep it in a safe, offline place.")

	// verify wallet is encrypted
	// failing case: missing password
	wcmd = NewWalletCmd(&types.BaseConfiguration{HomeDir: homeDir, ConsoleWriter: outputWriter, LogCfgFile: "logger-config.yaml"}, obsF)
	wcmd.SetArgs([]string{"add-key"})
	err = wcmd.Execute()
	require.ErrorContains(t, err, "invalid password")

	// failing case: wrong password
	wcmd = NewWalletCmd(&types.BaseConfiguration{HomeDir: homeDir, ConsoleWriter: outputWriter, LogCfgFile: "logger-config.yaml"}, obsF)
	wcmd.SetArgs([]string{"add-key", "--pn", "123"})
	err = wcmd.Execute()
	require.ErrorContains(t, err, "invalid password")

	// passing case:
	wcmd = NewWalletCmd(&types.BaseConfiguration{HomeDir: homeDir, ConsoleWriter: outputWriter, LogCfgFile: "logger-config.yaml"}, obsF)
	wcmd.SetArgs([]string{"add-key", "--pn", pw})
	err = wcmd.Execute()
	require.NoError(t, err)
}

func TestWalletCreateCmd_invalidSeed(t *testing.T) {
	outputWriter := &testutils.TestConsoleWriter{}
	homeDir := testutils.SetupTestHomeDir(t, "wallet-test")
	obsF := observability.NewFactory(t)
	wcmd := NewWalletCmd(&types.BaseConfiguration{HomeDir: homeDir, ConsoleWriter: outputWriter, LogCfgFile: "logger-config.yaml"}, obsF)
	wcmd.SetArgs([]string{"create", "-s", "--wallet-location", homeDir})
	err := wcmd.Execute()
	require.EqualError(t, err, `invalid value "--wallet-location" for flag "seed" (mnemonic)`)
	require.False(t, util.FileExists(filepath.Join(homeDir, "wallet", "accounts.db")))
}

func TestWalletGetBalanceCmd(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic())
	rpcUrl := mocksrv.StartStateApiServer(t, mocksrv.NewStateServiceMock(
		mocksrv.WithOwnerUnit(&abrpc.Unit[any]{
			UnitID:         money.NewBillID(nil, []byte{1}),
			Data:           money.BillData{V: 15 * 1e8},
			OwnerPredicate: testutils.TestPubKey0Hash(t),
		}),
	))

	stdout, err := execCommand(observability.NewFactory(t), homedir, "get-balance --rpc-url "+rpcUrl)
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "#1 15", "Total 15")
}

func TestWalletGetBalanceKeyCmdKeyFlag(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic(), testutils.WithNumberOfAccounts(2))
	rpcUrl := mocksrv.StartStateApiServer(t, mocksrv.NewStateServiceMock(mocksrv.WithOwnerUnit(&abrpc.Unit[any]{
		UnitID:         money.NewBillID(nil, []byte{1}),
		Data:           money.BillData{V: 15 * 1e8},
		OwnerPredicate: testutils.TestPubKey1Hash(t),
	})))

	obsF := observability.NewFactory(t)
	stdout, err := execCommand(obsF, homedir, "get-balance --key 2 --rpc-url "+rpcUrl)
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "#2 15")
	testutils.VerifyStdoutNotExists(t, stdout, "Total 15")
}

func TestWalletGetBalanceCmdTotalFlag(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic())
	rpcUrl := mocksrv.StartStateApiServer(t, mocksrv.NewStateServiceMock(mocksrv.WithOwnerUnit(&abrpc.Unit[any]{
		UnitID:         money.NewBillID(nil, []byte{1}),
		Data:           money.BillData{V: 15 * 1e8},
		OwnerPredicate: testutils.TestPubKey0Hash(t),
	})))

	stdout, _ := execCommand(observability.NewFactory(t), homedir, "get-balance --total --rpc-url "+rpcUrl)
	testutils.VerifyStdout(t, stdout, "Total 15")
	testutils.VerifyStdoutNotExists(t, stdout, "#1 15")
}

func TestWalletGetBalanceCmdTotalWithKeyFlag(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic())
	rpcUrl := mocksrv.StartStateApiServer(t, mocksrv.NewStateServiceMock(mocksrv.WithOwnerUnit(&abrpc.Unit[any]{
		UnitID:         money.NewBillID(nil, []byte{1}),
		Data:           money.BillData{V: 15 * 1e8},
		OwnerPredicate: testutils.TestPubKey0Hash(t),
	})))

	stdout, _ := execCommand(observability.NewFactory(t), homedir, "get-balance --key 1 --total --rpc-url "+rpcUrl)
	testutils.VerifyStdout(t, stdout, "#1 15")
	testutils.VerifyStdoutNotExists(t, stdout, "Total 15")
}

func TestWalletGetBalanceCmdQuietFlag(t *testing.T) {
	obsF := observability.NewFactory(t)
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic())
	rpcUrl := mocksrv.StartStateApiServer(t, mocksrv.NewStateServiceMock(mocksrv.WithOwnerUnit(&abrpc.Unit[any]{
		UnitID:         money.NewBillID(nil, []byte{1}),
		Data:           money.BillData{V: 15 * 1e8},
		OwnerPredicate: testutils.TestPubKey0Hash(t),
	})))

	// verify quiet flag does nothing if no key or total flag is not provided
	stdout, _ := execCommand(obsF, homedir, "get-balance --quiet --rpc-url "+rpcUrl)
	testutils.VerifyStdout(t, stdout, "#1 15")
	testutils.VerifyStdout(t, stdout, "Total 15")

	// verify quiet with total
	stdout, _ = execCommand(obsF, homedir, "get-balance --quiet --total --rpc-url "+rpcUrl)
	testutils.VerifyStdout(t, stdout, "15")
	testutils.VerifyStdoutNotExists(t, stdout, "#1 15")

	// verify quiet with key
	stdout, _ = execCommand(obsF, homedir, "get-balance --quiet --key 1 --rpc-url "+rpcUrl)
	testutils.VerifyStdout(t, stdout, "15")
	testutils.VerifyStdoutNotExists(t, stdout, "Total 15")

	// verify quiet with key and total (total is not shown if key is provided)
	stdout, _ = execCommand(obsF, homedir, "get-balance --quiet --key 1 --total --rpc-url "+rpcUrl)
	testutils.VerifyStdout(t, stdout, "15")
	testutils.VerifyStdoutNotExists(t, stdout, "#1 15")
}

func TestPubKeysCmd(t *testing.T) {
	am, homedir := testutils.CreateNewWallet(t)
	pk, err := am.GetPublicKey(0)
	require.NoError(t, err)
	am.Close()
	stdout, err := execCommand(observability.NewFactory(t), homedir, "get-pubkeys")
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "#1 "+hexutil.Encode(pk))
}

func TestSendingFailsWithInsufficientBalance(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t, testutils.WithDefaultMnemonic())
	rpcUrl := mocksrv.StartStateApiServer(t, mocksrv.NewStateServiceMock(
		mocksrv.WithOwnerUnit(&abrpc.Unit[any]{
			UnitID:         money.NewBillID(nil, []byte{8}),
			Data:           money.BillData{V: 5 * 1e8},
			OwnerPredicate: testutils.TestPubKey0Hash(t),
		}),
		mocksrv.WithOwnerUnit(&abrpc.Unit[any]{
			UnitID:         money.NewFeeCreditRecordID(nil, testutils.TestPubKey0Hash(t)),
			Data:           unit.FeeCreditRecord{Balance: 1e8},
			OwnerPredicate: testutils.TestPubKey0Hash(t),
		}),
	))

	_, err := execCommand(observability.NewFactory(t), homedir, "send --amount 10 --address 0x"+testutils.TestPubKey1Hex+" --rpc-url "+rpcUrl)
	require.ErrorContains(t, err, "insufficient balance for transaction")
}

func execCommand(obsF Factory, homeDir, command string) (*testutils.TestConsoleWriter, error) {
	outputWriter := &testutils.TestConsoleWriter{}
	wcmd := NewWalletCmd(&types.BaseConfiguration{HomeDir: homeDir, ConsoleWriter: outputWriter, LogCfgFile: "logger-config.yaml"}, obsF)
	wcmd.SetArgs(strings.Split(command, " "))
	return outputWriter, wcmd.Execute()
}
