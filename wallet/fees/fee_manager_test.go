package fees

import (
	"context"
	"crypto"
	"log/slog"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/stretchr/testify/require"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/logger"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/testutil"
)

const (
	moneySystemID  types.SystemID = 1
	tokensSystemID types.SystemID = 2
	maxFee                        = 3
)

/*
Wallet has single bill with value 1.00000000
Add fee credit with the full value 1.00000000
TransferFCTx with 1.00000000 value and AddFCTx transactions should be sent.
*/
func TestAddFeeCredit_OK(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	bill := testutil.NewBill(100000000, 20)

	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(bill))

	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

	// add fees
	res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000000})
	require.NoError(t, err)
	require.Len(t, res.Proofs, 1)
	require.Nil(t, res.Proofs[0].LockFC)
	require.NotNil(t, res.Proofs[0].TransferFC)
	require.NotNil(t, res.Proofs[0].AddFC)

	// verify fee context is deleted
	pk, err := am.GetPublicKey(0)
	require.NoError(t, err)
	feeCtx, err := feeManagerDB.GetAddFeeContext(pk)
	require.NoError(t, err)
	require.Nil(t, feeCtx)

	// verify correct transferFC amount was sent
	var attr *fc.TransferFeeCreditAttributes
	err = res.Proofs[0].TransferFC.TxRecord.TransactionOrder.UnmarshalAttributes(&attr)
	require.NoError(t, err)
	require.EqualValues(t, 100000000, attr.Amount)
}

func TestAddFeeCredit_TokensPartitionOK(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)

	// money client has round number 100, tokens client 1000
	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewBill(100000000, 2)),
		testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 1e8, Counter: 111})),
		testutil.WithRoundNumber(100),
	)
	tokensClient := testutil.NewRpcClientMock(
		testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 1e8, Counter: 222})),
		testutil.WithRoundNumber(1000),
	)
	db := createFeeManagerDB(t)
	feeManager := newTokensPartitionFeeManager(am, db, moneyClient, tokensClient, logger.New(t))

	// add fees
	res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000000, DisableLocking: true})
	require.NoError(t, err)
	require.Len(t, res.Proofs, 1)
	require.Nil(t, res.Proofs[0].LockFC)
	require.NotNil(t, res.Proofs[0].TransferFC)
	require.NotNil(t, res.Proofs[0].AddFC)

	// verify tokens partition timeout is used for transferFC
	var attr *fc.TransferFeeCreditAttributes
	err = res.Proofs[0].TransferFC.TxRecord.TransactionOrder.UnmarshalAttributes(&attr)
	require.NoError(t, err)
	require.EqualValues(t, 1000+transferFCLatestAdditionTime, attr.LatestAdditionTime)
}

/*
Wallet has single bill and fee credit record,
when adding fees LockFCTx, TransferFCTx and AddFCTx transactions should be sent.
*/
func TestAddFeeCredit_ExistingFeeCreditBillOK(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)

	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewBill(100000000, 1)),
		testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 100000002, Counter: 2})),
	)
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

	// add fees
	res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000000})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Proofs, 1)
	proofs := res.Proofs[0]
	require.NotNil(t, proofs.LockFC)
	require.NotNil(t, proofs.TransferFC)
	require.NotNil(t, proofs.AddFC)

	// verify fee ctx is removed
	pk, err := am.GetPublicKey(0)
	require.NoError(t, err)
	feeCtx, err := feeManagerDB.GetAddFeeContext(pk)
	require.NoError(t, err)
	require.Nil(t, feeCtx)
}

/*
Wallet has multiple bills,
when adding fee credit with amount greater than the largest bill then
the result should have two sets of txs with the combined amount that matches what was requested
*/
func TestAddFeeCredit_MultipleBills(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)

	largestBill := testutil.NewBill(100000003, 3)
	secondLargestBill := testutil.NewBill(100000002, 2)
	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewBill(100000001, 1)),
		testutil.WithOwnerBill(secondLargestBill),
		testutil.WithOwnerBill(largestBill),
		testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 100000004, Counter: 4})),
	)
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

	// verify that there are 2 pairs of txs sent and that the amounts match
	res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 200000000})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Proofs, 2)
	proofs := res.Proofs

	// first transfer amount should match the largest bill
	firstTransFCAttr := &fc.TransferFeeCreditAttributes{}
	err = proofs[0].TransferFC.TxRecord.TransactionOrder.UnmarshalAttributes(firstTransFCAttr)
	require.NoError(t, err)
	require.Equal(t, largestBill.ID, proofs[0].TransferFC.TxRecord.TransactionOrder.UnitID())
	require.EqualValues(t, 100000003, firstTransFCAttr.Amount)

	// second transfer amount should match the remaining value
	secondTransFCAttr := &fc.TransferFeeCreditAttributes{}
	err = proofs[1].TransferFC.TxRecord.TransactionOrder.UnmarshalAttributes(secondTransFCAttr)
	require.NoError(t, err)
	require.Equal(t, secondLargestBill.ID, proofs[1].TransferFC.TxRecord.TransactionOrder.UnitID())
	require.EqualValues(t, 200000000-100000003, secondTransFCAttr.Amount)
}

