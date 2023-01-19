package pubkey_indexer

import (
	"context"
	"crypto"
	"fmt"
	"net/http"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testhttp "github.com/alphabill-org/alphabill/internal/testutils/http"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

const (
	pubkeyHex = "0x000000000000000000000000000000000000000000000000000000000000000000"
)

type (
	mockWalletService struct {
		store *BoltBillStore
	}
	option func(service *mockWalletService) error
)

func newMockWalletService(t *testing.T, options ...option) *mockWalletService {
	store, _ := createTestBillStore(t)
	service := &mockWalletService{
		store: store,
	}
	for _, o := range options {
		_ = o(service)
	}
	return service
}

func withBills(pubkey []byte, bill ...*Bill) option {
	return func(s *mockWalletService) error {
		_ = s.AddKey(pubkey)
		_ = s.addBills(pubkey, bill...)
		return nil
	}
}

func (m *mockWalletService) GetBills(pubKey []byte) ([]*Bill, error) {
	return m.store.GetBills(pubKey)
}

func (m *mockWalletService) GetBill(pubkey []byte, unitID []byte) (*Bill, error) {
	return m.store.GetBill(pubkey, unitID)
}

func (m *mockWalletService) SetBills(pubkey []byte, bills *block.Bills) error {
	domainBills := newBillsFromProto(bills)
	return m.store.SetBills(pubkey, domainBills...)
}

func (m *mockWalletService) AddKey(pubkey []byte) error {
	return m.store.AddKey(&Pubkey{
		Pubkey:     pubkey,
		PubkeyHash: wallet.NewKeyHash(pubkey),
	})
}

func (m *mockWalletService) addBills(pubkey []byte, bill ...*Bill) error {
	return m.store.SetBills(pubkey, bill...)
}

func (m *mockWalletService) getKeys() ([]*Pubkey, error) {
	return m.store.GetKeys()
}

func (m *mockWalletService) GetMaxBlockNumber() (uint64, error) {
	return m.store.GetBlockNumber()
}

func TestListBillsRequest_Ok(t *testing.T) {
	expectedBill := &Bill{
		Id:    newUnitID(1),
		Value: 1,
	}
	pubkey, _ := hexutil.Decode(pubkeyHex)
	mockService := newMockWalletService(t, withBills(pubkey, expectedBill))
	startServer(t, mockService)

	res := &ListBillsResponse{}
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s", pubkeyHex), res)

	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Len(t, res.Bills, 1)
	expectedRes := toBillVMList([]*Bill{expectedBill})
	require.Equal(t, expectedRes, res.Bills)
}

func TestListBillsRequest_NilPubKey(t *testing.T) {
	startServer(t, newMockWalletService(t))

	res := &ErrorResponse{}
	httpRes := testhttp.DoGet(t, "http://localhost:7777/api/v1/list-bills", res)

	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, "missing required pubkey query parameter", res.Message)
}

func TestListBillsRequest_InvalidPubKey(t *testing.T) {
	startServer(t, newMockWalletService(t))

	res := &ErrorResponse{}
	pk := "0x00"
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s", pk), res)

	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, "pubkey hex string must be 68 characters long (with 0x prefix)", res.Message)
}

func TestListBillsRequest_PubKeyNotIndexed(t *testing.T) {
	startServer(t, newMockWalletService(t))

	res := &ErrorResponse{}
	pk := "0x000000000000000000000000000000000000000000000000000000000000000000"
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s", pk), res)

	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.ErrorContains(t, ErrPubKeyNotIndexed, res.Message)
}

func TestListBillsRequest_SortedByInsertionOrder(t *testing.T) {
	pubkey, _ := hexutil.Decode(pubkeyHex)
	mockService := newMockWalletService(t, withBills(pubkey,
		&Bill{
			Id:    newUnitID(2),
			Value: 2,
		},
		&Bill{
			Id:    newUnitID(1),
			Value: 1,
		},
	))
	startServer(t, mockService)

	res := &ListBillsResponse{}
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s", pubkeyHex), res)

	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, 2, res.Total)
	require.Len(t, res.Bills, 2)
	require.EqualValues(t, res.Bills[0].Value, 2)
	require.EqualValues(t, res.Bills[1].Value, 1)
}

