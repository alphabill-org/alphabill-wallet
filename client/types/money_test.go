package types

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/stretchr/testify/require"
)

func TestBillTransfer(t *testing.T) {
	b := &Bill{
		NetworkID:   types.NetworkLocal,
		PartitionID: money.DefaultPartitionID,
		ID:          money.NewBillID(nil, []byte{1}),
		Value:       2,
		Counter:     3,
	}
	refNo := []byte("refNo")
	timeout := uint64(4)
	newOwnerPredicate := []byte{5}
	fcrID := money.NewFeeCreditRecordID(nil, []byte{6})
	maxFee := uint64(7)
	tx, err := b.Transfer(newOwnerPredicate,
		WithFeeCreditRecordID(fcrID),
		WithTimeout(timeout),
		WithReferenceNumber(refNo),
		WithMaxFee(maxFee),
	)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, money.TransactionTypeTransfer)
	require.Equal(t, b.PartitionID, tx.GetPartitionID())
	require.Equal(t, b.ID, tx.GetUnitID())
	require.Equal(t, timeout, tx.Timeout())
	require.EqualValues(t, refNo, tx.Payload.ClientMetadata.ReferenceNumber)
	require.Equal(t, maxFee, tx.MaxFee())
	require.Nil(t, tx.AuthProof)
	require.Nil(t, tx.FeeProof)

	attr := &money.TransferAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, b.Value, attr.TargetValue)
	require.EqualValues(t, newOwnerPredicate, attr.NewOwnerPredicate)
	require.EqualValues(t, b.Counter, attr.Counter)
}

func TestSplitTransactionAmount(t *testing.T) {
	receiverPubKeyHash := hash.Sum256([]byte{1})
	billID := money.NewBillID(nil, nil)
	b := &Bill{
		PartitionID: money.DefaultPartitionID,
		ID:          billID,
		Value:       500,
		Counter:     1234,
	}
	amount := uint64(150)

	targetUnits := []*money.TargetUnit{
		{
			OwnerPredicate: templates.NewP2pkh256BytesFromKeyHash(receiverPubKeyHash),
			Amount:         amount,
		},
	}
	tx, err := b.Split(targetUnits)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, money.TransactionTypeSplit)
	require.EqualValues(t, b.PartitionID, tx.GetPartitionID())
	require.EqualValues(t, billID, tx.GetUnitID())
	require.Nil(t, tx.AuthProof)

	attr := &money.SplitAttributes{}
	err = tx.UnmarshalAttributes(attr)
	require.NoError(t, err)
	require.Equal(t, amount, attr.TargetUnits[0].Amount)
	require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(receiverPubKeyHash), attr.TargetUnits[0].OwnerPredicate)
	require.EqualValues(t, b.Counter, attr.Counter)
}

func TestBillTransferToDustCollector(t *testing.T) {
	b := &Bill{
		NetworkID:   types.NetworkLocal,
		PartitionID: money.DefaultPartitionID,
		ID:          money.NewBillID(nil, []byte{1}),
		Value:       2,
		Counter:     3,
	}
	targetBill := &Bill{
		ID:      money.NewBillID(nil, []byte{4}),
		Counter: 5,
	}
	tx, err := b.TransferToDustCollector(targetBill)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, money.TransactionTypeTransDC)
	require.Equal(t, b.PartitionID, tx.GetPartitionID())
	require.Equal(t, b.ID, tx.GetUnitID())
	require.Nil(t, tx.StateUnlock)
	require.Nil(t, tx.FeeProof)

	attr := &money.TransferDCAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, b.Value, attr.Value)
	require.EqualValues(t, targetBill.ID, attr.TargetUnitID)
	require.Equal(t, targetBill.Counter, attr.TargetUnitCounter)
	require.EqualValues(t, b.Counter, attr.Counter)
}

