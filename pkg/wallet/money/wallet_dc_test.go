package money

import (
	"context"
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	billtx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	txbuilder "github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
	"github.com/fxamacker/cbor/v2"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestDustCollectionWontRunForSingleBill(t *testing.T) {
	// create wallet with a single bill
	bills := []*Bill{addBill(1)}
	billsList := createBillListJsonResponse(bills)

	w, mockClient := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{balance: 3, customBillList: billsList}))

	// when dc runs
	err := w.collectDust(context.Background(), false, 0)
	require.NoError(t, err)

	// then no txs are broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 0)
}

func TestDustCollectionMaxBillCount(t *testing.T) {
	// create wallet with max allowed bills for dc + 1
	tempNonce := uint256.NewInt(1)
	nonceBytes := util.Uint256ToBytes(tempNonce)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	bills := make([]*Bill, maxBillsForDustCollection+1)
	dcBills := make([]*Bill, maxBillsForDustCollection+1)
	for i := 0; i < maxBillsForDustCollection+1; i++ {
		bills[i] = addBill(uint64(i))
		dcBills[i] = addDcBill(t, k, uint256.NewInt(uint64(i)), nonceBytes, uint64(i), dcTimeoutBlockCount)
	}
	billsList := createBillListJsonResponse(bills)
	proofList := createBlockProofJsonResponse(t, bills, nil, 0, dcTimeoutBlockCount, nil)
	proofList = append(proofList, createBlockProofJsonResponse(t, dcBills, nonceBytes, 0, dcTimeoutBlockCount, k)...)

	w, mockClient := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &wallet.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &wallet.Proof{},
		},
	}))

	// when dc runs
	err = w.collectDust(context.Background(), false, 0)
	require.NoError(t, err)

	// then dc tx count should be equal to max allowed bills for dc plus 1 for the swap
	require.Len(t, mockClient.GetRecordedTransactions(), maxBillsForDustCollection+1)
}

func TestBasicDustCollection(t *testing.T) {
	// create wallet with 2 normal bills
	tempNonce := uint256.NewInt(1)
	nonceBytes := util.Uint256ToBytes(tempNonce)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	dcBills := []*Bill{addDcBill(t, k, uint256.NewInt(1), nonceBytes, 1, dcTimeoutBlockCount), addDcBill(t, k, uint256.NewInt(2), nonceBytes, 2, dcTimeoutBlockCount)}
	bills := []*Bill{addBill(1), addBill(2)}
	billsList := createBillListJsonResponse(bills)
	proofList := createBlockProofJsonResponse(t, bills, nil, 0, dcTimeoutBlockCount, nil)
	proofList = append(proofList, createBlockProofJsonResponse(t, dcBills, nonceBytes, 0, dcTimeoutBlockCount, k)...)
	expectedDcNonce := calculateDcNonce(bills)

	w, mockClient := CreateTestWalletWithManager(t, withBackendMock(t, &backendMockReturnConf{
		balance:        3,
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &wallet.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &wallet.Proof{},
		}}), am)

	// when dc runs
	err = w.collectDust(context.Background(), false, 0)
	require.NoError(t, err)

	// then two dc txs are broadcast plus one swap
	require.Len(t, mockClient.GetRecordedTransactions(), 3)
	for i, tx := range mockClient.GetRecordedTransactions()[0:2] {
		dcTx := parseDcTx(t, tx)
		require.NotNil(t, dcTx)
		require.EqualValues(t, expectedDcNonce, dcTx.Nonce)
		require.EqualValues(t, bills[i].Value, dcTx.TargetValue)
		require.EqualValues(t, bills[i].TxRecordHash, dcTx.Backlink)
		require.EqualValues(t, script.PredicatePayToPublicKeyHashDefault(k.PubKeyHash.Sha256), dcTx.TargetBearer)
	}

	// and expected swap is added to dc wait group
	require.Len(t, w.dcWg.swaps, 1)
	swap := w.dcWg.swaps[string(expectedDcNonce)]
	require.EqualValues(t, expectedDcNonce, swap.dcNonce)
	require.EqualValues(t, 3, swap.dcSum)
	require.EqualValues(t, dcTimeoutBlockCount, swap.timeout)
}