func TestListBillsRequest_DCBillsIncluded(t *testing.T) {
	pubkey, _ := hexutil.Decode(pubkeyHex)
	mockService := newMockWalletService(t, withBills(pubkey,
		&Bill{
			Id:    newUnitID(1),
			Value: 1,
		},
		&Bill{
			Id:       newUnitID(2),
			Value:    2,
			IsDCBill: true,
		},
	))
	startServer(t, mockService)

	res := &ListBillsResponse{}
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s", pubkeyHex), res)

	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, 2, res.Total)
	require.Len(t, res.Bills, 2)
	bill := res.Bills[0]
	require.EqualValues(t, bill.Value, 1)
	require.False(t, bill.IsDCBill)
	bill = res.Bills[1]
	require.EqualValues(t, res.Bills[1].Value, 2)
	require.True(t, res.Bills[1].IsDCBill)
}

func TestListBillsRequest_Paging(t *testing.T) {
	// given set of bills
	var bills []*Bill
	for i := uint64(0); i < 200; i++ {
		bills = append(bills, &Bill{
			Id:          newUnitID(i),
			Value:       i,
			OrderNumber: i,
		})
	}
	pubkey, _ := hexutil.Decode(pubkeyHex)
	mockService := newMockWalletService(t, withBills(pubkey, bills...))
	startServer(t, mockService)

	// verify by default first 100 elements are returned
	res := &ListBillsResponse{}
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s", pubkeyHex), res)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 100)
	require.EqualValues(t, res.Bills[0].Value, 0)
	require.EqualValues(t, res.Bills[99].Value, 99)

	// verify offset=100 returns next 100 elements
	res = &ListBillsResponse{}
	httpRes = testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s&offset=100", pubkeyHex), res)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 100)
	require.EqualValues(t, res.Bills[0].Value, 100)
	require.EqualValues(t, res.Bills[99].Value, 199)

	// verify limit limits result size
	res = &ListBillsResponse{}
	httpRes = testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s&offset=100&limit=50", pubkeyHex), res)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 50)
	require.EqualValues(t, res.Bills[0].Value, 100)
	require.EqualValues(t, res.Bills[49].Value, 149)

	// verify out of bounds offset returns nothing
	res = &ListBillsResponse{}
	httpRes = testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s&offset=200", pubkeyHex), res)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 0)

	// verify limit gets capped to 100
	res = &ListBillsResponse{}
	httpRes = testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s&offset=0&limit=200", pubkeyHex), res)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 100)
	require.EqualValues(t, res.Bills[0].Value, 0)
	require.EqualValues(t, res.Bills[99].Value, 99)

	// verify out of bounds offset+limit return all available data
	res = &ListBillsResponse{}
	httpRes = testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/list-bills?pubkey=%s&offset=190&limit=100", pubkeyHex), res)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Equal(t, len(bills), res.Total)
	require.Len(t, res.Bills, 10)
	require.EqualValues(t, res.Bills[0].Value, 190)
	require.EqualValues(t, res.Bills[9].Value, 199)
}

func TestBalanceRequest_Ok(t *testing.T) {
	pubkey, _ := hexutil.Decode(pubkeyHex)
	startServer(t, newMockWalletService(t, withBills(pubkey, &Bill{
		Id:    newUnitID(1),
		Value: 1,
	})))

	res := &BalanceResponse{}
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/balance?pubkey=%s", pubkeyHex), res)

	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.EqualValues(t, 1, res.Balance)
}

