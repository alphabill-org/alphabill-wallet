package money

import (
	"context"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill-wallet/wallet/money/api"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/testutil"
)

func TestWalletSendFunction_Ok(t *testing.T) {
	w := createTestWallet(t, testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{1}, &money.BillData{V: 50, Counter: 1})),
		testutil.WithOwnerFeeCreditBill(newMoneyFCB(t, testPubKey0Hash, &fc.FeeCreditRecord{Balance: 100 * 1e8, Counter: 200})),
	))
	validPubKey := make([]byte, 33)
	amount := uint64(50)
	ctx := context.Background()

	// test ok response
	_, err := w.Send(ctx, SendCmd{Receivers: []ReceiverData{{PubKey: validPubKey, Amount: amount}}})
	require.NoError(t, err)
}

func TestWalletSendFunction_InvalidPubKey(t *testing.T) {
	w := createTestWallet(t, testutil.NewRpcClientMock())
	invalidPubKey := make([]byte, 32)
	amount := uint64(50)
	ctx := context.Background()

	// test ErrInvalidPubKey
	_, err := w.Send(ctx, SendCmd{Receivers: []ReceiverData{{PubKey: invalidPubKey, Amount: amount}}})
	require.ErrorContains(t, err, "invalid public key: public key must be in compressed secp256k1 format: got 32 "+
		"bytes, expected 33 bytes for public key 0x0000000000000000000000000000000000000000000000000000000000000000")
}

func TestWalletSendFunction_InsufficientBalance(t *testing.T) {
	w := createTestWallet(t, testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{1}, &money.BillData{V: 49, Counter: 1})),
		testutil.WithOwnerFeeCreditBill(newMoneyFCB(t, testPubKey0Hash, &fc.FeeCreditRecord{Balance: 100, Counter: 200})),
	))
	validPubKey := make([]byte, 33)
	amount := uint64(50)
	ctx := context.Background()

	// test ErrInsufficientBalance
	_, err := w.Send(ctx, SendCmd{Receivers: []ReceiverData{{PubKey: validPubKey, Amount: amount}}})
	require.ErrorContains(t, err, "insufficient balance for transaction")
}

func TestWalletSendFunction_ClientError(t *testing.T) {
	w := createTestWallet(t, testutil.NewRpcClientMock(
		testutil.WithError(errors.New("some error")),
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{1}, &money.BillData{V: 50, Counter: 1})),
		testutil.WithOwnerFeeCreditBill(newMoneyFCB(t, testPubKey0Hash, &fc.FeeCreditRecord{Balance: 100 * 1e8, Counter: 200})),
	))
	validPubKey := make([]byte, 33)
	amount := uint64(50)

	// test PostTransactions returns error
	_, err := w.Send(context.Background(), SendCmd{Receivers: []ReceiverData{{PubKey: validPubKey, Amount: amount}}})
	require.ErrorContains(t, err, "some error")
}

func TestWalletSendFunction_WaitForConfirmation(t *testing.T) {
	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{1}, &money.BillData{V: 100, Counter: 1})),
		testutil.WithOwnerFeeCreditBill(newMoneyFCB(t, testPubKey0Hash, &fc.FeeCreditRecord{Balance: 100, Counter: 200})),
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
	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{1}, &money.BillData{V: 10, Counter: 1})),
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{2}, &money.BillData{V: 10, Counter: 2})),
		testutil.WithOwnerFeeCreditBill(newMoneyFCB(t, testPubKey0Hash, &fc.FeeCreditRecord{Balance: 100, Counter: 200})),
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
	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{1}, &money.BillData{V: 100, Counter: 1})),
		testutil.WithOwnerFeeCreditBill(newMoneyFCB(t, testPubKey0Hash, &fc.FeeCreditRecord{Balance: 100, Counter: 200})),
	)
	w := createTestWallet(t, moneyClient)

	// when whole balance is spent
	_, err := w.Send(context.Background(), SendCmd{
		Receivers: []ReceiverData{{PubKey: make([]byte, 33), Amount: 100}},
	})
	require.NoError(t, err)

	// then bill transfer order should be sent
	require.Len(t, moneyClient.RecordedTxs, 1)
	transferTx := parseBillTransferTx(t, moneyClient.RecordedTxs[0])
	require.EqualValues(t, 100, transferTx.TargetValue)
}

func TestWalletSendFunction_LockedBillIsNotUsed(t *testing.T) {
	w := createTestWallet(t, testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{1}, &money.BillData{V: 50, Counter: 1, Locked: wallet.LockReasonManual})),
		testutil.WithOwnerFeeCreditBill(newMoneyFCB(t, testPubKey0Hash, &fc.FeeCreditRecord{Balance: 100 * 1e8, Counter: 200})),
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
	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{1}, &money.BillData{V: 100, Counter: 1})),
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{2}, &money.BillData{V: 77, Counter: 2})),
		testutil.WithOwnerFeeCreditBill(newMoneyFCB(t, testPubKey0Hash, &fc.FeeCreditRecord{Balance: 100, Counter: 200})),
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
	require.Equal(t, money.PayloadTypeTransfer, txProofs[0].TxRecord.TransactionOrder.PayloadType())
	require.EqualValues(t, money.NewBillID(nil, []byte{2}), txProofs[0].TxRecord.TransactionOrder.UnitID())
}

func TestWalletSendFunction_NWaySplit(t *testing.T) {
	// create test wallet with a single bill
	pubKey := make([]byte, 33)
	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{1}, &money.BillData{V: 100, Counter: 1})),
		testutil.WithOwnerFeeCreditBill(newMoneyFCB(t, testPubKey0Hash, &fc.FeeCreditRecord{Balance: 100, Counter: 200})),
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
	txProof := txProofs[0]
	require.Equal(t, money.PayloadTypeSplit, txProof.TxRecord.TransactionOrder.PayloadType())
	require.EqualValues(t, money.NewBillID(nil, []byte{1}), txProof.TxRecord.TransactionOrder.UnitID())
	attr := &money.SplitAttributes{}
	err = txProof.TxRecord.TransactionOrder.UnmarshalAttributes(attr)
	require.NoError(t, err)
	require.Len(t, attr.TargetUnits, 5)
	for _, u := range attr.TargetUnits {
		require.EqualValues(t, 5, u.Amount)
		require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(pubKey)), u.OwnerCondition)
	}
}

func parseBillTransferTx(t *testing.T, tx *types.TransactionOrder) *money.TransferAttributes {
	transferTx := &money.TransferAttributes{}
	err := tx.UnmarshalAttributes(transferTx)
	require.NoError(t, err)
	return transferTx
}

func newMoneyFCB(t *testing.T, pubKeyHashHex string, fcr *fc.FeeCreditRecord) *api.FeeCreditBill {
	pubKeyHash, err := hex.DecodeString(pubKeyHashHex)
	require.NoError(t, err)
	return testutil.NewMoneyFCR(pubKeyHash, fcr)
}