/*
Wallet has no bills.
Trying to add fee credit should return error "wallet does not contain any bills".
*/
func TestAddFeeCredit_NoBillsReturnsError(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyClient := testutil.NewRpcClientMock()
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

	// verify that error is returned
	res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000000})
	require.ErrorContains(t, err, "wallet does not contain any bills")
	require.Nil(t, res)
}

/*
Wallet contains existing context for reclaim. Trying to add fee credit should return error
"wallet contains unreclaimed fee credit, run the reclaim command before adding fee credit"
*/
func TestAddFeeCredit_FeeManagerContainsExistingReclaimContext(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyClient := testutil.NewRpcClientMock()
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

	// create fee context for reclaim
	err = feeManagerDB.SetReclaimFeeContext(accountKey.PubKey, &ReclaimFeeCreditCtx{})
	require.NoError(t, err)

	// verify error is returned
	res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000000})
	require.ErrorContains(t, err, "wallet contains unreclaimed fee credit, run the reclaim command before adding fee credit")
	require.Nil(t, res)
}

/*
Wallet has two bills: one locked for dust collection and one normal not locked bill.
Adding fee credit should use the unlocked bill not change the locked bill.
*/
func TestAddFeeCredit_WalletContainsLockedBillForDustCollection(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	unlockedBill := testutil.NewBill(100000001, 1)
	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(unlockedBill),
		testutil.WithOwnerBill(testutil.NewLockedBill(100000002, 2, wallet.LockReasonCollectDust)),
	)
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

	// verify that the smaller bill is used to create fee credit
	res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000001})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Proofs, 1)
	proofs := res.Proofs[0]
	require.Nil(t, proofs.LockFC)
	require.NotNil(t, proofs.TransferFC)
	require.NotNil(t, proofs.AddFC)
	require.EqualValues(t, unlockedBill.ID, proofs.TransferFC.TxRecord.TransactionOrder.UnitID())
}

func TestAddFeeCreditForMoneyPartition_ExistingAddProcessForTokensPartition(t *testing.T) {
	// create fee manager for money partition
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)

	bill := testutil.NewBill(0, 2)
	moneyClient := testutil.NewRpcClientMock(testutil.WithOwnerBill(bill))
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

	// create fee context with token partition id
	feeCtx := &AddFeeCreditCtx{
		TargetPartitionID: tokensSystemID,
		FeeCreditRecordID: []byte{1},
		TargetBillID:      bill.ID,
		TargetBillCounter: bill.Counter,
	}
	err = feeManagerDB.SetAddFeeContext(accountKey.PubKey, feeCtx)
	require.NoError(t, err)

	// when attempting to add fees for money partition
	res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})

	// then error must be returned
	require.ErrorIs(t, err, ErrInvalidPartition)
	require.Nil(t, res)

	// and feeCtx is not deleted
	actualFeeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
	require.NoError(t, err)
	require.EqualValues(t, feeCtx, actualFeeCtx)
}

func TestReclaimFeeCreditForMoneyPartition_ExistingReclaimProcessForTokensPartition(t *testing.T) {
	// create fee manager for money partition
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)

	moneyClient := testutil.NewRpcClientMock()
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

	// create fee context with token partition id
	feeCtx := &ReclaimFeeCreditCtx{
		TargetPartitionID: tokensSystemID,
		TargetBillID:      []byte{2},
		TargetBillCounter: 2,
	}
	err = feeManagerDB.SetReclaimFeeContext(accountKey.PubKey, feeCtx)
	require.NoError(t, err)

	// when attempting to reclaim fees for money partition
	res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})

	// then error must be returned
	require.ErrorIs(t, err, ErrInvalidPartition)
	require.Nil(t, res)

	// and money fee context is not deleted
	actualFeeCtx, err := feeManagerDB.GetReclaimFeeContext(accountKey.PubKey)
	require.NoError(t, err)
	require.Equal(t, feeCtx, actualFeeCtx)
}

/*
Wallet has three bills: one locked for dust collection, one normal not locked bill and fee credit record.
Reclaiming fee credit should target the unlocked bill not change the locked bill.
*/
func TestReclaimFeeCredit_WalletContainsLockedBillForDustCollection(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)

	unlockedBill := testutil.NewBill(100000001, 1)
	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(unlockedBill),
		testutil.WithOwnerBill(testutil.NewLockedBill(100000002, 2, wallet.LockReasonCollectDust)),
		testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 100, Counter: 111})),
	)
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

	// verify that the non-locked bill can be reclaimed
	res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, res.Proofs)
	require.NotNil(t, res.Proofs.Lock)
	require.NotNil(t, res.Proofs.CloseFC)
	require.NotNil(t, res.Proofs.ReclaimFC)

	var attr *fc.CloseFeeCreditAttributes
	require.NoError(t, res.Proofs.CloseFC.TxRecord.TransactionOrder.UnmarshalAttributes(&attr))
	require.EqualValues(t, unlockedBill.ID, attr.TargetUnitID)
}

func TestReclaimFeeCredit_TokensPartitionOK(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)

	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewBill(100000000, 2)),
		testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 1e8, Counter: 111})),
	)
	db := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, db, moneyClient, logger.New(t))

	// reclaim fee credit
	res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, res.Proofs)
	require.NotNil(t, res.Proofs.Lock)
	require.NotNil(t, res.Proofs.CloseFC)
	require.NotNil(t, res.Proofs.ReclaimFC)
}

