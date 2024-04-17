package evm

import (
	"bytes"
	"crypto"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/evm"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill/logger"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	cmdtypes "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils"
	testobserve "github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
)

func Test_evmCmdDeploy_error_cases(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	logF := testobserve.Default(t)
	// balance is returned by EVM in wei 10^-18
	mockServer, addr := mockClientCalls(&clientMockConf{balance: "15000000000000000000", backlink: make([]byte, 32)}, logF.Logger)
	defer mockServer.Close()
	_, err := execEvmCmd(logF, homedir, "evm deploy --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "required flag(s) \"data\", \"max-gas\" not set")
	_, err = execEvmCmd(logF, homedir, "evm deploy --max-gas 10000 --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "required flag(s) \"data\" not set")
	_, err = execEvmCmd(logF, homedir, "evm deploy --data accbdeef --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "required flag(s) \"max-gas\" not set")
	// smart contract code too big
	code := hex.EncodeToString(make([]byte, ScSizeLimit24Kb+1))
	_, err = execEvmCmd(logF, homedir, "evm deploy --max-gas 10000 --data "+code+" --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "contract code too big, maximum size is 24Kb")
	_, err = execEvmCmd(logF, homedir, "evm deploy --max-gas 1000 --data accbxdeef --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "failed to read 'data' parameter: hex decode error: encoding/hex: invalid byte: U+0078 'x'")
	_, err = execEvmCmd(logF, homedir, "evm deploy --max-gas abba --data accbdeef --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "invalid argument \"abba\" for \"--max-gas\"")
}

func Test_evmCmdDeploy_ok(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	evmDetails := evm.ProcessingDetails{
		ErrorDetails: "something went wrong",
	}
	detailBytes, err := types.Cbor.Marshal(evmDetails)
	require.NoError(t, err)
	mockConf := &clientMockConf{
		round:    3,
		balance:  "15000000000000000000", // balance is returned by EVM in wei 10^-18
		backlink: make([]byte, 32),
		nonce:    1,
		gasPrice: "10000",
		serverMeta: &types.ServerMetadata{
			ActualFee:         21000,
			TargetUnits:       []types.UnitID{test.RandomBytes(20)},
			SuccessIndicator:  types.TxStatusFailed,
			ProcessingDetails: detailBytes,
		},
	}
	logF := testobserve.Default(t)
	mockServer, addr := mockClientCalls(mockConf, logF.Logger)
	defer mockServer.Close()
	stdout, err := execEvmCmd(logF, homedir, "evm deploy --max-gas 10000 --data 9021ACFE0102 --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout,
		"Evm transaction failed: something went wrong",
		"Evm transaction processing fee: 0.000'210'00")
	// verify tx order
	require.Equal(t, "evm", mockConf.receivedTx.PayloadType())
	evmAttributes := &evm.TxAttributes{}
	require.NoError(t, mockConf.receivedTx.UnmarshalAttributes(evmAttributes))
	// verify attributes set by cli cmd
	data, err := hex.DecodeString("9021ACFE0102")
	require.NoError(t, err)
	require.NotNil(t, evmAttributes.From)
	require.Nil(t, evmAttributes.To)
	//value is currently hardcoded as 0
	require.Equal(t, big.NewInt(0), evmAttributes.Value)
	require.EqualValues(t, data, evmAttributes.Data)
	require.EqualValues(t, 10000, evmAttributes.Gas)
	// nonce is read from evm
	require.EqualValues(t, 1, evmAttributes.Nonce)
}

func Test_evmCmdExecute_error_cases(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	logF := testobserve.Default(t)
	// balance is returned by EVM in wei 10^-18
	mockServer, addr := mockClientCalls(&clientMockConf{balance: "15000000000000000000", backlink: make([]byte, 32), gasPrice: "20000000000000000000"}, logF.Logger)
	defer mockServer.Close()
	_, err := execEvmCmd(logF, homedir, "evm execute --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "required flag(s) \"address\", \"data\", \"max-gas\" not set")
	_, err = execEvmCmd(logF, homedir, "evm execute --max-gas 10000 --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "required flag(s) \"address\", \"data\" not set")
	_, err = execEvmCmd(logF, homedir, "evm execute --data accbdeee --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "required flag(s) \"address\", \"max-gas\" not set")
	_, err = execEvmCmd(logF, homedir, "evm execute --max-gas 1000 --address aabbccddeeff --data aabbccdd --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "invalid address aabbccddeeff, address must be 20 bytes")
	_, err = execEvmCmd(logF, homedir, "evm execute --max-gas 1000 --address 3443919fcbc4476b4f332fd5df6a82fe88dbf521 --data aabbkccdd --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "failed to read 'data' parameter: hex decode error: encoding/hex: invalid byte: U+006B 'k'")
	_, err = execEvmCmd(logF, homedir, "evm execute --max-gas 1 --address 3443919fcbc4476b4f332fd5df6a82fe88dbf521 --data aabbccdd --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "insufficient fee credit balance for transaction")
}

func Test_evmCmdExecute_ok(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	evmDetails := evm.ProcessingDetails{
		ReturnData: []byte{0xDE, 0xAD, 0x00, 0xBE, 0xEF},
	}
	detailBytes, err := types.Cbor.Marshal(evmDetails)
	require.NoError(t, err)
	mockConf := &clientMockConf{
		round:    3,
		balance:  "15000000000000000000", // balance is returned by EVM in wei 10^-18
		backlink: make([]byte, 32),
		nonce:    1,
		gasPrice: "10000",
		serverMeta: &types.ServerMetadata{
			ActualFee:         21000,
			TargetUnits:       []types.UnitID{test.RandomBytes(20)},
			SuccessIndicator:  types.TxStatusSuccessful,
			ProcessingDetails: detailBytes,
		},
	}
	logF := testobserve.Default(t)
	mockServer, addr := mockClientCalls(mockConf, logF.Logger)
	defer mockServer.Close()
	stdout, err := execEvmCmd(logF, homedir, "evm execute --address 3443919fcbc4476b4f332fd5df6a82fe88dbf521 --max-gas 10000 --data 9021ACFE --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout,
		"Evm transaction succeeded",
		"Evm transaction processing fee: 0.000'210'00",
		"Evm execution returned: DEAD00BEEF")
	// verify tx order
	require.Equal(t, "evm", mockConf.receivedTx.PayloadType())
	evmAttributes := &evm.TxAttributes{}
	require.NoError(t, mockConf.receivedTx.UnmarshalAttributes(evmAttributes))
	// verify attributes set by cli cmd
	require.NoError(t, err)
	require.NotNil(t, evmAttributes.From)
	toAddr, err := hex.DecodeString("3443919fcbc4476b4f332fd5df6a82fe88dbf521")
	require.NoError(t, err)
	require.EqualValues(t, toAddr, evmAttributes.To)
	//value is currently hardcoded as 0
	require.Equal(t, big.NewInt(0), evmAttributes.Value)
	data, err := hex.DecodeString("9021ACFE")
	require.NoError(t, err)
	require.EqualValues(t, data, evmAttributes.Data)
	require.EqualValues(t, 10000, evmAttributes.Gas)
	// nonce is read from evm
	require.EqualValues(t, 1, evmAttributes.Nonce)
}

func Test_evmCmdCall_error_cases(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	logF := testobserve.Default(t)
	// balance is returned by EVM in wei 10^-18
	mockServer, addr := mockClientCalls(&clientMockConf{balance: "15000000000000000000", backlink: make([]byte, 32)}, logF.Logger)
	defer mockServer.Close()
	_, err := execEvmCmd(logF, homedir, "evm call --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "required flag(s) \"address\", \"data\" not set")
	_, err = execEvmCmd(logF, homedir, "evm call --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "required flag(s) \"address\", \"data\" not set")
	_, err = execEvmCmd(logF, homedir, "evm call --data accbdeee --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "required flag(s) \"address\" not set")
	_, err = execEvmCmd(logF, homedir, "evm call --max-gas 1000 --address aabbccddeeff --data aabbccdd --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "invalid address aabbccddeeff, address must be 20 bytes")
	_, err = execEvmCmd(logF, homedir, "evm call --max-gas 1000 --address 3443919fcbc4476b4f332fd5df6a82fe88dbf521 --data aabbkccdd --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "failed to read 'data' parameter: hex decode error: encoding/hex: invalid byte: U+006B 'k'")
}

func Test_evmCmdCall_ok(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	evmDetails := &evm.ProcessingDetails{
		ReturnData: []byte{0xDE, 0xAD, 0x00, 0xBE, 0xEF},
	}
	mockConf := &clientMockConf{
		round:    3,
		balance:  "15000000000000000000", // balance is returned by EVM in wei 10^-18
		backlink: make([]byte, 32),
		nonce:    1,
		callResp: &evm.CallEVMResponse{
			ProcessingDetails: evmDetails,
		},
	}
	logF := testobserve.Default(t)
	mockServer, addr := mockClientCalls(mockConf, logF.Logger)
	defer mockServer.Close()
	stdout, err := execEvmCmd(logF, homedir, "evm call --address 3443919fcbc4476b4f332fd5df6a82fe88dbf521 --max-gas 10000 --data 9021ACFE --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout,
		"Evm transaction succeeded",
		"Evm transaction processing fee: 0.000'000'00",
		"Evm execution returned: DEAD00BEEF")
	// verify call attributes sent
	require.NotNil(t, mockConf.callReq.From)
	toAddr, err := hex.DecodeString("3443919fcbc4476b4f332fd5df6a82fe88dbf521")
	require.NoError(t, err)
	require.EqualValues(t, toAddr, mockConf.callReq.To)
	//value is currently hardcoded as 0
	require.Equal(t, big.NewInt(0), mockConf.callReq.Value)
	data, err := hex.DecodeString("9021ACFE")
	require.NoError(t, err)
	require.EqualValues(t, data, mockConf.callReq.Data)
	require.EqualValues(t, 10000, mockConf.callReq.Gas)
}

func Test_evmCmdCall_ok_defaultGas(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	evmDetails := &evm.ProcessingDetails{
		ReturnData: []byte{0xDE, 0xAD, 0x00, 0xBE, 0xEF},
	}
	mockConf := &clientMockConf{
		round:    3,
		balance:  "15000000000000000000", // balance is returned by EVM in wei 10^-18
		backlink: make([]byte, 32),
		nonce:    1,
		callResp: &evm.CallEVMResponse{
			ProcessingDetails: evmDetails,
		},
	}
	logF := testobserve.Default(t)
	mockServer, addr := mockClientCalls(mockConf, logF.Logger)
	defer mockServer.Close()
	stdout, err := execEvmCmd(logF, homedir, "evm call --address 3443919fcbc4476b4f332fd5df6a82fe88dbf521 --data 9021ACFE --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	testutils.VerifyStdout(t, stdout,
		"Evm transaction succeeded",
		"Evm transaction processing fee: 0.000'000'00",
		"Evm execution returned: DEAD00BEEF")
	// verify call attributes sent
	require.NotNil(t, mockConf.callReq.From)
	toAddr, err := hex.DecodeString("3443919fcbc4476b4f332fd5df6a82fe88dbf521")
	require.NoError(t, err)
	require.EqualValues(t, toAddr, mockConf.callReq.To)
	//value is currently hardcoded as 0
	require.Equal(t, big.NewInt(0), mockConf.callReq.Value)
	require.EqualValues(t, DefaultCallMaxGas, mockConf.callReq.Gas)
	data, err := hex.DecodeString("9021ACFE")
	require.NoError(t, err)
	require.EqualValues(t, data, mockConf.callReq.Data)
}

func Test_evmCmdBalance(t *testing.T) {
	homedir := testutils.CreateNewTestWallet(t)
	logF := testobserve.Default(t)
	// balance is returned by EVM in wei 10^-18
	mockServer, addr := mockClientCalls(&clientMockConf{balance: "15000000000000000000", backlink: make([]byte, 32)}, logF.Logger)
	defer mockServer.Close()
	stdout, _ := execEvmCmd(logF, homedir, "evm balance --alphabill-api-uri "+addr.Host)
	testutils.VerifyStdout(t, stdout, "#1 15.000'000'00 (eth: 15.000'000'000'000'000'000)")
	// -k 2 -> no such account
	_, err := execEvmCmd(logF, homedir, "evm balance -k 2 --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "get balance failed, account key read failed: account does not exist")
}

type clientMockConf struct {
	balance    string
	backlink   []byte
	round      uint64
	nonce      uint64
	gasPrice   string
	receivedTx *types.TransactionOrder
	serverMeta *types.ServerMetadata
	callReq    *evm.CallEVMRequest
	callResp   *evm.CallEVMResponse
}

func mockClientCalls(br *clientMockConf, logF func() *slog.Logger) (*httptest.Server, *url.URL) {
	log := logF()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/api/v1/evm/balance/"):
			writeCBORResponse(w, &struct {
				_        struct{} `cbor:",toarray"`
				Balance  string
				Backlink []byte
			}{
				Balance:  br.balance,
				Backlink: br.backlink,
			}, http.StatusOK, log)
		case strings.Contains(r.URL.Path, "/api/v1/evm/transactionCount/"):
			writeCBORResponse(w, &struct {
				_     struct{} `cbor:",toarray"`
				Nonce uint64
			}{
				Nonce: br.nonce,
			}, http.StatusOK, log)
		case strings.Contains(r.URL.Path, "/api/v1/evm/call"):
			br.callReq = &evm.CallEVMRequest{}
			if err := types.Cbor.Decode(r.Body, br.callReq); err != nil {
				writeCBORError(w, fmt.Errorf("unable to decode request body: %w", err), http.StatusBadRequest, log)
				return
			}
			writeCBORResponse(w, br.callResp, http.StatusOK, log)
		case strings.Contains(r.URL.Path, "/api/v1/evm/gasPrice"):
			writeCBORResponse(w, &struct {
				_        struct{} `cbor:",toarray"`
				GasPrice string
			}{
				GasPrice: br.gasPrice,
			}, http.StatusOK, log)
		case strings.Contains(r.URL.Path, "/api/v1/rounds/latest"):
			writeCBORResponse(w, br.round, http.StatusOK, log)
		case strings.Contains(r.URL.Path, "/api/v1/transactions"):
			if r.Method == "POST" {
				buf := new(bytes.Buffer)
				if _, err := buf.ReadFrom(r.Body); err != nil {
					writeCBORError(w, fmt.Errorf("reading request body failed: %w", err), http.StatusBadRequest, log)
					return
				}
				tx := &types.TransactionOrder{}
				if err := types.Cbor.Unmarshal(buf.Bytes(), tx); err != nil {
					writeCBORError(w, fmt.Errorf("unable to decode request body as transaction: %w", err), http.StatusBadRequest, log)
					return
				}
				br.receivedTx = tx
				writeCBORResponse(w, tx.Hash(crypto.SHA256), http.StatusAccepted, log)
				return
			}
			// GET
			writeCBORResponse(w, struct {
				_        struct{} `cbor:",toarray"`
				TxRecord *types.TransactionRecord
				TxProof  *types.TxProof
			}{
				TxRecord: &types.TransactionRecord{
					TransactionOrder: br.receivedTx,
					ServerMetadata:   br.serverMeta,
				},
				TxProof: &types.TxProof{},
			}, http.StatusOK, log)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}

func execEvmCmd(obs *testobserve.Observability, homeDir, command string) (*testutils.TestConsoleWriter, error) {
	outputWriter := &testutils.TestConsoleWriter{}
	command = strings.TrimPrefix(command, "evm ")
	ccmd := NewEvmCmd(&cmdtypes.WalletConfig{
		Base:          &cmdtypes.BaseConfiguration{HomeDir: homeDir, ConsoleWriter: outputWriter, Observe: obs},
		WalletHomeDir: filepath.Join(homeDir, "wallet")})
	ccmd.SetArgs(strings.Split(command, " "))
	return outputWriter, ccmd.Execute()
}

// WriteCBORResponse replies to the request with the given response and HTTP code.
func writeCBORResponse(w http.ResponseWriter, response any, statusCode int, log *slog.Logger) {
	w.Header().Set("Content-Type", "application/cbor")
	w.WriteHeader(statusCode)
	if err := types.Cbor.Encode(w, response); err != nil {
		log.Warn("failed to write CBOR response", logger.Error(err))
	}
}

func writeCBORError(w http.ResponseWriter, e error, code int, log *slog.Logger) {
	w.Header().Set("Content-Type", "application/cbor")
	w.WriteHeader(code)
	if err := types.Cbor.Encode(w, struct {
		_   struct{} `cbor:",toarray"`
		Err string
	}{
		Err: fmt.Sprintf("%v", e),
	}); err != nil {
		log.Warn("failed to write CBOR error response", logger.Error(err))
	}
}