func TestDustCollectionWithSwap(t *testing.T) {
	// create wallet with 2 normal bills
	tempNonce := uint256.NewInt(1)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	bills := []*Bill{addBill(1), addBill(2)}
	expectedDcNonce := calculateDcNonce(bills)
	billsList := createBillListJsonResponse(bills)
	// proofs are polled twice, one for the regular bills and one for dc bills
	proofList := createBlockProofJsonResponse(t, bills, nil, 0, dcTimeoutBlockCount, k)
	proofList = append(proofList, createBlockProofJsonResponse(t, []*Bill{addDcBill(t, k, tempNonce, expectedDcNonce, 1, dcTimeoutBlockCount), addDcBill(t, k, tempNonce, expectedDcNonce, 2, dcTimeoutBlockCount)}, expectedDcNonce, 0, dcTimeoutBlockCount, k)...)

	w, mockClient := CreateTestWalletWithManager(t, withBackendMock(t, &backendMockReturnConf{
		balance:        3,
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &wallet.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &wallet.Proof{},
		},
	}), am)

	// when dc runs
	err = w.collectDust(context.Background(), false, 0)
	require.NoError(t, err)

	// then two dc txs + one swap tx are broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 3)
	for _, tx := range mockClient.GetRecordedTransactions()[0:2] {
		require.NotNil(t, parseDcTx(t, tx))
	}
	txSwap := parseSwapTx(t, mockClient.GetRecordedTransactions()[2])
	require.EqualValues(t, 3, txSwap.TargetValue)
	require.EqualValues(t, [][]byte{util.Uint256ToBytes(tempNonce), util.Uint256ToBytes(tempNonce)}, txSwap.BillIdentifiers)
	require.EqualValues(t, script.PredicatePayToPublicKeyHashDefault(k.PubKeyHash.Sha256), txSwap.OwnerCondition)
	require.Len(t, txSwap.DcTransfers, 2)
	require.Len(t, txSwap.Proofs, 2)

	// and expected swap is updated with swap timeout
	require.Len(t, w.dcWg.swaps, 1)
	swap := w.dcWg.swaps[string(expectedDcNonce)]
	require.EqualValues(t, expectedDcNonce, swap.dcNonce)
	require.EqualValues(t, 3, swap.dcSum)
	require.EqualValues(t, swapTimeoutBlockCount, swap.timeout)
}

func TestSwapWithExistingDCBillsBeforeDCTimeout(t *testing.T) {
	// create wallet with 2 dc bills
	roundNr := uint64(5)
	tempNonce := uint256.NewInt(1)
	nonceBytes := util.Uint256ToBytes(tempNonce)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	bills := []*Bill{addDcBill(t, k, tempNonce, nonceBytes, 1, dcTimeoutBlockCount), addDcBill(t, k, tempNonce, nonceBytes, 2, dcTimeoutBlockCount)}
	billsList := createBillListJsonResponse(bills)
	proofList := createBlockProofJsonResponse(t, bills, nonceBytes, 0, dcTimeoutBlockCount, k)
	w, mockClient := CreateTestWalletWithManager(t, withBackendMock(t, &backendMockReturnConf{
		balance:        3,
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &wallet.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &wallet.Proof{},
		}}), am)
	// set specific round number
	mockClient.SetMaxRoundNumber(roundNr)

	// when dc runs
	err = w.collectDust(context.Background(), false, 0)
	require.NoError(t, err)

	// then a swap tx is broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 1)
	txSwap := parseSwapTx(t, mockClient.GetRecordedTransactions()[0])
	require.EqualValues(t, 3, txSwap.TargetValue)
	require.EqualValues(t, [][]byte{nonceBytes, nonceBytes}, txSwap.BillIdentifiers)
	require.EqualValues(t, script.PredicatePayToPublicKeyHashDefault(k.PubKeyHash.Sha256), txSwap.OwnerCondition)
	require.Len(t, txSwap.DcTransfers, 2)
	require.Len(t, txSwap.Proofs, 2)

	// and expected swap is updated with swap timeout + round number
	require.Len(t, w.dcWg.swaps, 1)
	swap := w.dcWg.swaps[string(nonceBytes)]
	require.EqualValues(t, nonceBytes, swap.dcNonce)
	require.EqualValues(t, 3, swap.dcSum)
	require.EqualValues(t, swapTimeoutBlockCount+roundNr, swap.timeout)
}