func TestAddAndReclaimWithInsufficientCredit(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)

	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewBill(100000002, 2)),
		testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 2, Counter: 111})),
	)
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

	_, err = feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 2})
	require.ErrorIs(t, err, ErrMinimumFeeAmount)

	_, err = feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
	require.ErrorIs(t, err, ErrMinimumFeeAmount)
}

func TestAddWithInsufficientBalance(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewBill(10, 2)),
		testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 2, Counter: 111})),
	)
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

	_, err = feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
	require.ErrorIs(t, err, ErrInsufficientBalance)
}

func TestAddWithInsufficientBalanceInSmallBills(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewBill(1, 1)),
		testutil.WithOwnerBill(testutil.NewBill(2, 2)),
		testutil.WithOwnerBill(testutil.NewBill(1, 3)),
		testutil.WithOwnerBill(testutil.NewBill(2, 4)),
		testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 2, Counter: 111})),
	)
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

	_, err = feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 40})
	require.ErrorIs(t, err, ErrInsufficientBalance)
}

func TestAddFeeCredit_FeeCreditRecordIsLocked(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)

	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewBill(100, 1)),
		testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 2, Counter: 111, Locked: wallet.LockReasonManual})),
	)
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

	// add fees
	addRes, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 40})
	require.ErrorContains(t, err, "fee credit record is locked")
	require.Nil(t, addRes)

	// reclaim fees
	recRes, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
	require.ErrorContains(t, err, "fee credit record is locked")
	require.Nil(t, recRes)
}

func TestAddFeeCredit_LockingDisabled(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewBill(100, 1)),
		testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 100, Counter: 111})),
	)
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

	// add fees
	res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 40, DisableLocking: true})
	require.NoError(t, err, "fee credit record is locked")
	require.NotNil(t, res)
	require.Len(t, res.Proofs, 1)
	require.Nil(t, res.Proofs[0].LockFC)
	require.NotNil(t, res.Proofs[0].TransferFC)
	require.NotNil(t, res.Proofs[0].AddFC)
}

func TestReclaimFeeCredit_LockingDisabled(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)

	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewBill(100000001, 1)),
		testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 100, Counter: 111})),
	)
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

	// verify that lock tx is not send
	res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{DisableLocking: true})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.NotNil(t, res.Proofs)
	require.Nil(t, res.Proofs.Lock)
	require.NotNil(t, res.Proofs.CloseFC)
	require.NotNil(t, res.Proofs.ReclaimFC)
}

/*
Fee manager contains LockFC context, test that fee manager:
1. waits for confirmation
2. if confirmed => send lockFC
3. if timed out => create new lockFC
*/
func TestAddFeeCredit_ExistingLockFC(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	feeManagerDB := createFeeManagerDB(t)
	fcrCounter := uint64(1)
	fcr := sdktypes.FeeCreditRecord{
		SystemID: money.DefaultSystemID,
		ID:       money.NewFeeCreditRecordID(nil, []byte{1}),
		Counter:  &fcrCounter,
	}
	lockFCTx, err := fcr.Lock(wallet.LockReasonManual)
	require.NoError(t, err)
	lockFCRecord := &types.TransactionRecord{
		TransactionOrder: lockFCTx,
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	lockFCTxHash := lockFCRecord.TransactionOrder.Hash(crypto.SHA256)
	lockFCProof := &sdktypes.Proof{TxRecord: lockFCRecord, TxProof: &types.TxProof{}}

	targetBill := testutil.NewBill(0, 200)

	t.Run("lockFC confirmed => send follow-up transactions", func(t *testing.T) {
		// create fee context
		err = feeManagerDB.SetAddFeeContext(accountKey.PubKey, &AddFeeCreditCtx{
			TargetPartitionID: targetBill.SystemID,
			FeeCreditRecordID: []byte{1},
			TargetBillID:      targetBill.ID,
			TargetBillCounter: targetBill.Counter,
			TargetAmount:      50,
			LockFCTx:          lockFCRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx confirmed on node
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithTxProof(lockFCTxHash, lockFCProof),
			testutil.WithOwnerBill(targetBill),
		)

		// when fees are added
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)

		// then follow-up transactions are sent
		require.NotNil(t, res)
		require.Len(t, res.Proofs, 1)
		proofs := res.Proofs[0]
		require.NotNil(t, proofs.LockFC)
		require.NotNil(t, proofs.TransferFC)
		require.NotNil(t, proofs.AddFC)

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})

	t.Run("lockFC timed out => create new lockFC and send follow-up transactions", func(t *testing.T) {
		// create fee context
		err = feeManagerDB.SetAddFeeContext(accountKey.PubKey, &AddFeeCreditCtx{
			TargetPartitionID: targetBill.SystemID,
			FeeCreditRecordID: []byte{1},
			TargetBillID:      targetBill.ID,
			TargetBillCounter: targetBill.Counter,
			LockFCTx:          lockFCRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx timed out
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithRoundNumber(lockFCRecord.TransactionOrder.Timeout()+10),
			testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 100, Counter: 111})),
			testutil.WithOwnerBill(targetBill),
		)

		// when fees are added
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Len(t, res.Proofs, 1)
		proofs := res.Proofs[0]
		require.NotNil(t, proofs.LockFC)
		require.NotNil(t, proofs.TransferFC)
		require.NotNil(t, proofs.AddFC)

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})
}

