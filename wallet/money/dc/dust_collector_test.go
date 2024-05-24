package dc

import (
	"context"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/internal/testutils/logger"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/testutil"
)

func TestDC_OK(t *testing.T) {
	// create wallet with 3 normal bills
	accountKeys, err := account.NewKeys("dinosaur simple verify deliver bless ridge monkey design venue six problem lucky")
	require.NoError(t, err)
	targetBillID := money.NewBillID(nil, []byte{3})
	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{1}, &money.BillData{V: 1, Counter: 1})),
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{2}, &money.BillData{V: 2, Counter: 2})),
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{3}, &money.BillData{V: 3, Counter: 3})),
		testutil.WithOwnerFeeCreditBill(testutil.NewMoneyFCR(accountKeys.AccountKey.PubKeyHash.Sha256, &fc.FeeCreditRecord{Balance: 100, Counter: 100})),
	)
	dc := NewDustCollector(money.DefaultSystemID, 10, 10, moneyClient, logger.New(t))

	// when dc runs
	dcResult, err := dc.CollectDust(context.Background(), accountKeys.AccountKey)
	require.NoError(t, err)
	require.NotNil(t, dcResult.SwapProof)

	// then swap contains two dc txs
	attr := &money.SwapDCAttributes{}
	txo := dcResult.SwapProof.TxRecord.TransactionOrder
	err = txo.UnmarshalAttributes(&attr)
	require.NoError(t, err)
	require.EqualValues(t, 3, attr.TargetValue)
	require.Len(t, attr.DcTransfers, 2)
	require.Len(t, attr.DcTransferProofs, 2)
	require.EqualValues(t, targetBillID, txo.UnitID())
}

func TestDCWontRunForSingleBill(t *testing.T) {
	// create rpc client mock with single bill
	accountKeys, err := account.NewKeys("dinosaur simple verify deliver bless ridge monkey design venue six problem lucky")
	require.NoError(t, err)
	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{1}, &money.BillData{V: 1, Counter: 1})),
		testutil.WithOwnerFeeCreditBill(testutil.NewMoneyFCR(accountKeys.AccountKey.PubKeyHash.Sha256, &fc.FeeCreditRecord{Balance: 100, Counter: 100})),
	)
	dc := NewDustCollector(money.DefaultSystemID, 10, 10, moneyClient, logger.New(t))

	// when dc runs
	dcResult, err := dc.CollectDust(context.Background(), accountKeys.AccountKey)
	require.NoError(t, err)

	// then swap proof is not returned
	require.Nil(t, dcResult)
}

func TestAllBillsAreSwapped_WhenWalletBillCountEqualToMaxBillCount(t *testing.T) {
	// create wallet with bill count equal to max dust collection bill count
	maxBillsPerDC := 3
	accountKeys, err := account.NewKeys("dinosaur simple verify deliver bless ridge monkey design venue six problem lucky")
	require.NoError(t, err)
	targetBillID := money.NewBillID(nil, []byte{3})
	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{1}, &money.BillData{V: 1, Counter: 1})),
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{2}, &money.BillData{V: 2, Counter: 2})),
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{3}, &money.BillData{V: 3, Counter: 3})),
		testutil.WithOwnerFeeCreditBill(testutil.NewMoneyFCR(accountKeys.AccountKey.PubKeyHash.Sha256, &fc.FeeCreditRecord{Balance: 100, Counter: 100})),
	)
	w := NewDustCollector(money.DefaultSystemID, maxBillsPerDC, 10, moneyClient, logger.New(t))

	// when dc runs
	dcResult, err := w.CollectDust(context.Background(), accountKeys.AccountKey)
	require.NoError(t, err)

	// then swap tx should be returned
	require.NotNil(t, dcResult.SwapProof)
	require.EqualValues(t, targetBillID, dcResult.SwapProof.TxRecord.TransactionOrder.UnitID())

	// and swap contains correct dc transfers
	swapAttr := &money.SwapDCAttributes{}
	swapTxo := dcResult.SwapProof.TxRecord.TransactionOrder
	err = swapTxo.UnmarshalAttributes(swapAttr)
	require.NoError(t, err)
	require.Len(t, swapAttr.DcTransfers, maxBillsPerDC-1)
	require.Len(t, swapAttr.DcTransferProofs, maxBillsPerDC-1)
	require.EqualValues(t, 3, swapAttr.TargetValue)
	require.EqualValues(t, targetBillID, swapTxo.UnitID())
}

func TestOnlyFirstNBillsAreSwapped_WhenBillCountOverLimit(t *testing.T) {
	// create rpc client mock with bills = max dust collection bill count
	maxBillsPerDC := 3
	accountKeys, err := account.NewKeys("dinosaur simple verify deliver bless ridge monkey design venue six problem lucky")
	require.NoError(t, err)
	targetBillID := money.NewBillID(nil, []byte{4})
	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{1}, &money.BillData{V: 1, Counter: 1})),
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{2}, &money.BillData{V: 2, Counter: 2})),
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{3}, &money.BillData{V: 3, Counter: 3})),
		testutil.WithOwnerBill(testutil.NewMoneyBill([]byte{4}, &money.BillData{V: 4, Counter: 4})),
		testutil.WithOwnerFeeCreditBill(testutil.NewMoneyFCR(accountKeys.AccountKey.PubKeyHash.Sha256, &fc.FeeCreditRecord{Balance: 100, Counter: 100})),
	)
	w := NewDustCollector(money.DefaultSystemID, maxBillsPerDC, 10, moneyClient, logger.New(t))

	// when dc runs
	dcResult, err := w.CollectDust(context.Background(), accountKeys.AccountKey)
	require.NoError(t, err)
	require.NotNil(t, dcResult.SwapProof)

	// then swap contains correct dc transfers
	swapTxo := dcResult.SwapProof.TxRecord.TransactionOrder
	swapAttr := &money.SwapDCAttributes{}
	err = swapTxo.UnmarshalAttributes(swapAttr)
	require.EqualValues(t, targetBillID, swapTxo.UnitID())
	require.NoError(t, err)
	require.Len(t, swapAttr.DcTransfers, maxBillsPerDC)
	require.Len(t, swapAttr.DcTransferProofs, maxBillsPerDC)
	require.EqualValues(t, 6, swapAttr.TargetValue)
	require.EqualValues(t, targetBillID, swapTxo.UnitID())
}