func TestSwapWithExistingExpiredDCBills(t *testing.T) {
	// create wallet with 2 timed out dc bills
	tempNonce := uint256.NewInt(1)
	nonceBytes := util.Uint256ToBytes(tempNonce)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	bills := []*Bill{addDcBill(t, k, tempNonce, nonceBytes, 1, 0), addDcBill(t, k, tempNonce, nonceBytes, 2, 0)}
	billsList := createBillListJsonResponse(bills)
	proofList := createBlockProofJsonResponse(t, bills, nonceBytes, 0, 0, k)
	w, mockClient := CreateTestWalletWithManager(t, withBackendMock(t, &backendMockReturnConf{
		balance:        3,
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &wallet.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &wallet.Proof{},
		},
	}), am)

	// when dc runs
	err = w.collectDust(context.Background(), false, 0)
	require.NoError(t, err)

	// then a swap tx is broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 1)
	txSwap := parseSwapTx(t, mockClient.GetRecordedTransactions()[0])
	require.EqualValues(t, 3, txSwap.TargetValue)
	require.EqualValues(t, [][]byte{nonceBytes, nonceBytes}, txSwap.BillIdentifiers)
	require.EqualValues(t, script.PredicatePayToPublicKeyHashDefault(k.PubKeyHash.Sha256), txSwap.OwnerCondition)
	require.Len(t, txSwap.DcTransfers, 2)
	require.Len(t, txSwap.Proofs, 2)

	// and expected swap is updated with swap timeout
	require.Len(t, w.dcWg.swaps, 1)
	swap := w.dcWg.swaps[string(nonceBytes)]
	require.EqualValues(t, nonceBytes, swap.dcNonce)
	require.EqualValues(t, 3, swap.dcSum)
	require.EqualValues(t, swapTimeoutBlockCount, swap.timeout)
}

func TestDcNonceHashIsCalculatedInCorrectBillOrder(t *testing.T) {
	bills := []*Bill{
		{Id: uint256.NewInt(2)},
		{Id: uint256.NewInt(1)},
		{Id: uint256.NewInt(0)},
	}
	hasher := crypto.SHA256.New()
	for i := len(bills) - 1; i >= 0; i-- {
		hasher.Write(bills[i].GetID())
	}
	expectedNonce := hasher.Sum(nil)

	nonce := calculateDcNonce(bills)
	require.EqualValues(t, expectedNonce, nonce)
}

func TestSwapTxValuesAreCalculatedInCorrectBillOrder(t *testing.T) {
	w, _ := CreateTestWallet(t, nil)
	k, _ := w.am.GetAccountKey(0)

	dcBills := []*Bill{
		{Id: uint256.NewInt(2), TxProof: &wallet.Proof{TxRecord: createRandomDcTx()}},
		{Id: uint256.NewInt(1), TxProof: &wallet.Proof{TxRecord: createRandomDcTx()}},
		{Id: uint256.NewInt(0), TxProof: &wallet.Proof{TxRecord: createRandomDcTx()}},
	}
	dcNonce := calculateDcNonce(dcBills)
	var dcBillIds [][]byte
	for _, dcBill := range dcBills {
		dcBillIds = append(dcBillIds, dcBill.GetID())
	}

	var protoDcBills []*wallet.Bill
	for _, b := range dcBills {
		protoDcBills = append(protoDcBills, b.ToProto())
	}

	swapTxOrder, err := txbuilder.NewSwapTx(k, w.SystemID(), protoDcBills, dcNonce, dcBillIds, 10)
	require.NoError(t, err)

	swapAttr := &billtx.SwapDCAttributes{}
	err = swapTxOrder.UnmarshalAttributes(swapAttr)
	require.NoError(t, err)

	// verify bill ids in swap tx are in correct order (equal hash values)
	hasher := crypto.SHA256.New()
	for _, billId := range swapAttr.BillIdentifiers {
		hasher.Write(billId)
	}
	actualDcNonce := hasher.Sum(nil)
	require.EqualValues(t, dcNonce, actualDcNonce)
}