func TestBalanceRequest_NilPubKey(t *testing.T) {
	startServer(t, newMockWalletService(t))

	res := &ErrorResponse{}
	httpRes := testhttp.DoGet(t, "http://localhost:7777/api/v1/balance", res)

	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, "missing required pubkey query parameter", res.Message)
}

func TestBalanceRequest_InvalidPubKey(t *testing.T) {
	startServer(t, newMockWalletService(t))

	res := &ErrorResponse{}
	pk := "0x00"
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/balance?pubkey=%s", pk), res)

	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, "pubkey hex string must be 68 characters long (with 0x prefix)", res.Message)
}

func TestBalanceRequest_PubKeyNotIndexed(t *testing.T) {
	startServer(t, newMockWalletService(t))

	res := &ErrorResponse{}
	pk := "0x000000000000000000000000000000000000000000000000000000000000000000"
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/balance?pubkey=%s", pk), res)

	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.ErrorContains(t, ErrPubKeyNotIndexed, res.Message)
}

func TestBalanceRequest_DCBillNotIncluded(t *testing.T) {
	pubkey, _ := hexutil.Decode(pubkeyHex)
	mockService := newMockWalletService(t, withBills(pubkey,
		&Bill{
			Id:    newUnitID(1),
			Value: 1,
		},
		&Bill{
			Id:       newUnitID(2),
			Value:    2,
			IsDCBill: true,
		}),
	)
	startServer(t, mockService)

	res := &BalanceResponse{}
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/balance?pubkey=%s", pubkeyHex), res)

	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.EqualValues(t, 1, res.Balance)
}

func TestProofRequest_Ok(t *testing.T) {
	billId := "0x0000000000000000000000000000000000000000000000000000000000000001"
	pubkey, _ := hexutil.Decode(pubkeyHex)
	b := &Bill{
		Id:     newUnitID(1),
		Value:  1,
		TxHash: []byte{0},
		TxProof: &TxProof{
			BlockNumber: 1,
			Tx:          testtransaction.NewTransaction(t),
			Proof: &block.BlockProof{
				BlockHeaderHash: []byte{0},
				BlockTreeHashChain: &block.BlockTreeHashChain{
					Items: []*block.ChainItem{{Val: []byte{0}, Hash: []byte{0}}},
				},
			},
		},
	}
	mockService := newMockWalletService(t, withBills(pubkey, b))
	startServer(t, mockService)

	response := &block.Bills{}
	httpRes := testhttp.DoGetProto(t, fmt.Sprintf("http://localhost:7777/api/v1/proof/%s?bill_id=%s", pubkeyHex, billId), response)

	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.Len(t, response.Bills, 1)
	res := response.Bills[0]
	require.Equal(t, b.Id, res.Id)
	require.Equal(t, b.Value, res.Value)
	require.Equal(t, b.TxHash, res.TxHash)

	ep := b.TxProof
	ap := res.TxProof
	require.Equal(t, ep.BlockNumber, ap.BlockNumber)
	require.EqualValues(t, ep.Tx.UnitId, ap.Tx.UnitId)
	require.EqualValues(t, ep.Proof.BlockHeaderHash, ap.Proof.BlockHeaderHash)
}

func TestProofRequest_MissingBillId(t *testing.T) {
	startServer(t, newMockWalletService(t))

	res := &ErrorResponse{}
	httpRes := testhttp.DoGet(t, "http://localhost:7777/api/v1/proof/"+pubkeyHex, res)

	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, "missing required bill_id query parameter", res.Message)
}

func TestProofRequest_InvalidBillIdLength(t *testing.T) {
	startServer(t, newMockWalletService(t))

	// verify bill id larger than 32 bytes returns error
	res := &ErrorResponse{}
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/proof/%s?bill_id=0x000000000000000000000000000000000000000000000000000000000000000001", pubkeyHex), res)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, errInvalidBillIDLength.Error(), res.Message)

	// verify bill id smaller than 32 bytes returns error
	res = &ErrorResponse{}
	httpRes = testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/proof/%s?bill_id=0x01", pubkeyHex), res)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, errInvalidBillIDLength.Error(), res.Message)

	// verify bill id with correct length but missing prefix returns error
	res = &ErrorResponse{}
	httpRes = testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/proof/%s?bill_id=0000000000000000000000000000000000000000000000000000000000000001", pubkeyHex), res)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, errInvalidBillIDLength.Error(), res.Message)
}

