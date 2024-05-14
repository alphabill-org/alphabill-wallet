package client

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/evm"
	"github.com/alphabill-org/alphabill-go-base/types"

	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"

	test "github.com/alphabill-org/alphabill-wallet/internal/testutils"
)

// writeCBORResponse replies to the request with the given response and HTTP code.
func writeCBORResponse(t *testing.T, w http.ResponseWriter, response any, statusCode int) {
	w.Header().Set("Content-Type", "application/cbor")
	w.WriteHeader(statusCode)
	if err := types.Cbor.Encode(w, response); err != nil {
		t.Errorf("Failed to write response body, CBOR error: %v", err)
	}
}

// writeCBORError replies to the request with the specified error message and HTTP code.
// It does not otherwise end the request; the caller should ensure no further
// writes are done to w.
func writeCBORError(t *testing.T, w http.ResponseWriter, e error, code int) {
	w.Header().Set("Content-Type", "application/cbor")
	w.WriteHeader(code)
	if err := types.Cbor.Encode(w, struct {
		_   struct{} `cbor:",toarray"`
		Err string
	}{
		Err: fmt.Sprintf("%v", e),
	}); err != nil {
		t.Errorf("Failed to write response body, CBOR error: %v", err)
	}
}

func createTxOrder(t *testing.T) *types.TransactionOrder {
	transaction := testtransaction.NewTransactionOrder(t,
		testtransaction.WithAttributes([]byte{0, 0, 0, 0, 0, 0, 0}),
		testtransaction.WithUnitID([]byte{0, 0, 0, 1}),
		testtransaction.WithSystemID(1),
		testtransaction.WithOwnerProof([]byte{0, 0, 0, 2}),
		testtransaction.WithClientMetadata(&types.ClientMetadata{Timeout: 100}),
		testtransaction.WithPayloadType("test"),
	)
	return transaction
}

func TestEvmClient_GetBalance(t *testing.T) {
	t.Parallel()
	addr := test.RandomBytes(20)

	t.Run("valid response", func(t *testing.T) {
		cli := &EvmClient{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					if r.URL.String() != `http://localhost/api/v1/evm/balance/`+hex.EncodeToString(addr) {
						t.Errorf("unexpected request URL: %s", r.URL.String())
					}
					if ua := r.Header.Get(userAgentHeader); ua != clientUserAgent {
						t.Errorf("expected User-Agent header %q, got %q", clientUserAgent, ua)
					}
					w := httptest.NewRecorder()
					response := struct {
						_       struct{} `cbor:",toarray"`
						Balance string
						Counter uint64
					}{
						Balance: "13000000",
						Counter: 0,
					}
					writeCBORResponse(t, w, response, http.StatusOK)
					return w.Result(), nil
				},
			}},
		}
		amount, counter, err := cli.GetBalance(context.Background(), addr)
		require.NoError(t, err)
		require.EqualValues(t, "13000000", amount)
		require.EqualValues(t, 0, counter)
	})
	t.Run("not found", func(t *testing.T) {
		cli := &EvmClient{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					w := httptest.NewRecorder()
					response := struct {
						_       struct{} `cbor:",toarray"`
						Balance string
						Counter uint64
					}{
						Balance: "130000001",
						Counter: 12345,
					}
					writeCBORResponse(t, w, response, http.StatusOK)
					return w.Result(), nil
				},
			}},
		}
		amount, counter, err := cli.GetBalance(context.Background(), addr)
		require.NoError(t, err)
		require.EqualValues(t, "130000001", amount)
		require.EqualValues(t, counter, 12345)
	})
	t.Run("not found", func(t *testing.T) {
		cli := &EvmClient{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					w := httptest.NewRecorder()
					writeCBORError(t, w, errors.New("address not found"), http.StatusNotFound)
					return w.Result(), nil
				},
			}},
		}
		amount, counter, err := cli.GetBalance(context.Background(), addr)
		require.ErrorIs(t, err, ErrNotFound)
		require.EqualValues(t, "", amount)
		require.EqualValues(t, 0, counter)
	})
}

