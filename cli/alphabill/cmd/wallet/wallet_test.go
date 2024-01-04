package wallet

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/backend/client"
)

const walletBaseDir = "wallet"

type (
	backendMockReturnConf struct {
		balance        uint64
		blockHeight    uint64
		targetBill     *wallet.Bill
		feeCreditBill  *wallet.Bill
		proofList      string
		customBillList string
		customPath     string
		customFullPath string
		customResponse string
	}
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
	verifyStdout(t, outputWriter,
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
	verifyStdout(t, outputWriter,
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
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15 * 1e8})
	defer mockServer.Close()
	stdout, _ := execCommand(observability.NewFactory(t), homedir, "get-balance --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "#1 15", "Total 15")
}

func TestWalletGetBalanceKeyCmdKeyFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15 * 1e8})
	defer mockServer.Close()
	obsF := observability.NewFactory(t)
	addAccount(t, obsF, homedir)
	stdout, err := execCommand(obsF, homedir, "get-balance --key 2 --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout, "#2 15")
	verifyStdoutNotExists(t, stdout, "Total 15")
}

func TestWalletGetBalanceCmdTotalFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15 * 1e8})
	defer mockServer.Close()
	stdout, _ := execCommand(observability.NewFactory(t), homedir, "get-balance --total --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "Total 15")
	verifyStdoutNotExists(t, stdout, "#1 15")
}

func TestWalletGetBalanceCmdTotalWithKeyFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15 * 1e8})
	defer mockServer.Close()
	stdout, _ := execCommand(observability.NewFactory(t), homedir, "get-balance --key 1 --total --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "#1 15")
	verifyStdoutNotExists(t, stdout, "Total 15")
}

func TestWalletGetBalanceCmdQuietFlag(t *testing.T) {
	obsF := observability.NewFactory(t)
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15 * 1e8})
	defer mockServer.Close()

	// verify quiet flag does nothing if no key or total flag is not provided
	stdout, _ := execCommand(obsF, homedir, "get-balance --quiet --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "#1 15")
	verifyStdout(t, stdout, "Total 15")

	// verify quiet with total
	stdout, _ = execCommand(obsF, homedir, "get-balance --quiet --total --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "15")
	verifyStdoutNotExists(t, stdout, "#1 15")

	// verify quiet with key
	stdout, _ = execCommand(obsF, homedir, "get-balance --quiet --key 1 --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "15")
	verifyStdoutNotExists(t, stdout, "Total 15")

	// verify quiet with key and total (total is not shown if key is provided)
	stdout, _ = execCommand(obsF, homedir, "get-balance --quiet --key 1 --total --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "15")
	verifyStdoutNotExists(t, stdout, "#1 15")
}

func TestPubKeysCmd(t *testing.T) {
	am, homedir := createNewWallet(t)
	pk, err := am.GetPublicKey(0)
	require.NoError(t, err)
	am.Close()
	stdout, err := execCommand(observability.NewFactory(t), homedir, "get-pubkeys")
	require.NoError(t, err)
	verifyStdout(t, stdout, "#1 "+hexutil.Encode(pk))
}

func TestSendingFailsWithInsufficientBalance(t *testing.T) {
	am, homedir := createNewWallet(t)
	pubKey, err := am.GetPublicKey(0)
	require.NoError(t, err)
	am.Close()

	mockServer, addr := mockBackendCalls(&backendMockReturnConf{
		targetBill:    &wallet.Bill{Id: []byte{8}, Value: 5e8},
		feeCreditBill: &wallet.Bill{Id: []byte{9}},
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

func createNewWallet(t *testing.T) (account.Manager, string) {
	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, walletBaseDir)
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	t.Cleanup(am.Close)
	err = am.CreateKeys("")
	require.NoError(t, err)
	return am, homeDir
}

func createNewTestWallet(t *testing.T) string {
	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, walletBaseDir)
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	defer am.Close()
	err = am.CreateKeys("")
	require.NoError(t, err)
	return homeDir
}

func verifyStdout(t *testing.T, consoleWriter *testutils.TestConsoleWriter, expectedLines ...string) {
	joined := consoleWriter.String()
	for _, expectedLine := range expectedLines {
		require.Contains(t, joined, expectedLine)
	}
}

func verifyStdoutNotExists(t *testing.T, consoleWriter *testutils.TestConsoleWriter, expectedLines ...string) {
	for _, expectedLine := range expectedLines {
		require.NotContains(t, consoleWriter.Lines, expectedLine)
	}
}

func execCommand(obsF Factory, homeDir, command string) (*testutils.TestConsoleWriter, error) {
	outputWriter := &testutils.TestConsoleWriter{}
	wcmd := NewWalletCmd(&types.BaseConfiguration{HomeDir: homeDir, ConsoleWriter: outputWriter, LogCfgFile: "logger-config.yaml"}, obsF)
	wcmd.SetArgs(strings.Split(command, " "))
	return outputWriter, wcmd.Execute()
}

func mockBackendCalls(br *backendMockReturnConf) (*httptest.Server, *url.URL) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == br.customPath || r.URL.RequestURI() == br.customFullPath {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(br.customResponse))
		} else {
			path := r.URL.Path
			switch {
			case path == "/"+client.BalancePath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fmt.Sprintf(`{"balance": "%d"}`, br.balance)))
			case path == "/"+client.RoundNumberPath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fmt.Sprintf(`{"blockHeight": "%d"}`, br.blockHeight)))
			case path == "/api/v1/units/":
				if br.proofList != "" {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(br.proofList))
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
			case path == "/"+client.ListBillsPath:
				w.WriteHeader(http.StatusOK)
				if br.customBillList != "" {
					w.Write([]byte(br.customBillList))
				} else {
					b, _ := json.Marshal(br.targetBill)
					w.Write([]byte(fmt.Sprintf(`{"bills": [%s]}`, b)))
				}
			case strings.Contains(path, client.FeeCreditPath):
				w.WriteHeader(http.StatusOK)
				fcb, _ := json.Marshal(br.feeCreditBill)
				w.Write(fcb)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}

func toBase64(bytes []byte) string {
	return base64.StdEncoding.EncodeToString(bytes)
}