func TestProofRequest_PubKeyNotIndexed(t *testing.T) {
	startServer(t, newMockWalletService(t))

	res := &ErrorResponse{}
	billId := "0x0000000000000000000000000000000000000000000000000000000000000001"
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/proof/%s?bill_id=%s", pubkeyHex, billId), res)

	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, "pubkey not indexed", res.Message)
}

func TestProofRequest_ProofDoesNotExist(t *testing.T) {
	pubkey, _ := hexutil.Decode(pubkeyHex)
	startServer(t, newMockWalletService(t, withBills(pubkey, &Bill{})))

	res := &ErrorResponse{}
	billId := "0x0000000000000000000000000000000000000000000000000000000000000001"
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://localhost:7777/api/v1/proof/%s?bill_id=%s", pubkeyHex, billId), res)

	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, "bill does not exist", res.Message)
}

func TestAddProofRequest_Ok(t *testing.T) {
	_ = log.InitStdoutLogger(log.INFO)
	pubkey := make([]byte, 33)
	txValue := uint64(100)
	tx := testtransaction.NewTransaction(t, testtransaction.WithAttributes(&moneytx.TransferOrder{
		TargetValue: txValue,
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubkey)),
	}))
	txConverter := backend.NewTxConverter(moneySystemID)
	gtx, _ := txConverter.ConvertTx(tx)
	txHash := gtx.Hash(crypto.SHA256)
	proof, verifiers := createProofForTx(t, tx)
	store, _ := createTestBillStore(t)
	service := New(nil, store, txConverter, verifiers)
	_ = service.AddKey(pubkey)
	startServer(t, service)

	req := &block.Bills{
		Bills: []*block.Bill{
			{
				Id:     tx.UnitId,
				Value:  txValue,
				TxHash: txHash,
				TxProof: &block.TxProof{
					BlockNumber: 1,
					Tx:          tx,
					Proof:       proof,
				},
			},
		},
	}
	res := &EmptyResponse{}
	pubkeyHex := hexutil.Encode(pubkey)
	httpRes := testhttp.DoPostProto(t, "http://localhost:7777/api/v1/proof/"+pubkeyHex, req, res)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)

	bills, err := service.GetBills(pubkey)
	require.NoError(t, err)
	require.Len(t, bills, 1)
	b := bills[0]
	require.Equal(t, tx.UnitId, b.Id)
	require.Equal(t, txHash, b.TxHash)
	require.EqualValues(t, txValue, b.Value)
	txProof := b.TxProof
	require.NotNil(t, txProof)
	require.EqualValues(t, 1, txProof.BlockNumber)
	require.True(t, proto.Equal(tx, txProof.Tx))
	require.NotNil(t, proof, txProof.Proof)
}

func TestAddProofRequest_UnindexedKey_NOK(t *testing.T) {
	_ = log.InitStdoutLogger(log.INFO)
	txValue := uint64(100)
	tx := testtransaction.NewTransaction(t, testtransaction.WithAttributes(&moneytx.TransferOrder{
		TargetValue: txValue,
	}))
	txConverter := backend.NewTxConverter(moneySystemID)
	gtx, _ := txConverter.ConvertTx(tx)
	txHash := gtx.Hash(crypto.SHA256)
	proof, verifiers := createProofForTx(t, tx)

	store, _ := createTestBillStore(t)
	service := New(nil, store, txConverter, verifiers)
	startServer(t, service)

	pubkey := make([]byte, 33)
	req := &block.Bills{
		Bills: []*block.Bill{
			{
				Id:     tx.UnitId,
				Value:  txValue,
				TxHash: txHash,
				TxProof: &block.TxProof{
					BlockNumber: 1,
					Tx:          tx,
					Proof:       proof,
				},
			},
		},
	}
	res := &ErrorResponse{}
	pubkeyHex := hexutil.Encode(pubkey)
	httpRes := testhttp.DoPostProto(t, "http://localhost:7777/api/v1/proof/"+pubkeyHex, req, res)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, errKeyNotIndexed.Error(), res.Message)
}