func TestEvmClient_GetFeeCreditBill(t *testing.T) {
	t.Parallel()
	addr := test.RandomBytes(20)

	t.Run("valid", func(t *testing.T) {
		cli := &EvmClient{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					if r.URL.String() != `http://localhost/api/v1/evm/balance/`+hex.EncodeToString(addr) {
						t.Errorf("unexpected request URL: %s", r.URL.String())
					}
					if ua := r.Header.Get(userAgentHeader); ua != clientUserAgent {
						t.Errorf("expected User-Agent header %q, got %q", clientUserAgent, ua)
					}
					w := httptest.NewRecorder()
					response := struct {
						_       struct{} `cbor:",toarray"`
						Balance string
						Counter uint64
					}{
						Balance: "1300000000000000",
						Counter: 12345,
					}
					writeCBORResponse(t, w, response, http.StatusOK)
					return w.Result(), nil
				},
			}},
		}
		fcrBill, err := cli.GetFeeCreditBill(context.Background(), addr)
		require.NoError(t, err)
		require.EqualValues(t, addr, fcrBill.Id)
		value := new(big.Int)
		value.SetString("1300000000000000", 10)
		require.EqualValues(t, WeiToAlpha(value), fcrBill.Value)
		require.EqualValues(t, 12345, fcrBill.Counter)
	})
}

func TestEvmClient_GetTransactionCount(t *testing.T) {
	t.Parallel()
	addr := test.RandomBytes(20)

	t.Run("valid response", func(t *testing.T) {
		cli := &EvmClient{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					if r.URL.String() != `http://localhost/api/v1/evm/transactionCount/`+hex.EncodeToString(addr) {
						t.Errorf("unexpected request URL: %s", r.URL.String())
					}
					if ua := r.Header.Get(userAgentHeader); ua != clientUserAgent {
						t.Errorf("expected User-Agent header %q, got %q", clientUserAgent, ua)
					}
					w := httptest.NewRecorder()
					response := struct {
						_     struct{} `cbor:",toarray"`
						Nonce uint64
					}{Nonce: 3}
					writeCBORResponse(t, w, response, http.StatusOK)
					return w.Result(), nil
				},
			}},
		}
		nonce, err := cli.GetTransactionCount(context.Background(), addr)
		require.NoError(t, err)
		require.EqualValues(t, 3, nonce)
	})
	t.Run("not found", func(t *testing.T) {
		cli := &EvmClient{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					w := httptest.NewRecorder()
					writeCBORError(t, w, errors.New("address not found"), http.StatusNotFound)
					return w.Result(), nil
				},
			}},
		}
		nonce, err := cli.GetTransactionCount(context.Background(), addr)
		require.ErrorIs(t, err, ErrNotFound)
		require.Zero(t, nonce)
	})
}

func TestEvmClient_Call(t *testing.T) {
	t.Parallel()

	t.Run("valid request and response", func(t *testing.T) {
		cli := &EvmClient{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					if r.URL.String() != `http://localhost/api/v1/evm/call` {
						t.Errorf("unexpected request URL: %s", r.URL.String())
					}
					if ua := r.Header.Get(userAgentHeader); ua != clientUserAgent {
						t.Errorf("expected User-Agent header %q, got %q", clientUserAgent, ua)
					}
					defer func() { require.NoError(t, r.Body.Close()) }()
					buf, err := io.ReadAll(r.Body)
					if err != nil {
						return nil, fmt.Errorf("failed to read request body: %w", err)
					}
					require.NotEmpty(t, buf)
					w := httptest.NewRecorder()
					callEVMResponse := &struct {
						_                 struct{} `cbor:",toarray"`
						ProcessingDetails *evm.ProcessingDetails
					}{
						ProcessingDetails: &evm.ProcessingDetails{ErrorDetails: "some error occurred"},
					}
					writeCBORResponse(t, w, callEVMResponse, http.StatusOK)
					return w.Result(), nil
				},
			}},
		}

		attr := &evm.CallEVMRequest{}
		result, err := cli.Call(context.Background(), attr)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "some error occurred", result.ErrorDetails)
	})

	t.Run("error is returned", func(t *testing.T) {
		cli := &EvmClient{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					w := httptest.NewRecorder()
					writeCBORError(t, w, errors.New("not a valid transaction"), http.StatusBadRequest)
					return w.Result(), nil
				},
			}},
		}
		attr := &evm.CallEVMRequest{}
		result, err := cli.Call(context.Background(), attr)
		require.ErrorContains(t, err, "transaction send failed: 400 Bad Request, not a valid transaction")
		require.Nil(t, result)
	})
}