func TestSwapContainsUnconfirmedDustBillIds(t *testing.T) {
	// create wallet with three bills
	_ = log.InitStdoutLogger(log.INFO)
	b1 := addBill(1)
	b2 := addBill(2)
	b3 := addBill(3)
	nonce := calculateDcNonce([]*Bill{b1, b2, b3})
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)

	billsList := createBillListJsonResponse([]*Bill{b1, b2, b3})
	// proofs are polled twice, one for the regular bills and one for dc bills
	proofList := createBlockProofJsonResponse(t, []*Bill{b1, b2, b3}, nil, 0, dcTimeoutBlockCount, k)
	proofList = append(proofList, createBlockProofJsonResponse(t, []*Bill{addDcBill(t, k, b1.Id, nonce, 1, dcTimeoutBlockCount), addDcBill(t, k, b2.Id, nonce, 2, dcTimeoutBlockCount), addDcBill(t, k, b3.Id, nonce, 3, dcTimeoutBlockCount)}, nonce, 0, dcTimeoutBlockCount, k)...)
	w, mockClient := CreateTestWalletWithManager(t, withBackendMock(t, &backendMockReturnConf{
		balance:        3,
		customBillList: billsList,
		proofList:      proofList,
		feeCreditBill: &wallet.Bill{
			Id:      k.PrivKeyHash,
			Value:   100 * 1e8,
			TxProof: &wallet.Proof{},
		},
	}), am)

	// when dc runs
	err = w.collectDust(context.Background(), false, 0)
	require.NoError(t, err)

	verifyBlockHeight(t, w, 0)

	// and three dc txs are broadcast
	dcTxs := mockClient.GetRecordedTransactions()
	require.Len(t, dcTxs, 4)
	for _, tx := range dcTxs[0:3] {
		require.NotNil(t, parseDcTx(t, tx))
	}

	// and swap should contain all bill ids
	tx := mockClient.GetRecordedTransactions()[3]
	attr := parseSwapTx(t, tx)
	require.EqualValues(t, nonce, tx.UnitID())
	require.Len(t, attr.BillIdentifiers, 3)
	require.Equal(t, b1.Id, uint256.NewInt(0).SetBytes(attr.BillIdentifiers[0]))
	require.Equal(t, b2.Id, uint256.NewInt(0).SetBytes(attr.BillIdentifiers[1]))
	require.Equal(t, b3.Id, uint256.NewInt(0).SetBytes(attr.BillIdentifiers[2]))
	require.Len(t, attr.DcTransfers, 3)
	require.Equal(t, dcTxs[0], attr.DcTransfers[0].TransactionOrder)
	require.Equal(t, dcTxs[1], attr.DcTransfers[1].TransactionOrder)
	require.Equal(t, dcTxs[2], attr.DcTransfers[2].TransactionOrder)
}

func addBill(value uint64) *Bill {
	b1 := Bill{
		Id:      uint256.NewInt(value),
		Value:   value,
		TxHash:  hash.Sum256([]byte{byte(value)}),
		TxProof: &wallet.Proof{},
	}
	return &b1
}

func addDcBill(t *testing.T, k *account.AccountKey, id *uint256.Int, nonce []byte, value uint64, timeout uint64) *Bill {
	b := Bill{
		Id:      id,
		Value:   value,
		TxHash:  hash.Sum256([]byte{byte(value)}),
		TxProof: &wallet.Proof{},
	}

	tx, err := txbuilder.NewDustTx(k, []byte{0, 0, 0, 0}, b.ToProto(), nonce, timeout)
	require.NoError(t, err)
	b.TxProof = &wallet.Proof{TxRecord: &types.TransactionRecord{TransactionOrder: tx}}

	b.IsDcBill = true
	b.DcNonce = nonce
	b.DcTimeout = timeout
	b.DcExpirationTimeout = dustBillDeletionTimeout

	require.NoError(t, err)
	return &b
}

func verifyBlockHeight(t *testing.T, w *Wallet, blockHeight uint64) {
	actualBlockHeight, err := w.AlphabillClient.GetRoundNumber(context.Background())
	require.NoError(t, err)
	require.Equal(t, blockHeight, actualBlockHeight)
}

func parseBillTransferTx(t *testing.T, tx *types.TransactionOrder) *billtx.TransferAttributes {
	transferTx := &billtx.TransferAttributes{}
	err := tx.UnmarshalAttributes(transferTx)
	require.NoError(t, err)
	return transferTx
}

func parseDcTx(t *testing.T, tx *types.TransactionOrder) *billtx.TransferDCAttributes {
	dcTx := &billtx.TransferDCAttributes{}
	err := tx.UnmarshalAttributes(dcTx)
	require.NoError(t, err)
	return dcTx
}

func parseSwapTx(t *testing.T, tx *types.TransactionOrder) *billtx.SwapDCAttributes {
	txSwap := &billtx.SwapDCAttributes{}
	err := tx.UnmarshalAttributes(txSwap)
	require.NoError(t, err)
	return txSwap
}

func createRandomDcTx() *types.TransactionRecord {
	return &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID:       []byte{0, 0, 0, 0},
				Type:           billtx.PayloadTypeTransDC,
				UnitID:         hash.Sum256([]byte{0x00}),
				Attributes:     randomTransferDCAttributes(),
				ClientMetadata: &types.ClientMetadata{Timeout: 1000},
			},
			OwnerProof: script.PredicateArgumentEmpty(),
		},
		ServerMetadata: nil,
	}
}

func randomTransferDCAttributes() []byte {
	attr := &billtx.TransferDCAttributes{
		TargetBearer: script.PredicateAlwaysTrue(),
		Backlink:     hash.Sum256([]byte{}),
		Nonce:        hash.Sum256([]byte{}),
		TargetValue:  100,
	}
	attrBytes, _ := cbor.Marshal(attr)
	return attrBytes
}