func TestAddProofRequest_InvalidPredicate_NOK(t *testing.T) {
	_ = log.InitStdoutLogger(log.INFO)
	txValue := uint64(100)
	tx := testtransaction.NewTransaction(t, testtransaction.WithAttributes(&moneytx.TransferOrder{
		TargetValue: txValue,
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256([]byte("invalid pub key"))),
	}))
	txConverter := backend.NewTxConverter(moneySystemID)
	gtx, _ := txConverter.ConvertTx(tx)
	txHash := gtx.Hash(crypto.SHA256)
	proof, verifiers := createProofForTx(t, tx)

	pubkey := make([]byte, 33)
	store, _ := createTestBillStore(t)
	service := New(nil, store, txConverter, verifiers)
	_ = service.AddKey(pubkey)
	startServer(t, service)

	req := &block.Bills{
		Bills: []*block.Bill{
			{
				Id:     tx.UnitId,
				Value:  txValue,
				TxHash: txHash,
				TxProof: &block.TxProof{
					BlockNumber: 1,
					Tx:          tx,
					Proof:       proof,
				},
			},
		},
	}
	res := &ErrorResponse{}
	pubkeyHex := hexutil.Encode(pubkey)
	httpRes := testhttp.DoPostProto(t, "http://localhost:7777/api/v1/proof/"+pubkeyHex, req, res)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, "p2pkh predicate verification failed: invalid bearer predicate", res.Message)
}

func TestAddDCBillProofRequest_Ok(t *testing.T) {
	_ = log.InitStdoutLogger(log.INFO)
	pubkey := make([]byte, 33)
	txValue := uint64(100)
	tx := testtransaction.NewTransaction(t, testtransaction.WithAttributes(&moneytx.TransferDCOrder{
		TargetValue:  txValue,
		TargetBearer: script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubkey)),
	}))
	txConverter := backend.NewTxConverter(moneySystemID)
	gtx, _ := txConverter.ConvertTx(tx)
	txHash := gtx.Hash(crypto.SHA256)
	proof, verifiers := createProofForTx(t, tx)
	store, _ := createTestBillStore(t)
	service := New(nil, store, txConverter, verifiers)
	_ = service.AddKey(pubkey)
	startServer(t, service)

	req := &block.Bills{
		Bills: []*block.Bill{
			{
				Id:       tx.UnitId,
				Value:    txValue,
				TxHash:   txHash,
				IsDcBill: true,
				TxProof: &block.TxProof{
					BlockNumber: 1,
					Tx:          tx,
					Proof:       proof,
				},
			},
		},
	}
	res := &EmptyResponse{}
	pubkeyHex := hexutil.Encode(pubkey)
	httpRes := testhttp.DoPostProto(t, "http://localhost:7777/api/v1/proof/"+pubkeyHex, req, res)
	require.Equal(t, http.StatusOK, httpRes.StatusCode)

	bills, err := service.GetBills(pubkey)
	require.NoError(t, err)
	require.Len(t, bills, 1)
	b := bills[0]
	require.Equal(t, tx.UnitId, b.Id)
	require.Equal(t, txHash, b.TxHash)
	require.EqualValues(t, txValue, b.Value)
	require.True(t, b.IsDCBill)

	txProof := b.TxProof
	require.NotNil(t, txProof)
	require.EqualValues(t, 1, txProof.BlockNumber)
	require.True(t, proto.Equal(tx, txProof.Tx))
	require.NotNil(t, proof, txProof.Proof)
}