func TestEvmClient_GetRoundNumber(t *testing.T) {
	t.Parallel()
	t.Run("valid request is built", func(t *testing.T) {
		cli := &EvmClient{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					if r.URL.String() != `http://localhost/api/v1/rounds/latest` {
						t.Errorf("unexpected request URL: %s", r.URL.String())
					}
					if ua := r.Header.Get(userAgentHeader); ua != clientUserAgent {
						t.Errorf("expected User-Agent header %q, got %q", clientUserAgent, ua)
					}
					w := httptest.NewRecorder()
					round := uint64(0)
					writeCBORResponse(t, w, round, http.StatusOK)
					return w.Result(), nil
				},
			}},
		}
		rnr, err := cli.GetRoundNumber(context.Background())
		require.NoError(t, err)
		require.NotNil(t, rnr)
	})

	createClient := func(t *testing.T, data any) *EvmClient {
		t.Helper()
		return &EvmClient{
			hc: &http.Client{
				Transport: &mockRoundTripper{
					do: func(r *http.Request) (*http.Response, error) {
						w := httptest.NewRecorder()
						writeCBORResponse(t, w, data, http.StatusOK)
						return w.Result(), nil
					},
				},
			},
		}
	}

	t.Run("rpc node returns empty response body", func(t *testing.T) {
		cli := createClient(t, ``)
		rn, err := cli.GetRoundNumber(context.Background())
		require.EqualError(t, err, `get round-number request failed: failed to decode response body: cbor: cannot unmarshal UTF-8 text string into Go value of type uint64`)
		require.Zero(t, rn)
	})

	t.Run("success", func(t *testing.T) {
		round := uint64(3)
		cli := createClient(t, round)
		rnr, err := cli.GetRoundNumber(context.Background())
		require.NoError(t, err)
		require.EqualValues(t, 3, rnr.RoundNumber)
	})
}

func TestEvmClient_GetTxProof(t *testing.T) {
	t.Parallel()
	txHash := test.RandomBytes(32)

	t.Run("valid request is built", func(t *testing.T) {
		cli := &EvmClient{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					if r.URL.String() != `http://localhost/api/v1/transactions/`+hex.EncodeToString(txHash) {
						t.Errorf("unexpected request URL: %s", r.URL.String())
					}
					if ua := r.Header.Get(userAgentHeader); ua != clientUserAgent {
						t.Errorf("expected User-Agent header %q, got %q", clientUserAgent, ua)
					}
					w := httptest.NewRecorder()
					response := struct {
						_        struct{} `cbor:",toarray"`
						TxRecord *types.TransactionRecord
						TxProof  *types.TxProof
					}{}
					writeCBORResponse(t, w, response, http.StatusOK)
					return w.Result(), nil
				},
			}},
		}
		proof, err := cli.GetTxProof(context.Background(), []byte{}, txHash)
		require.NoError(t, err)
		require.NotNil(t, proof)
	})

	t.Run("not found", func(t *testing.T) {
		cli := &EvmClient{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					if r.URL.String() != `http://localhost/api/v1/transactions/`+hex.EncodeToString(txHash) {
						t.Errorf("unexpected request URL: %s", r.URL.String())
					}
					w := httptest.NewRecorder()
					writeCBORError(t, w, errors.New("not found"), http.StatusNotFound)
					return w.Result(), nil
				},
			}},
		}
		proof, err := cli.GetTxProof(context.Background(), []byte{}, txHash)
		require.NoError(t, err)
		require.Nil(t, proof)
	})

	t.Run("internal error", func(t *testing.T) {
		cli := &EvmClient{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					if r.URL.String() != `http://localhost/api/v1/transactions/`+hex.EncodeToString(txHash) {
						t.Errorf("unexpected request URL: %s", r.URL.String())
					}
					w := httptest.NewRecorder()
					writeCBORError(t, w, fmt.Errorf("some error"), http.StatusInternalServerError)
					return w.Result(), nil
				},
			}},
		}
		proof, err := cli.GetTxProof(context.Background(), []byte{}, txHash)
		require.ErrorContains(t, err, "get tx proof request failed: 500 Internal Server Error, some error")
		require.Nil(t, proof)
	})
}