func TestBillSwapWithDustCollector(t *testing.T) {
	db1 := &Bill{
		NetworkID:   types.NetworkLocal,
		PartitionID: money.DefaultPartitionID,
		ID:          money.NewBillID(nil, []byte{1}),
		Value:       2,
		Counter:     3,
	}
	db2 := &Bill{
		NetworkID:   types.NetworkLocal,
		PartitionID: money.DefaultPartitionID,
		ID:          money.NewBillID(nil, []byte{2}),
		Value:       3,
		Counter:     3,
	}
	targetBill := &Bill{
		NetworkID:   types.NetworkLocal,
		PartitionID: money.DefaultPartitionID,
		ID:          money.NewBillID(nil, []byte{3}),
		Counter:     4,
	}
	tx1, err := db1.TransferToDustCollector(targetBill)
	tx2, err := db2.TransferToDustCollector(targetBill)

	proofs := []*types.TxRecordProof{
		{
			TxRecord: &types.TransactionRecord{TransactionOrder: tx1},
			TxProof:  &types.TxProof{},
		},
		{
			TxRecord: &types.TransactionRecord{TransactionOrder: tx2},
			TxProof:  &types.TxProof{},
		},
	}
	tx3, err := targetBill.SwapWithDustCollector(proofs)
	require.NoError(t, err)
	require.NotNil(t, tx3)
	require.Equal(t, tx3.Type, money.TransactionTypeSwapDC)
	require.Equal(t, targetBill.PartitionID, tx3.GetPartitionID())
	require.Equal(t, targetBill.ID, tx3.GetUnitID())
	require.Nil(t, tx3.AuthProof)
	require.Nil(t, tx3.FeeProof)

	attr := &money.SwapDCAttributes{}
	require.NoError(t, tx3.UnmarshalAttributes(attr))
	require.Len(t, attr.DustTransferProofs, 2)
}

func TestBillTransferToFeeCredit(t *testing.T) {
	b := &Bill{
		NetworkID:   types.NetworkLocal,
		PartitionID: money.DefaultPartitionID,
		ID:          money.NewBillID(nil, []byte{1}),
		Value:       2,
		Counter:     3,
	}
	fcrCounter := uint64(4)
	fcr := &FeeCreditRecord{
		NetworkID:   types.NetworkLocal,
		PartitionID: 5,
		ID:          money.NewFeeCreditRecordID(nil, []byte{6}),
		Counter:     &fcrCounter,
	}
	amount := uint64(1)
	latestAdditionTime := uint64(7)
	tx, err := b.TransferToFeeCredit(fcr, amount, latestAdditionTime)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, fc.TransactionTypeTransferFeeCredit)
	require.Equal(t, b.PartitionID, tx.GetPartitionID())
	require.Equal(t, b.ID, tx.GetUnitID())

	attr := &fc.TransferFeeCreditAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, amount, attr.Amount)
	require.Equal(t, fcr.PartitionID, attr.TargetPartitionID)
	require.EqualValues(t, fcr.ID, attr.TargetRecordID)
	require.Equal(t, latestAdditionTime, attr.LatestAdditionTime)
	require.Equal(t, fcr.Counter, attr.TargetUnitCounter)
	require.Equal(t, b.Counter, attr.Counter)
}

func TestBillReclaimFromFeeCredit(t *testing.T) {
	b := &Bill{
		NetworkID:   types.NetworkLocal,
		PartitionID: money.DefaultPartitionID,
		ID:          money.NewBillID(nil, []byte{1}),
		Value:       2,
		Counter:     3,
	}
	closeFCProof := &types.TxRecordProof{
		TxRecord: &types.TransactionRecord{},
		TxProof:  &types.TxProof{Version: types.ABVersion(1)},
	}
	tx, err := b.ReclaimFromFeeCredit(closeFCProof)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, fc.TransactionTypeReclaimFeeCredit)
	require.Equal(t, b.PartitionID, tx.GetPartitionID())
	require.Equal(t, b.ID, tx.GetUnitID())

	attr := &fc.ReclaimFeeCreditAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, closeFCProof, attr.CloseFeeCreditProof)
}

func TestBillLock(t *testing.T) {
	b := &Bill{
		NetworkID:   types.NetworkLocal,
		PartitionID: money.DefaultPartitionID,
		ID:          money.NewBillID(nil, []byte{1}),
		Value:       2,
		Counter:     3,
	}
	lockStatus := uint64(4)
	tx, err := b.Lock(lockStatus)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, money.TransactionTypeLock)
	require.Equal(t, b.PartitionID, tx.GetPartitionID())
	require.Equal(t, b.ID, tx.GetUnitID())

	attr := &money.LockAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, lockStatus, attr.LockStatus)
	require.Equal(t, b.Counter, attr.Counter)
}

func TestBillUnlock(t *testing.T) {
	b := &Bill{
		NetworkID:   types.NetworkLocal,
		PartitionID: money.DefaultPartitionID,
		ID:          money.NewBillID(nil, []byte{1}),
		Value:       2,
		Counter:     3,
	}
	tx, err := b.Unlock()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, money.TransactionTypeUnlock)
	require.Equal(t, b.PartitionID, tx.GetPartitionID())
	require.Equal(t, b.ID, tx.GetUnitID())

	attr := &money.UnlockAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, b.Counter, attr.Counter)
}
