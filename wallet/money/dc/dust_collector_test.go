package dc

import (
	"context"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	testmoney "github.com/alphabill-org/alphabill-wallet/internal/testutils/money"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/internal/testutils/logger"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
)

const maxFee = 10

func TestDC_OK(t *testing.T) {
	// create wallet with 3 normal bills
	accountKeys, err := account.NewKeys("dinosaur simple verify deliver bless ridge monkey design venue six problem lucky")
	require.NoError(t, err)
	targetBill := testmoney.NewBill(3, 3)
	moneyClient := testmoney.NewRpcClientMock(
		testmoney.WithOwnerBill(testmoney.NewBill(1, 1)),
		testmoney.WithOwnerBill(testmoney.NewBill(2, 2)),
		testmoney.WithOwnerBill(targetBill),
		testmoney.WithOwnerFeeCreditRecord(
			testmoney.NewMoneyFCR(t, accountKeys.AccountKey.PubKeyHash.Sha256, 100, maxFee, 100)),
	)
	dc := NewDustCollector(10, 10, moneyClient, 10, logger.New(t))

	// when dc runs
	dcResult, err := dc.CollectDust(context.Background(), accountKeys.AccountKey)
	require.NoError(t, err)
	require.NotNil(t, dcResult.SwapProof)

	// then swap contains two dc txs
	attr := &money.SwapDCAttributes{}
	txo, err := dcResult.SwapProof.GetTransactionOrderV1()
	require.NoError(t, err)
	err = txo.UnmarshalAttributes(&attr)
	require.NoError(t, err)
	require.Len(t, attr.DustTransferProofs, 2)
	require.EqualValues(t, targetBill.ID, txo.GetUnitID())
}

func TestDCWontRunForSingleBill(t *testing.T) {
	// create rpc client mock with single bill
	accountKeys, err := account.NewKeys("dinosaur simple verify deliver bless ridge monkey design venue six problem lucky")
	require.NoError(t, err)
	moneyClient := testmoney.NewRpcClientMock(
		testmoney.WithOwnerBill(testmoney.NewBill(1, 1)),
		testmoney.WithOwnerFeeCreditRecord(
			testmoney.NewMoneyFCR(t, accountKeys.AccountKey.PubKeyHash.Sha256, 100, 0, 100)),
	)
	dc := NewDustCollector(10, 10, moneyClient, maxFee, logger.New(t))

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
	targetBill := testmoney.NewBill(3, 3)
	moneyClient := testmoney.NewRpcClientMock(
		testmoney.WithOwnerBill(testmoney.NewBill(1, 1)),
		testmoney.WithOwnerBill(testmoney.NewBill(2, 2)),
		testmoney.WithOwnerBill(targetBill),
		testmoney.WithOwnerFeeCreditRecord(testmoney.NewMoneyFCR(t, accountKeys.AccountKey.PubKeyHash.Sha256, 100, 0, 100)),
	)
	w := NewDustCollector(maxBillsPerDC, 10, moneyClient, maxFee, logger.New(t))

	// when dc runs
	dcResult, err := w.CollectDust(context.Background(), accountKeys.AccountKey)
	require.NoError(t, err)

	// then swap tx should be returned
	require.NotNil(t, dcResult.SwapProof)
	swapTxo, err := dcResult.SwapProof.GetTransactionOrderV1()
	require.NoError(t, err)
	require.EqualValues(t, targetBill.ID, swapTxo.GetUnitID())

	// and swap contains correct dc transfers
	swapAttr := &money.SwapDCAttributes{}
	require.NoError(t, err)
	err = swapTxo.UnmarshalAttributes(swapAttr)
	require.NoError(t, err)
	require.Len(t, swapAttr.DustTransferProofs, maxBillsPerDC-1)
	require.EqualValues(t, targetBill.ID, swapTxo.GetUnitID())
}

func TestOnlyFirstNBillsAreSwapped_WhenBillCountOverLimit(t *testing.T) {
	// create rpc client mock with bills = max dust collection bill count
	maxBillsPerDC := 3
	accountKeys, err := account.NewKeys("dinosaur simple verify deliver bless ridge monkey design venue six problem lucky")
	require.NoError(t, err)
	targetBill := testmoney.NewBill(4, 4)
	moneyClient := testmoney.NewRpcClientMock(
		testmoney.WithOwnerBill(testmoney.NewBill(1, 1)),
		testmoney.WithOwnerBill(testmoney.NewBill(2, 2)),
		testmoney.WithOwnerBill(testmoney.NewBill(3, 3)),
		testmoney.WithOwnerBill(targetBill),
		testmoney.WithOwnerFeeCreditRecord(testmoney.NewMoneyFCR(t, accountKeys.AccountKey.PubKeyHash.Sha256, 100, 0, 100)),
	)
	w := NewDustCollector(maxBillsPerDC, 10, moneyClient, maxFee, logger.New(t))

	// when dc runs
	dcResult, err := w.CollectDust(context.Background(), accountKeys.AccountKey)
	require.NoError(t, err)
	require.NotNil(t, dcResult.SwapProof)

	// then swap contains correct dc transfers
	swapTxo, err := dcResult.SwapProof.GetTransactionOrderV1()
	require.NoError(t, err)
	swapAttr := &money.SwapDCAttributes{}
	err = swapTxo.UnmarshalAttributes(swapAttr)
	require.EqualValues(t, targetBill.ID, swapTxo.GetUnitID())
	require.NoError(t, err)
	require.Len(t, swapAttr.DustTransferProofs, maxBillsPerDC)
	require.EqualValues(t, targetBill.ID, swapTxo.GetUnitID())
}