func TestEvmClient_PostTransaction(t *testing.T) {
	t.Parallel()

	t.Run("valid request is built", func(t *testing.T) {
		cli := &EvmClient{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					if r.URL.String() != `http://localhost/api/v1/transactions` {
						t.Errorf("unexpected request URL: %s", r.URL.String())
					}
					if ua := r.Header.Get(userAgentHeader); ua != clientUserAgent {
						t.Errorf("expected User-Agent header %q, got %q", clientUserAgent, ua)
					}

					defer func() { require.NoError(t, r.Body.Close()) }()
					buf, err := io.ReadAll(r.Body)
					if err != nil {
						return nil, fmt.Errorf("failed to read request body: %w", err)
					}
					require.NotEmpty(t, buf)
					w := httptest.NewRecorder()
					w.WriteHeader(http.StatusAccepted)
					return w.Result(), nil
				},
			}},
		}

		tx := createTxOrder(t)
		err := cli.PostTransaction(context.Background(), tx)
		require.NoError(t, err)
	})

	t.Run("invalid request", func(t *testing.T) {
		cli := &EvmClient{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					w := httptest.NewRecorder()
					writeCBORError(t, w, fmt.Errorf("test error"), http.StatusBadRequest)
					return w.Result(), nil
				},
			}},
		}
		err := cli.PostTransaction(context.Background(), &types.TransactionOrder{})
		require.EqualError(t, err, "transaction send failed: 400 Bad Request, test error")
	})

	t.Run("success", func(t *testing.T) {
		cli := &EvmClient{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					w := httptest.NewRecorder()
					w.WriteHeader(http.StatusAccepted)
					return w.Result(), nil
				},
			}},
		}

		tx := createTxOrder(t)
		err := cli.PostTransaction(context.Background(), tx)
		require.NoError(t, err)
	})
}

func TestEvmClient_GetGasPrice(t *testing.T) {
	t.Parallel()

	t.Run("valid response", func(t *testing.T) {
		cli := &EvmClient{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					if r.URL.String() != "http://localhost/api/v1/evm/gasPrice" {
						t.Errorf("unexpected request URL: %s", r.URL.String())
					}
					if ua := r.Header.Get(userAgentHeader); ua != clientUserAgent {
						t.Errorf("expected User-Agent header %q, got %q", clientUserAgent, ua)
					}
					w := httptest.NewRecorder()
					response := struct {
						_        struct{} `cbor:",toarray"`
						GasPrice string
					}{
						GasPrice: "13000000",
					}
					writeCBORResponse(t, w, response, http.StatusOK)
					return w.Result(), nil
				},
			}},
		}
		price, err := cli.GetGasPrice(context.Background())
		require.NoError(t, err)
		require.EqualValues(t, "13000000", price)
	})
	t.Run("not found", func(t *testing.T) {
		cli := &EvmClient{
			addr: url.URL{Scheme: "http", Host: "localhost"},
			hc: &http.Client{Transport: &mockRoundTripper{
				do: func(r *http.Request) (*http.Response, error) {
					w := httptest.NewRecorder()
					writeCBORError(t, w, errors.New("address not found"), http.StatusNotFound)
					return w.Result(), nil
				},
			}},
		}
		price, err := cli.GetGasPrice(context.Background())
		require.ErrorIs(t, err, ErrNotFound)
		require.EqualValues(t, "", price)
	})
}

type mockRoundTripper struct {
	do func(*http.Request) (*http.Response, error)
}

func (mrt *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return mrt.do(req)
}