/*
Fee manager contains TransferFC context, test that fee manager:
1. waits for confirmation
2. if confirmed => send addFC using the confirmed transferFC
3. if timed out and unit still valid => create new transferFC
4. if timed out and unit no longer valid => return error, unlock units
*/
func TestAddFeeCredit_ExistingTransferFC(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	feeManagerDB := createFeeManagerDB(t)

	targetBill := testutil.NewBill(50, 200)
	transferFCTx, err := targetBill.Transfer(nil)
	require.NoError(t, err)
	transferFCRecord := &types.TransactionRecord{
		TransactionOrder: transferFCTx,
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	transferFCTxHash := transferFCRecord.TransactionOrder.Hash(crypto.SHA256)
	transferFCProof := &sdktypes.Proof{TxRecord: transferFCRecord, TxProof: &types.TxProof{}}

	t.Run("transferFC confirmed => send addFC using the confirmed transferFC", func(t *testing.T) {
		// create fee context
		err = feeManagerDB.SetAddFeeContext(accountKey.PubKey, &AddFeeCreditCtx{
			TargetPartitionID: targetBill.SystemID,
			FeeCreditRecordID: []byte{1},
			TargetBillID:      targetBill.ID,
			TargetBillCounter: targetBill.Counter,
			TransferFCTx:      transferFCRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx confirmed on node
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithTxProof(transferFCTxHash, transferFCProof),
		)

		// when fees are added
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)

		// then addFC tx must be sent using the confirmed transferFC
		require.NotNil(t, res)
		require.Len(t, res.Proofs, 1)
		proofs := res.Proofs[0]
		require.NotNil(t, proofs.TransferFC)
		require.NotNil(t, proofs.AddFC)

		sentAddFCAttr := &fc.AddFeeCreditAttributes{}
		err = proofs.AddFC.TxRecord.TransactionOrder.UnmarshalAttributes(sentAddFCAttr)
		require.NoError(t, err)
		require.Equal(t, transferFCRecord, sentAddFCAttr.FeeCreditTransfer)

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})

	t.Run("transferFC timed out => create new transferFC", func(t *testing.T) {
		// create fee context
		err = feeManagerDB.SetAddFeeContext(accountKey.PubKey, &AddFeeCreditCtx{
			TargetPartitionID: moneySystemID,
			FeeCreditRecordID: []byte{1},
			TargetBillID:      targetBill.ID,
			TargetBillCounter: targetBill.Counter,
			TransferFCTx:      transferFCRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx timed out and the same bill used for transferFC is still valid
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithRoundNumber(transferFCRecord.TransactionOrder.Timeout()+10),
			testutil.WithOwnerBill(targetBill),
		)

		// when fees are added
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Len(t, res.Proofs, 1)
		proofs := res.Proofs[0]
		require.NotNil(t, proofs.TransferFC)
		require.NotNil(t, proofs.AddFC)

		// then new transferFC must be sent (same id, new timeout)
		require.EqualValues(t, transferFCRecord.TransactionOrder.UnitID(), proofs.TransferFC.TxRecord.TransactionOrder.UnitID())
		require.EqualValues(t, moneyClient.RoundNumber+10, proofs.TransferFC.TxRecord.TransactionOrder.Timeout())

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})

	t.Run("transferFC timed out and target unit no longer valid => return error", func(t *testing.T) {
		// create fee context
		err = feeManagerDB.SetAddFeeContext(accountKey.PubKey, &AddFeeCreditCtx{
			TargetPartitionID: targetBill.SystemID,
			FeeCreditRecordID: []byte{1},
			TargetBillID:      targetBill.ID,
			TargetBillCounter: targetBill.Counter,
			TransferFCTx:      transferFCRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx timed out and transferFC unit no longer exists
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithRoundNumber(transferFCRecord.TransactionOrder.Timeout() + 10),
		)

		// when fees are added then error must be returned
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.Errorf(t, err, "transferFC target unit is no longer valid")
		require.Nil(t, res)

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})
}

/*
Fee manager contains AddFC ctx, test that fee manager:
1. waits for confirmation
2. if confirmed => send addFC using the confirmed transferFC
3. if timed out and transferFC still usable => create new addFC
3. if timed out and transferFC no longer usable => return money lost error
*/
func TestAddFeeCredit_ExistingAddFC(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	feeManagerDB := createFeeManagerDB(t)

	targetBill := testutil.NewBill(50, 200)

	fcrCounter := uint64(1)
	fcr := &sdktypes.FeeCreditRecord{
		SystemID: money.DefaultSystemID,
		ID:       money.NewFeeCreditRecordID(nil, []byte{1}),
		Counter:  &fcrCounter,
	}

	transFCTx, err := targetBill.TransferToFeeCredit(fcr, 5, 10)
	transFCRecord := &types.TransactionRecord{
		TransactionOrder: transFCTx,
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	transFCProof := &sdktypes.Proof{
		TxRecord: transFCRecord,
		TxProof: &types.TxProof{},
	}

	addFCTx, err := fcr.AddFeeCredit(nil, transFCProof,
		sdktypes.WithTimeout(5),
		sdktypes.WithMaxFee(2))
	require.NoError(t, err)
	addFCAttr := fc.AddFeeCreditAttributes{}
	require.NoError(t, addFCTx.Payload.UnmarshalAttributes(&addFCAttr))
	addFCRecord := &types.TransactionRecord{
		TransactionOrder: addFCTx,
		ServerMetadata: &types.ServerMetadata{ActualFee: 1},
	}
	addFCTxHash := addFCRecord.TransactionOrder.Hash(crypto.SHA256)
	addFCProof := &sdktypes.Proof{TxRecord: addFCRecord, TxProof: &types.TxProof{}}

	t.Run("addFC confirmed => return no error (and optionally the fee txs)", func(t *testing.T) {
		// create fee context
		err := feeManagerDB.SetAddFeeContext(accountKey.PubKey, &AddFeeCreditCtx{
			TargetPartitionID: moneySystemID,
			TargetBillID:      targetBill.ID,
			TargetBillCounter: targetBill.Counter,
			TransferFCTx:      addFCAttr.FeeCreditTransfer.TransactionOrder,
			TransferFCProof:   &sdktypes.Proof{TxRecord: addFCAttr.FeeCreditTransfer, TxProof: addFCAttr.FeeCreditTransferProof},
			AddFCTx:           addFCRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx confirmed on node
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithTxProof(addFCTxHash, addFCProof),
		)
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

		// when fees are added then addFC proof must be returned
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, addFCProof, res.Proofs[0].AddFC)

		// and fee context must be cleared
		lockedBill, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})

	t.Run("addFC timed out => create new addFC", func(t *testing.T) {
		// create fee context
		err := feeManagerDB.SetAddFeeContext(accountKey.PubKey, &AddFeeCreditCtx{
			TargetPartitionID: moneySystemID,
			TargetBillID:      targetBill.ID,
			TargetBillCounter: targetBill.Counter,
			TransferFCTx:      addFCAttr.FeeCreditTransfer.TransactionOrder,
			TransferFCProof:   &sdktypes.Proof{TxRecord: addFCAttr.FeeCreditTransfer, TxProof: addFCAttr.FeeCreditTransferProof},
			AddFCTx:           addFCRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx timed out
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithRoundNumber(addFCRecord.TransactionOrder.Timeout() + 1),
		)
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

		// when fees are added
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.Proofs[0].AddFC)

		// and fee context must be cleared
		lockedBill, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})

	t.Run("addFC timed out and transferFC no longer usable => return money lost error", func(t *testing.T) {
		// create fee context
		err := feeManagerDB.SetAddFeeContext(accountKey.PubKey, &AddFeeCreditCtx{
			TargetPartitionID: moneySystemID,
			TargetBillID:      targetBill.ID,
			TargetBillCounter: targetBill.Counter,
			TransferFCTx:      addFCAttr.FeeCreditTransfer.TransactionOrder,
			TransferFCProof:   &sdktypes.Proof{TxRecord: addFCAttr.FeeCreditTransfer, TxProof: addFCAttr.FeeCreditTransferProof},
			AddFCTx:           addFCRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx timed out
		// round number > latest addition time
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithRoundNumber(11),
		)
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

		// when fees are added
		// then money lost error must be returned
		res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 50})
		require.ErrorContains(t, err, "addFC timed out and transferFC latestAdditionTime exceeded, the target bill is no longer usable")
		require.Nil(t, res)
	})
}

/*
Fee manager contains Lock ctx, test that fee manager:
1. waits for confirmation
2. if confirmed => send lock tx
3. if timed out => create new lock tx
*/
func TestReclaimFeeCredit_ExistingLock(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	feeManagerDB := createFeeManagerDB(t)

	lockTxRecord := &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{},
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	lockTxProof := &sdktypes.Proof{TxRecord: lockTxRecord, TxProof: &types.TxProof{}}
	lockTxHash := lockTxRecord.TransactionOrder.Hash(crypto.SHA256)
	targetBill := testutil.NewBill(50, 200)

	t.Run("lock tx confirmed => update target bill counter and send follow-up transactions", func(t *testing.T) {
		// create fee context
		err = feeManagerDB.SetReclaimFeeContext(accountKey.PubKey, &ReclaimFeeCreditCtx{
			TargetPartitionID: moneySystemID,
			TargetBillID:      targetBill.ID,
			TargetBillCounter: targetBill.Counter,
			LockTx:            lockTxRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock locked fee credit record
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 100, Counter: 0, Locked: wallet.LockReasonReclaimFees})),
			testutil.WithTxProof(lockTxHash, lockTxProof),
			testutil.WithOwnerBill(targetBill),
		)
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

		// when fees are reclaimed
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.NoError(t, err)

		// then follow-up transactions are sent
		require.NotNil(t, res)
		require.NotNil(t, res.Proofs)
		require.NotNil(t, res.Proofs.Lock)
		require.NotNil(t, res.Proofs.CloseFC)
		require.NotNil(t, res.Proofs.ReclaimFC)

		// with updated target unit counter
		var attr *fc.CloseFeeCreditAttributes
		err = res.Proofs.CloseFC.TxRecord.TransactionOrder.UnmarshalAttributes(&attr)
		require.NoError(t, err)
		require.EqualValues(t, 201, attr.TargetUnitCounter)

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})

	t.Run("lock tx timed out => create new lock tx and send follow-up transactions", func(t *testing.T) {
		// create fee context
		err = feeManagerDB.SetReclaimFeeContext(accountKey.PubKey, &ReclaimFeeCreditCtx{
			TargetPartitionID: moneySystemID,
			TargetBillID:      targetBill.ID,
			TargetBillCounter: targetBill.Counter,
			LockTx:            lockTxRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx timed out
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 100, Counter: 200})),
			testutil.WithRoundNumber(lockTxRecord.TransactionOrder.Timeout()+10),
			testutil.WithOwnerBill(targetBill),
		)
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

		// when fees are reclaimed
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.Proofs)
		require.NotNil(t, res.Proofs.Lock)
		require.NotNil(t, res.Proofs.CloseFC)
		require.NotNil(t, res.Proofs.ReclaimFC)

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})
}

