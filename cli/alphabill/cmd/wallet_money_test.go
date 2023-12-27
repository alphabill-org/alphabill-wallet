package cmd

import (
	"context"
	"crypto"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	test "github.com/alphabill-org/alphabill-wallet/internal/testutils"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/rpc/alphabill"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/alphabill-org/alphabill-wallet/internal/testutils/logger"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/partition"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/backend/client"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/testutil"
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
	outputWriter := &testConsoleWriter{}
	consoleWriter = outputWriter
	homeDir := setupTestHomeDir(t, "wallet-test")

	cmd := New(observability.NewFactory(t))
	args := "wallet --home " + homeDir + " create"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.Execute(context.Background())
	require.NoError(t, err)
	require.True(t, util.FileExists(filepath.Join(homeDir, "wallet", "accounts.db")))
	verifyStdout(t, outputWriter,
		"The following mnemonic key can be used to recover your wallet. Please write it down now, and keep it in a safe, offline place.")
}

func TestWalletCreateCmd_encrypt(t *testing.T) {
	outputWriter := &testConsoleWriter{}
	consoleWriter = outputWriter
	homeDir := setupTestHomeDir(t, "wallet-test")
	logF := observability.NewFactory(t)

	cmd := New(logF)
	pw := "123456"
	cmd.baseCmd.SetArgs(strings.Split("wallet --home "+homeDir+" create --pn "+pw, " "))
	err := cmd.Execute(context.Background())
	require.NoError(t, err)
	require.True(t, util.FileExists(filepath.Join(homeDir, "wallet", "accounts.db")))
	verifyStdout(t, outputWriter,
		"The following mnemonic key can be used to recover your wallet. Please write it down now, and keep it in a safe, offline place.")

	// verify wallet is encrypted
	// failing case: missing password
	cmd = New(logF)
	cmd.baseCmd.SetArgs(strings.Split("wallet --home "+homeDir+" add-key", " "))
	err = cmd.Execute(context.Background())
	require.ErrorContains(t, err, "invalid password")
	// failing case: wrong password
	cmd = New(logF)
	cmd.baseCmd.SetArgs(strings.Split("wallet --home "+homeDir+" add-key --pn 123", " "))
	err = cmd.Execute(context.Background())
	require.ErrorContains(t, err, "invalid password")
	// passing case:
	cmd = New(logF)
	cmd.baseCmd.SetArgs(strings.Split("wallet --home "+homeDir+" add-key --pn "+pw, " "))
	err = cmd.Execute(context.Background())
	require.NoError(t, err)
}

