package money

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	testmoney "github.com/alphabill-org/alphabill-wallet/internal/testutils/money"
	"github.com/stretchr/testify/require"
)

func TestWalletSendFunction_Ok(t *testing.T) {
	w := createTestWallet(t, testmoney.NewRpcClientMock(
		testmoney.WithOwnerBill(testmoney.NewBill(t, 50, 1)),
		testmoney.WithOwnerFeeCreditRecord(newMoneyFCR(t, testPubKey0Hash, 100*1e8, 200)),
	))
	validPubKey := make([]byte, 33)
	amount := uint64(50)
	ctx := context.Background()

	// test ok response
	_, err := w.Send(ctx, SendCmd{Receivers: []ReceiverData{{PubKey: validPubKey, Amount: amount}}})
	require.NoError(t, err)
}

func TestWalletSendFunction_NoFCR(t *testing.T) {
	w := createTestWallet(t, testmoney.NewRpcClientMock(
		testmoney.WithOwnerBill(testmoney.NewBill(t, 50, 1)),
	))
	validPubKey := make([]byte, 33)
	amount := uint64(50)
	ctx := context.Background()

	// test ok response
	_, err := w.Send(ctx, SendCmd{Receivers: []ReceiverData{{PubKey: validPubKey, Amount: amount}}})
	require.ErrorContains(t, err, "fee credit record not found")
}

func TestWalletSendFunction_InvalidPubKey(t *testing.T) {
	w := createTestWallet(t, testmoney.NewRpcClientMock())
	invalidPubKey := make([]byte, 32)
	amount := uint64(50)
	ctx := context.Background()

	// test ErrInvalidPubKey
	_, err := w.Send(ctx, SendCmd{Receivers: []ReceiverData{{PubKey: invalidPubKey, Amount: amount}}})
	require.ErrorContains(t, err, "invalid public key: public key must be in compressed secp256k1 format: got 32 "+
		"bytes, expected 33 bytes for public key 0x0000000000000000000000000000000000000000000000000000000000000000")
}

func TestWalletSendFunction_InsufficientBalance(t *testing.T) {
	w := createTestWallet(t, testmoney.NewRpcClientMock(
		testmoney.WithOwnerBill(testmoney.NewBill(t, 49, 1)),
		testmoney.WithOwnerFeeCreditRecord(newMoneyFCR(t, testPubKey0Hash, 100, 200)),
	))
	validPubKey := make([]byte, 33)
	amount := uint64(50)
	ctx := context.Background()

	// test ErrInsufficientBalance
	_, err := w.Send(ctx, SendCmd{Receivers: []ReceiverData{{PubKey: validPubKey, Amount: amount}}})
	require.ErrorContains(t, err, "insufficient balance for transaction")
}

func TestWalletSendFunction_ClientError(t *testing.T) {
	w := createTestWallet(t, testmoney.NewRpcClientMock(
		testmoney.WithError(errors.New("some error")),
		testmoney.WithOwnerBill(testmoney.NewBill(t, 50, 1)),
		testmoney.WithOwnerFeeCreditRecord(newMoneyFCR(t, testPubKey0Hash, 100*1e8, 200)),
	))
	validPubKey := make([]byte, 33)
	amount := uint64(50)

	// test PostTransactions returns error
	_, err := w.Send(context.Background(), SendCmd{Receivers: []ReceiverData{{PubKey: validPubKey, Amount: amount}}})
	require.ErrorContains(t, err, "some error")
}

func TestWalletSendFunction_WaitForConfirmation(t *testing.T) {
	moneyClient := testmoney.NewRpcClientMock(
		testmoney.WithOwnerBill(testmoney.NewBill(t, 100, 1)),
		testmoney.WithOwnerFeeCreditRecord(newMoneyFCR(t, testPubKey0Hash, 100, 200)),
	)
	w := createTestWallet(t, moneyClient)

	// test send successfully waits for confirmation
	txProofs, err := w.Send(context.Background(), SendCmd{
		Receivers:           []ReceiverData{{PubKey: make([]byte, 33), Amount: 50}},
		WaitForConfirmation: true,
	})
	require.NoError(t, err)
	require.NotNil(t, txProofs)
	require.Len(t, txProofs, 1)
	require.NotNil(t, txProofs[0])

	balance, err := w.GetBalance(context.Background(), GetBalanceCmd{})
	require.NoError(t, err)
	require.EqualValues(t, 100, balance)
}

func TestWalletSendFunction_WaitForMultipleTxConfirmations(t *testing.T) {
	moneyClient := testmoney.NewRpcClientMock(
		testmoney.WithOwnerBill(testmoney.NewBill(t, 10, 1)),
		testmoney.WithOwnerBill(testmoney.NewBill(t, 10, 2)),
		testmoney.WithOwnerFeeCreditRecord(newMoneyFCR(t, testPubKey0Hash, 100, 200)),
	)
	w := createTestWallet(t, moneyClient)

	// test send successfully waits for confirmation
	_, err := w.Send(context.Background(), SendCmd{
		Receivers:           []ReceiverData{{PubKey: make([]byte, 33), Amount: 20}},
		WaitForConfirmation: true,
	})
	require.NoError(t, err)
}