/*
Fee manager contains CloseFC ctx, test that fee manager:
1. waits for confirmation
2. if confirmed => send reclaimFC using the confirmed closeFC
3. if timed out => create new closeFC and reclaimFC
*/
func TestReclaimFeeCredit_ExistingCloseFC(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	feeManagerDB := createFeeManagerDB(t)

	fcrCounter := uint64(1)
	fcr := sdktypes.FeeCreditRecord{
		SystemID: money.DefaultSystemID,
		ID:       money.NewFeeCreditRecordID(nil, []byte{1}),
		Counter:  &fcrCounter,
	}
	targetBill := testutil.NewBill(50, 200)

	closeFCTx, err := fcr.CloseFeeCredit(targetBill.ID, targetBill.Counter,
		sdktypes.WithTimeout(5),
		sdktypes.WithMaxFee(2))
	require.NoError(t, err)
	closeFCAttr := fc.CloseFeeCreditAttributes{}
	require.NoError(t, closeFCTx.Payload.UnmarshalAttributes(&closeFCAttr))
	closeFCRecord := &types.TransactionRecord{
		TransactionOrder: closeFCTx,
		ServerMetadata: &types.ServerMetadata{ActualFee: 1},
	}
	closeFCTxHash := closeFCRecord.TransactionOrder.Hash(crypto.SHA256)
	closeFCProof := &sdktypes.Proof{TxRecord: closeFCRecord, TxProof: &types.TxProof{}}

	t.Run("closeFC confirmed => send reclaimFC using the confirmed closeFC", func(t *testing.T) {
		// create fee context
		err := feeManagerDB.SetReclaimFeeContext(accountKey.PubKey, &ReclaimFeeCreditCtx{
			TargetPartitionID: moneySystemID,
			TargetBillID:      targetBill.ID,
			TargetBillCounter: targetBill.Counter,
			CloseFCTx:         closeFCRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx confirmed on node
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithTxProof(closeFCTxHash, closeFCProof),
			testutil.WithOwnerBill(targetBill),
		)
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

		// when fees are reclaimed
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.NoError(t, err)

		// then reclaimFC tx must be sent using the confirmed closeFC
		require.NotNil(t, res)
		require.NotNil(t, res.Proofs)
		require.NotNil(t, res.Proofs.CloseFC)
		require.NotNil(t, res.Proofs.ReclaimFC)

		sentReclaimFCAttr := &fc.ReclaimFeeCreditAttributes{}
		err = res.Proofs.ReclaimFC.TxRecord.TransactionOrder.UnmarshalAttributes(sentReclaimFCAttr)
		require.NoError(t, err)
		require.Equal(t, closeFCRecord, sentReclaimFCAttr.CloseFeeCreditTransfer)

		// and fee context must be cleared
		lockedBill, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, lockedBill)
	})

	t.Run("closeFC timed out => create new closeFC", func(t *testing.T) {
		// create fee context
		err := feeManagerDB.SetReclaimFeeContext(accountKey.PubKey, &ReclaimFeeCreditCtx{
			TargetPartitionID: moneySystemID,
			TargetBillID:      targetBill.ID,
			TargetBillCounter: targetBill.Counter,
			CloseFCTx:         closeFCRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx timed out and add bill to wallet
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithRoundNumber(closeFCRecord.TransactionOrder.Timeout()+10),
			testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 1e8, Counter: 100})),
			testutil.WithOwnerBill(targetBill),
		)
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

		// when fees are reclaimed
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.Proofs)
		require.NotNil(t, res.Proofs.CloseFC)
		require.NotNil(t, res.Proofs.ReclaimFC)

		// then new closeFC must be sent (same id but timeout changed)
		require.Equal(t, moneyClient.RoundNumber+10, res.Proofs.CloseFC.TxRecord.TransactionOrder.Timeout())

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})
}