func TestWalletCreateCmd_invalidSeed(t *testing.T) {
	outputWriter := &testConsoleWriter{}
	consoleWriter = outputWriter
	homeDir := setupTestHomeDir(t, "wallet-test")

	cmd := New(observability.NewFactory(t))
	cmd.baseCmd.SetArgs(strings.Split("wallet create -s --wallet-location "+homeDir, " "))
	err := cmd.Execute(context.Background())
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
	stdout, _ := execCommand(obsF, homedir, "get-balance --key 2 --alphabill-api-uri "+addr.Host)
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
	logF := observability.NewFactory(t)
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15 * 1e8})
	defer mockServer.Close()

	// verify quiet flag does nothing if no key or total flag is not provided
	stdout, _ := execCommand(logF, homedir, "get-balance --quiet --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "#1 15")
	verifyStdout(t, stdout, "Total 15")

	// verify quiet with total
	stdout, _ = execCommand(logF, homedir, "get-balance --quiet --total --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "15")
	verifyStdoutNotExists(t, stdout, "#1 15")

	// verify quiet with key
	stdout, _ = execCommand(logF, homedir, "get-balance --quiet --key 1 --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "15")
	verifyStdoutNotExists(t, stdout, "Total 15")

	// verify quiet with key and total (total is not shown if key is provided)
	stdout, _ = execCommand(logF, homedir, "get-balance --quiet --key 1 --total --alphabill-api-uri "+addr.Host)
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

func createMoneyPartition(t *testing.T, genesisConfig *testutil.MoneyGenesisConfig, nodeCount uint8) *testpartition.NodePartition {
	genesisState := testutil.MoneyGenesisState(t, genesisConfig)
	moneyPart, err := testpartition.NewPartition(t, nodeCount, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		genesisState = genesisState.Clone()
		system, err := money.NewTxSystem(
			logger.New(t),
			money.WithSystemIdentifier(money.DefaultSystemIdentifier),
			money.WithHashAlgorithm(crypto.SHA256),
			money.WithSystemDescriptionRecords([]*genesis.SystemDescriptionRecord{
				{
					SystemIdentifier: money.DefaultSystemIdentifier,
					T2Timeout:        defaultT2Timeout,
					FeeCreditBill: &genesis.FeeCreditBill{
						UnitId:         money.NewBillID(nil, []byte{2}),
						OwnerPredicate: templates.AlwaysTrueBytes(),
					},
				},
			}),
			money.WithTrustBase(tb),
			money.WithState(genesisState),
		)
		require.NoError(t, err)
		return system
	}, money.DefaultSystemIdentifier, genesisState)
	require.NoError(t, err)
	return moneyPart
}

func startAlphabill(t *testing.T, partitions []*testpartition.NodePartition) *testpartition.AlphabillNetwork {
	abNetwork, err := testpartition.NewAlphabillPartition(partitions)
	require.NoError(t, err)
	require.NoError(t, abNetwork.Start(t))
	t.Cleanup(func() { abNetwork.WaitClose(t) })

	return abNetwork
}

func startPartitionRPCServers(t *testing.T, partition *testpartition.NodePartition) {
	for _, n := range partition.Nodes {
		n.AddrGRPC = startRPCServer(t, n.Node, logger.NOP())
	}
	// wait for partition servers to start
	for _, n := range partition.Nodes {
		require.Eventually(t, func() bool {
			_, err := n.LatestBlockNumber()
			return err == nil
		}, test.WaitDuration, test.WaitTick)
	}
}

func startRPCServer(t *testing.T, node *partition.Node, log *slog.Logger) string {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcServer, err := initRPCServer(node, &grpcServerConfiguration{
		Address:               listener.Addr().String(),
		MaxGetBlocksBatchSize: defaultMaxGetBlocksBatchSize,
		MaxRecvMsgSize:        defaultMaxRecvMsgSize,
		MaxSendMsgSize:        defaultMaxSendMsgSize,
	}, observability.Default(t), log)
	require.NoError(t, err)

	t.Cleanup(func() {
		grpcServer.GracefulStop()
	})

	go func() {
		require.NoError(t, grpcServer.Serve(listener), "gRPC server exited with error")
	}()

	return listener.Addr().String()
}

func initRPCServer(node *partition.Node, cfg *grpcServerConfiguration, obs partition.Observability, log *slog.Logger) (*grpc.Server, error) {
	grpcServer := grpc.NewServer(
		grpc.MaxSendMsgSize(cfg.MaxSendMsgSize),
		grpc.MaxRecvMsgSize(cfg.MaxRecvMsgSize),
		grpc.KeepaliveParams(cfg.GrpcKeepAliveServerParameters()),
		grpc.UnaryInterceptor(rpc.InstrumentMetricsUnaryServerInterceptor(obs.Meter(rpc.MetricsScopeGRPCAPI), log)),
		grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithTracerProvider(obs.TracerProvider()))),
	)
	grpc_health_v1.RegisterHealthServer(grpcServer, health.NewServer())

	rpcServer, err := rpc.NewGRPCServer(node, obs, rpc.WithMaxGetBlocksBatchSize(cfg.MaxGetBlocksBatchSize))
	if err != nil {
		return nil, err
	}

	alphabill.RegisterAlphabillServiceServer(grpcServer, rpcServer)
	return grpcServer, nil
}

// addAccount calls "add-key" cli function on given wallet and returns the added pubkey hex
func addAccount(t *testing.T, obsF Factory, homedir string) string {
	stdout := execWalletCmd(t, obsF, homedir, "add-key")
	for _, line := range stdout.lines {
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

func verifyStdout(t *testing.T, consoleWriter *testConsoleWriter, expectedLines ...string) {
	joined := consoleWriter.String()
	for _, expectedLine := range expectedLines {
		require.Contains(t, joined, expectedLine)
	}
}

func verifyStdoutNotExists(t *testing.T, consoleWriter *testConsoleWriter, expectedLines ...string) {
	for _, expectedLine := range expectedLines {
		require.NotContains(t, consoleWriter.lines, expectedLine)
	}
}

func execCommand(obsF Factory, homeDir, command string) (*testConsoleWriter, error) {
	outputWriter := &testConsoleWriter{}
	consoleWriter = outputWriter

	cmd := New(obsF)
	args := "wallet --home " + homeDir + " " + command
	cmd.baseCmd.SetArgs(strings.Split(args, " "))

	return outputWriter, cmd.Execute(context.Background())
}

func execWalletCmd(t *testing.T, obsF Factory, homedir string, command string) *testConsoleWriter {
	outputWriter := &testConsoleWriter{}
	consoleWriter = outputWriter

	cmd := New(obsF)
	args := fmt.Sprintf("wallet --home %s %s", homedir, command)
	cmd.baseCmd.SetArgs(strings.Split(args, " "))

	err := cmd.Execute(context.Background())
	require.NoError(t, err)

	return outputWriter
}

type testConsoleWriter struct {
	lines []string
}

func (w *testConsoleWriter) String() string {
	return strings.Join(w.lines, "\n")
}

func (w *testConsoleWriter) Println(a ...any) {
	s := fmt.Sprintln(a...)
	w.lines = append(w.lines, s[:len(s)-1]) // remove newline
}

func (w *testConsoleWriter) Print(a ...any) {
	w.Println(a...)
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