func TestWholeBalanceIsSentUsingBillTransferOrder(t *testing.T) {
	// create wallet with single bill
	moneyClient := testmoney.NewRpcClientMock(
		testmoney.WithOwnerBill(testmoney.NewBill(t, 100, 1)),
		testmoney.WithOwnerFeeCreditRecord(newMoneyFCR(t, testPubKey0Hash, 100, 200)),
	)
	w := createTestWallet(t, moneyClient)

	// when whole balance is spent
	_, err := w.Send(context.Background(), SendCmd{
		Receivers: []ReceiverData{{PubKey: make([]byte, 33), Amount: 100}},
	})
	require.NoError(t, err)

	// then a single transfer order should be sent
	require.Len(t, moneyClient.RecordedTxs, 1)
	require.Equal(t, moneyClient.RecordedTxs[0].Type, money.TransactionTypeTransfer)
}

func TestWalletSendFunction_LockedBillIsNotUsed(t *testing.T) {
	w := createTestWallet(t, testmoney.NewRpcClientMock(
		testmoney.WithOwnerBill(testmoney.NewLockedBill(t, 50, 1, []byte{1})),
		testmoney.WithOwnerFeeCreditRecord(newMoneyFCR(t, testPubKey0Hash, 100*1e8, 200)),
	))
	pubKey, err := hex.DecodeString(testPubKey0Hex)
	require.NoError(t, err)

	// test send returns error
	_, err = w.Send(context.Background(), SendCmd{
		Receivers: []ReceiverData{{PubKey: pubKey, Amount: 1}},
	})
	require.ErrorContains(t, err, "insufficient balance for transaction")
}

func TestWalletSendFunction_BillWithExactAmount(t *testing.T) {
	// create test wallet with 2 bills with different values
	exactBill := testmoney.NewBill(t, 77, 2)
	moneyClient := testmoney.NewRpcClientMock(
		testmoney.WithOwnerBill(testmoney.NewBill(t, 100, 1)),
		testmoney.WithOwnerBill(exactBill),
		testmoney.WithOwnerFeeCreditRecord(newMoneyFCR(t, testPubKey0Hash, 100, 200)),
	)
	w := createTestWallet(t, moneyClient)

	// run send command with amount equal to one of the bills
	txProofs, err := w.Send(context.Background(), SendCmd{
		Receivers:           []ReceiverData{{PubKey: make([]byte, 33), Amount: 77}},
		WaitForConfirmation: true,
	})

	// verify that the send command creates a single transfer for the bill with the exact value requested
	require.NoError(t, err)
	require.Len(t, txProofs, 1)
	txo, err := txProofs[0].GetTransactionOrderV1()
	require.NoError(t, err)
	require.Equal(t, money.TransactionTypeTransfer, txo.Type)
	require.EqualValues(t, exactBill.ID, txo.GetUnitID())
}

func TestWalletSendFunction_NWaySplit(t *testing.T) {
	// create test wallet with a single bill
	pubKey := make([]byte, 33)
	bill := testmoney.NewBill(t, 100, 1)
	moneyClient := testmoney.NewRpcClientMock(
		testmoney.WithOwnerBill(bill),
		testmoney.WithOwnerFeeCreditRecord(newMoneyFCR(t, testPubKey0Hash, 100, 200)),
	)
	w := createTestWallet(t, moneyClient)

	// execute send command to multiple receivers
	txProofs, err := w.Send(context.Background(), SendCmd{
		Receivers: []ReceiverData{
			{PubKey: pubKey, Amount: 5},
			{PubKey: pubKey, Amount: 5},
			{PubKey: pubKey, Amount: 5},
			{PubKey: pubKey, Amount: 5},
			{PubKey: pubKey, Amount: 5},
		},
		WaitForConfirmation: true,
	})

	// verify that the send command creates N-way split tx
	require.NoError(t, err)
	require.Len(t, txProofs, 1)
	txo, err := txProofs[0].GetTransactionOrderV1()
	require.NoError(t, err)
	require.Equal(t, money.TransactionTypeSplit, txo.Type)
	require.EqualValues(t, bill.ID, txo.GetUnitID())
	attr := &money.SplitAttributes{}
	err = txo.UnmarshalAttributes(attr)
	require.NoError(t, err)
	require.Len(t, attr.TargetUnits, 5)
	for _, u := range attr.TargetUnits {
		require.EqualValues(t, 5, u.Amount)
		require.EqualValues(t, templates.NewP2pkh256BytesFromKey(pubKey), u.OwnerPredicate)
	}
}

func newMoneyFCR(t *testing.T, pubKeyHashHex string, balance, counter uint64) *sdktypes.FeeCreditRecord {
	pubKeyHash, err := hex.DecodeString(pubKeyHashHex)
	require.NoError(t, err)
	return testmoney.NewMoneyFCR(t, pubKeyHash, balance, nil, counter)
}