/*
Fee manager contains ReclaimFC ctx, test that fee manager:
1. waits for confirmation
2. if confirmed => ok
3. if partially timed out => create new tx (target bill still usable)
4. if fully timed out => return money lost error (target bill has been used)
*/
func TestReclaimFeeCredit_ExistingReclaimFC(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	feeManagerDB := createFeeManagerDB(t)

	targetBill := testutil.NewBill(50, 200)
	closeFCProof := &sdktypes.Proof{
		TxRecord: &types.TransactionRecord{},
		TxProof: &types.TxProof{},
	}
	reclaimFCTx, err := targetBill.ReclaimFromFeeCredit(closeFCProof, sdktypes.WithTimeout(5), sdktypes.WithMaxFee(2))
	require.NoError(t, err)
	reclaimFCAttr := fc.ReclaimFeeCreditAttributes{}
	require.NoError(t, reclaimFCTx.Payload.UnmarshalAttributes(&reclaimFCAttr))
	reclaimFCRecord := &types.TransactionRecord{
		TransactionOrder: reclaimFCTx,
		ServerMetadata: &types.ServerMetadata{ActualFee: 1},
	}
	reclaimFCTxHash := reclaimFCRecord.TransactionOrder.Hash(crypto.SHA256)
	reclaimFCProof := &sdktypes.Proof{TxRecord: reclaimFCRecord, TxProof: &types.TxProof{}}

	t.Run("reclaimFC confirmed => return proofs", func(t *testing.T) {
		// create fee context
		err := feeManagerDB.SetReclaimFeeContext(accountKey.PubKey, &ReclaimFeeCreditCtx{
			TargetPartitionID: moneySystemID,
			TargetBillID:      reclaimFCTx.UnitID(),
			TargetBillCounter: 200,

			CloseFCTx:         reclaimFCAttr.CloseFeeCreditTransfer.TransactionOrder,
			CloseFCProof:      &sdktypes.Proof{TxRecord: reclaimFCAttr.CloseFeeCreditTransfer, TxProof: reclaimFCAttr.CloseFeeCreditProof},
			ReclaimFCTx:       reclaimFCProof.TxRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx confirmed on node
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithTxProof(reclaimFCTxHash, reclaimFCProof),
		)
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

		// when fees are reclaimed
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.NoError(t, err)

		// then reclaimFC proof must be returned
		require.NotNil(t, res)
		require.NotNil(t, res.Proofs)
		require.Equal(t, reclaimFCProof, res.Proofs.ReclaimFC)

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})

	t.Run("reclaimFC timed out => create new reclaimFC", func(t *testing.T) {
		// create fee context
		err := feeManagerDB.SetReclaimFeeContext(accountKey.PubKey, &ReclaimFeeCreditCtx{
			TargetPartitionID: moneySystemID,
			TargetBillID:      reclaimFCTx.UnitID(),
			TargetBillCounter: 200,

			CloseFCTx:         reclaimFCAttr.CloseFeeCreditTransfer.TransactionOrder,
			CloseFCProof:      &sdktypes.Proof{TxRecord: reclaimFCAttr.CloseFeeCreditTransfer, TxProof: reclaimFCAttr.CloseFeeCreditProof},
			ReclaimFCTx:       reclaimFCProof.TxRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx timed out and return locked bill
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithRoundNumber(reclaimFCTx.Timeout()+1),
			testutil.WithOwnerBill(targetBill),
		)
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

		// when fees are reclaimed
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.Proofs.ReclaimFC)

		// then new reclaimFC must be sent using the existing closeFC
		// new reclaimFC has new tx timeout = round number + tx timeout
		require.EqualValues(t, moneyClient.RoundNumber+txTimeoutBlockCount, res.Proofs.ReclaimFC.TxRecord.TransactionOrder.Timeout())

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})

	t.Run("reclaimFC timed out and closeFC no longer usable => return money lost error", func(t *testing.T) {
		// create fee context
		err := feeManagerDB.SetReclaimFeeContext(accountKey.PubKey, &ReclaimFeeCreditCtx{
			TargetPartitionID: moneySystemID,
			TargetBillID:      reclaimFCTx.UnitID(),
			TargetBillCounter: 200,

			CloseFCTx:         reclaimFCAttr.CloseFeeCreditTransfer.TransactionOrder,
			CloseFCProof:      &sdktypes.Proof{TxRecord: reclaimFCAttr.CloseFeeCreditTransfer, TxProof: reclaimFCAttr.CloseFeeCreditProof},
			ReclaimFCTx:       reclaimFCProof.TxRecord.TransactionOrder,
		})
		require.NoError(t, err)

		// mock tx timed out and no bills are available
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithRoundNumber(11),
		)
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

		// when fees are reclaimed
		// then money lost error must be returned
		res, err := feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
		require.ErrorContains(t, err, "reclaimFC target bill is no longer usable")
		require.Nil(t, res)

		// and fee context must be cleared
		feeCtx, err := feeManagerDB.GetAddFeeContext(accountKey.PubKey)
		require.NoError(t, err)
		require.Nil(t, feeCtx)
	})
}