func createProofForTx(t *testing.T, tx *txsystem.Transaction) (*block.BlockProof, map[string]abcrypto.Verifier) {
	b := &block.Block{
		SystemIdentifier:  moneySystemID,
		BlockNumber:       1,
		PreviousBlockHash: hash.Sum256([]byte{}),
		Transactions:      []*txsystem.Transaction{tx},
	}
	txConverter := backend.NewTxConverter(moneySystemID)
	b, verifiers := testblock.CertifyBlock(t, b, txConverter)
	genericBlock, _ := b.ToGenericBlock(txConverter)
	proof, _ := block.NewPrimaryProof(genericBlock, tx.UnitId, crypto.SHA256)
	return proof, verifiers
}

func TestAddKeyRequest_Ok(t *testing.T) {
	mockService := newMockWalletService(t)
	startServer(t, mockService)

	req := &AddKeyRequest{Pubkey: "0x000000000000000000000000000000000000000000000000000000000000000000"}
	res := &EmptyResponse{}
	httpRes := testhttp.DoPost(t, "http://localhost:7777/api/v1/admin/add-key", req, res)

	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	keys, _ := mockService.getKeys()
	require.Len(t, keys, 1)
	pubkeyBytes, _ := hexutil.Decode(req.Pubkey)
	require.Equal(t, keys[0].Pubkey, pubkeyBytes)
}

func TestAddKeyRequest_KeyAlreadyExists(t *testing.T) {
	pubkey := "0x000000000000000000000000000000000000000000000000000000000000000000"
	pubkeyBytes, _ := hexutil.Decode(pubkey)
	mockService := newMockWalletService(t)
	_ = mockService.AddKey(pubkeyBytes)
	startServer(t, mockService)

	req := &AddKeyRequest{Pubkey: pubkey}
	res := &ErrorResponse{}
	httpRes := testhttp.DoPost(t, "http://localhost:7777/api/v1/admin/add-key", req, res)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, res.Message, "pubkey already exists")
}

func TestAddKeyRequest_InvalidKey(t *testing.T) {
	mockService := newMockWalletService(t)
	startServer(t, mockService)

	req := &AddKeyRequest{Pubkey: "0x00"}
	res := &ErrorResponse{}
	httpRes := testhttp.DoPost(t, "http://localhost:7777/api/v1/admin/add-key", req, res)
	require.Equal(t, http.StatusBadRequest, httpRes.StatusCode)
	require.Equal(t, res.Message, "pubkey hex string must be 68 characters long (with 0x prefix)")
}

func TestBlockHeightRequest_Ok(t *testing.T) {
	service := newMockWalletService(t)
	startServer(t, service)

	blockNumber := uint64(100)
	_ = service.store.SetBlockNumber(blockNumber)
	res := &BlockHeightResponse{}
	httpRes := testhttp.DoGet(t, "http://localhost:7777/api/v1/block-height", res)

	require.Equal(t, http.StatusOK, httpRes.StatusCode)
	require.EqualValues(t, blockNumber, res.BlockHeight)
}

func TestInvalidUrl_NotFound(t *testing.T) {
	startServer(t, newMockWalletService(t))

	// verify request to to non-existent /api2 endpoint returns 404
	httpRes, err := http.Get("http://localhost:7777/api2/v1/list-bills")
	require.NoError(t, err)
	require.Equal(t, 404, httpRes.StatusCode)

	// verify request to to non-existent version endpoint returns 404
	httpRes, err = http.Get("http://localhost:7777/api/v5/list-bills")
	require.NoError(t, err)
	require.Equal(t, 404, httpRes.StatusCode)
}

func startServer(t *testing.T, service WalletBackendService) {
	server := NewHttpServer(":7777", 100, service)
	err := server.Start()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = server.Shutdown(context.Background())
	})
}