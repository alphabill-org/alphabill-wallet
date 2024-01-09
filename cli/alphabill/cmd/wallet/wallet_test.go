package wallet

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils/mocksrv"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	"github.com/alphabill-org/alphabill-wallet/wallet"
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
	homedir := testutils.CreateNewTestWallet(t)
	mockServer, addr := mocksrv.MockBackendCalls(&mocksrv.BackendMockReturnConf{Balance: 15 * 1e8})
	defer mockServer.Close()
	stdout, _ := execCommand(observability.NewFactory(t), homedir, "get-balance --alphabill-api-uri "+addr.Host)
	testutils.VerifyStdout(t, stdout, "#1 15", "Total 15")
}

func TestWalletGetBalanceKeyCmdKeyFlag(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	mockServer, addr := mocksrv.MockBackendCalls(&mocksrv.BackendMockReturnConf{Balance: 15 * 1e8})
	defer mockServer.Close()
	obsF := observability.NewFactory(t)
	addAccount(t, obsF, homedir)
	stdout, err := execCommand(obsF, homedir, "get-balance --key 2 --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout, "#2 15")
	testutils.VerifyStdoutNotExists(t, stdout, "Total 15")
}

func TestWalletGetBalanceCmdTotalFlag(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	mockServer, addr := mocksrv.MockBackendCalls(&mocksrv.BackendMockReturnConf{Balance: 15 * 1e8})
	defer mockServer.Close()
	stdout, _ := execCommand(observability.NewFactory(t), homedir, "get-balance --total --alphabill-api-uri "+addr.Host)
	testutils.VerifyStdout(t, stdout, "Total 15")
	testutils.VerifyStdoutNotExists(t, stdout, "#1 15")
}

func TestWalletGetBalanceCmdTotalWithKeyFlag(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	mockServer, addr := mocksrv.MockBackendCalls(&mocksrv.BackendMockReturnConf{Balance: 15 * 1e8})
	defer mockServer.Close()
	stdout, _ := execCommand(observability.NewFactory(t), homedir, "get-balance --key 1 --total --alphabill-api-uri "+addr.Host)
	testutils.VerifyStdout(t, stdout, "#1 15")
	testutils.VerifyStdoutNotExists(t, stdout, "Total 15")
}

func TestWalletGetBalanceCmdQuietFlag(t *testing.T) {
	obsF := observability.NewFactory(t)
	homedir := testutils.CreateNewTestWallet(t)
	mockServer, addr := mocksrv.MockBackendCalls(&mocksrv.BackendMockReturnConf{Balance: 15 * 1e8})
	defer mockServer.Close()

	// verify quiet flag does nothing if no key or total flag is not provided
	stdout, _ := execCommand(obsF, homedir, "get-balance --quiet --alphabill-api-uri "+addr.Host)
	testutils.VerifyStdout(t, stdout, "#1 15")
	testutils.VerifyStdout(t, stdout, "Total 15")

	// verify quiet with total
	stdout, _ = execCommand(obsF, homedir, "get-balance --quiet --total --alphabill-api-uri "+addr.Host)
	testutils.VerifyStdout(t, stdout, "15")
	testutils.VerifyStdoutNotExists(t, stdout, "#1 15")

	// verify quiet with key
	stdout, _ = execCommand(obsF, homedir, "get-balance --quiet --key 1 --alphabill-api-uri "+addr.Host)
	testutils.VerifyStdout(t, stdout, "15")
	testutils.VerifyStdoutNotExists(t, stdout, "Total 15")

	// verify quiet with key and total (total is not shown if key is provided)
	stdout, _ = execCommand(obsF, homedir, "get-balance --quiet --key 1 --total --alphabill-api-uri "+addr.Host)
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
	am, homedir := testutils.CreateNewWallet(t)
	pubKey, err := am.GetPublicKey(0)
	require.NoError(t, err)
	am.Close()

	mockServer, addr := mocksrv.MockBackendCalls(&mocksrv.BackendMockReturnConf{
		TargetBill:    &wallet.Bill{Id: []byte{8}, Value: 5e8},
		FeeCreditBill: &wallet.Bill{Id: []byte{9}},
	})
	defer mockServer.Close()

	_, err = execCommand(observability.NewFactory(t), homedir, "send --amount 10 --address "+hexutil.Encode(pubKey)+" --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "insufficient balance for transaction")
}

// addAccount calls "add-key" cli function on given wallet and returns the added pubkey hex
func addAccount(t *testing.T, obsF Factory, homedir string) string {
	stdout, err := execCommand(obsF, homedir, "add-key")
	require.NoError(t, err)
	for _, line := range stdout.Lines {
		if strings.HasPrefix(line, "Added key #") {
			return line[13:]
		}
	}
	return ""
}

func execCommand(obsF Factory, homeDir, command string) (*testutils.TestConsoleWriter, error) {
	outputWriter := &testutils.TestConsoleWriter{}
	wcmd := NewWalletCmd(&types.BaseConfiguration{HomeDir: homeDir, ConsoleWriter: outputWriter, LogCfgFile: "logger-config.yaml"}, obsF)
	wcmd.SetArgs(strings.Split(command, " "))
	return outputWriter, wcmd.Execute()
}