func TestLockFeeCredit(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	feeManagerDB := createFeeManagerDB(t)

	t.Run("ok", func(t *testing.T) {
		// fcb exists
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 21, Counter: 100})),
		)
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

		// when fee credit is successfully locked then lockFC proof should be returned
		res, err := feeManager.LockFeeCredit(context.Background(), LockFeeCreditCmd{LockStatus: wallet.LockReasonManual})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, fc.PayloadTypeLockFeeCredit, res.TxRecord.TransactionOrder.PayloadType())
	})

	t.Run("fcb already locked", func(t *testing.T) {
		// fcb already locked
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 21, Counter: 100, Locked: wallet.LockReasonManual})),
		)
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

		// when fees are added
		res, err := feeManager.LockFeeCredit(context.Background(), LockFeeCreditCmd{LockStatus: wallet.LockReasonManual})
		require.ErrorContains(t, err, "fee credit record is already locked")
		require.Nil(t, res)
	})

	t.Run("no fee credit", func(t *testing.T) {
		// no fcb in wallet
		moneyClient := testutil.NewRpcClientMock()
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

		// when fees are added
		res, err := feeManager.LockFeeCredit(context.Background(), LockFeeCreditCmd{LockStatus: wallet.LockReasonManual})
		require.ErrorContains(t, err, "not enough fee credit in wallet")
		require.Nil(t, res)
	})

	t.Run("not enough fee credit", func(t *testing.T) {
		// no fcb in wallet
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 1, Counter: 100})),
		)
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

		// when fees are added
		res, err := feeManager.LockFeeCredit(context.Background(), LockFeeCreditCmd{LockStatus: wallet.LockReasonManual})
		require.ErrorContains(t, err, "not enough fee credit in wallet")
		require.Nil(t, res)
	})
}

func TestUnlockFeeCredit(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	feeManagerDB := createFeeManagerDB(t)

	t.Run("ok", func(t *testing.T) {
		// locked fcb exists
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 3, Counter: 100, Locked: wallet.LockReasonManual})),
		)
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

		// when fee credit is successfully unlocked then unlockFC proof should be returned
		res, err := feeManager.UnlockFeeCredit(context.Background(), UnlockFeeCreditCmd{})
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Equal(t, fc.PayloadTypeUnlockFeeCredit, res.TxRecord.TransactionOrder.PayloadType())
	})

	t.Run("fcb already unlocked", func(t *testing.T) {
		// mock fcb already unlocked
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 3, Counter: 100})),
		)
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

		// when fees are added
		res, err := feeManager.UnlockFeeCredit(context.Background(), UnlockFeeCreditCmd{})
		require.ErrorContains(t, err, "fee credit record is already unlocked")
		require.Nil(t, res)
	})

	t.Run("no fee credit in wallet", func(t *testing.T) {
		// mock fcb already locked
		moneyClient := testutil.NewRpcClientMock(
			testutil.WithOwnerFeeCreditRecord(newMoneyFCR(accountKey, &fc.FeeCreditRecord{Balance: 0, Counter: 100})),
		)
		feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

		// when fees are added
		res, err := feeManager.UnlockFeeCredit(context.Background(), UnlockFeeCreditCmd{})
		require.ErrorContains(t, err, "not enough fee credit in wallet")
		require.Nil(t, res)
	})
}

/*
Wallet has a single bill but no fee credit record
*/
func TestNonExistingFeeCreditRecord(t *testing.T) {
	// create fee manager
	am := newAccountManager(t)
	moneyClient := testutil.NewRpcClientMock(
		testutil.WithOwnerBill(testutil.NewBill(100000000, 1)),
	)
	feeManagerDB := createFeeManagerDB(t)
	feeManager := newMoneyPartitionFeeManager(am, feeManagerDB, moneyClient, logger.New(t))

	res, err := feeManager.AddFeeCredit(context.Background(), AddFeeCmd{Amount: 100000000})
	require.NoError(t, err)
	require.NotNil(t, res)
	require.Len(t, res.Proofs, 1)
	proofs := res.Proofs[0]
	// no lockFC because fcr does not exist
	require.Nil(t, proofs.LockFC)
	require.NotNil(t, proofs.TransferFC)
	require.NotNil(t, proofs.AddFC)

	_, err = feeManager.ReclaimFeeCredit(context.Background(), ReclaimFeeCmd{})
	require.ErrorContains(t, err, "fee credit record not found")

	_, err = feeManager.LockFeeCredit(context.Background(), LockFeeCreditCmd{})
	require.ErrorContains(t, err, "not enough fee credit in wallet")

	_, err = feeManager.UnlockFeeCredit(context.Background(), UnlockFeeCreditCmd{})
	require.ErrorContains(t, err, "not enough fee credit in wallet")
}

func newMoneyPartitionFeeManager(am account.Manager, db FeeManagerDB, moneyClient sdktypes.MoneyPartitionClient, log *slog.Logger) *FeeManager {
	return NewFeeManager(am, db, moneySystemID, moneyClient, testFeeCreditRecordIDFromPublicKey, moneySystemID, moneyClient, testFeeCreditRecordIDFromPublicKey, maxFee, log)
}

func newTokensPartitionFeeManager(am account.Manager, db FeeManagerDB, moneyClient sdktypes.MoneyPartitionClient, tokensClient sdktypes.PartitionClient, log *slog.Logger) *FeeManager {
	return NewFeeManager(am, db, moneySystemID, moneyClient, testFeeCreditRecordIDFromPublicKey, tokensSystemID, tokensClient, testFeeCreditRecordIDFromPublicKey, maxFee, log)
}

func newAccountManager(t *testing.T) account.Manager {
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	t.Cleanup(am.Close)
	err = am.CreateKeys("dinosaur simple verify deliver bless ridge monkey design venue six problem lucky")
	require.NoError(t, err)
	return am
}

func createFeeManagerDB(t *testing.T) *BoltStore {
	feeManagerDB, err := NewFeeManagerDB(t.TempDir())
	require.NoError(t, err)
	return feeManagerDB
}

func testFeeCreditRecordIDFromPublicKey(shardPart, pubKey []byte, latestAdditionTime uint64) types.UnitID {
	return money.NewFeeCreditRecordIDFromPublicKey(shardPart, pubKey, latestAdditionTime)
}

func newMoneyFCR(accountKey *account.AccountKey, fcr *fc.FeeCreditRecord) *sdktypes.FeeCreditRecord {
	return testutil.NewMoneyFCR(accountKey.PubKeyHash.Sha256, fcr.Balance, fcr.Locked, fcr.Counter)
}
