package types

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc/permissioned"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/stretchr/testify/require"
)

func TestFeeCreditRecordAddFeeCredit(t *testing.T) {
	fcrCounter := uint64(1)
	fcr := &FeeCreditRecord{
		NetworkID:   types.NetworkLocal,
		PartitionID: 2,
		ID:          money.NewFeeCreditRecordID(nil, []byte{3}),
		Counter:     &fcrCounter,
	}

	ownerPredicate := []byte{4}
	transFCProof := &types.TxRecordProof{
		TxRecord: &types.TransactionRecord{},
		TxProof:  &types.TxProof{Version: types.ABVersion(1)},
	}

	tx, err := fcr.AddFeeCredit(ownerPredicate, transFCProof)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, fc.TransactionTypeAddFeeCredit)
	require.Equal(t, fcr.PartitionID, tx.GetPartitionID())
	require.Equal(t, fcr.ID, tx.GetUnitID())

	attr := &fc.AddFeeCreditAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, transFCProof, attr.FeeCreditTransferProof)
	require.Equal(t, ownerPredicate, attr.FeeCreditOwnerPredicate)
}

func TestFeeCreditRecordCloseFeeCredit(t *testing.T) {
	fcrCounter := uint64(1)
	fcr := &FeeCreditRecord{
		NetworkID:   types.NetworkLocal,
		PartitionID: 1,
		ID:          money.NewFeeCreditRecordID(nil, []byte{2}),
		Balance:     3,
		Counter:     &fcrCounter,
	}

	targetBillID := money.NewBillID(nil, []byte{4})
	targetBillCounter := uint64(5)

	tx, err := fcr.CloseFeeCredit(targetBillID, targetBillCounter)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, fc.TransactionTypeCloseFeeCredit)
	require.Equal(t, fcr.PartitionID, tx.GetPartitionID())
	require.Equal(t, fcr.ID, tx.GetUnitID())

	attr := &fc.CloseFeeCreditAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, fcr.Balance, attr.Amount)
	require.EqualValues(t, targetBillID, attr.TargetUnitID)
	require.Equal(t, targetBillCounter, attr.TargetUnitCounter)
	require.Equal(t, *fcr.Counter, attr.Counter)
}

func TestFeeCreditRecordLock(t *testing.T) {
	fcrCounter := uint64(1)
	fcr := &FeeCreditRecord{
		NetworkID:   types.NetworkLocal,
		PartitionID: 2,
		ID:          money.NewFeeCreditRecordID(nil, []byte{3}),
		Counter:     &fcrCounter,
	}
	lockStatus := uint64(4)
	tx, err := fcr.Lock(lockStatus)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, fc.TransactionTypeLockFeeCredit)
	require.Equal(t, fcr.PartitionID, tx.GetPartitionID())
	require.Equal(t, fcr.ID, tx.GetUnitID())

	attr := &fc.LockFeeCreditAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, lockStatus, attr.LockStatus)
	require.Equal(t, *fcr.Counter, attr.Counter)
}

func TestFeeCreditRecordUnlock(t *testing.T) {
	fcrCounter := uint64(1)
	fcr := &FeeCreditRecord{
		NetworkID:   types.NetworkLocal,
		PartitionID: 2,
		ID:          money.NewFeeCreditRecordID(nil, []byte{3}),
		Counter:     &fcrCounter,
	}
	tx, err := fcr.Unlock()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, fc.TransactionTypeUnlockFeeCredit)
	require.Equal(t, fcr.PartitionID, tx.GetPartitionID())
	require.Equal(t, fcr.ID, tx.GetUnitID())

	attr := &fc.UnlockFeeCreditAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, *fcr.Counter, attr.Counter)
}

func TestFeeCreditRecordSetFeeCredit(t *testing.T) {
	fcrCounter := uint64(1)
	fcr := &FeeCreditRecord{
		NetworkID:   types.NetworkLocal,
		PartitionID: 2,
		ID:          money.NewFeeCreditRecordID(nil, []byte{3}),
		Counter:     &fcrCounter,
	}

	ownerPredicate := []byte{4}
	tx, err := fcr.SetFeeCredit(ownerPredicate, 5)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, permissioned.TransactionTypeSetFeeCredit)
	require.Equal(t, fcr.PartitionID, tx.GetPartitionID())
	require.Equal(t, fcr.ID, tx.GetUnitID())

	attr := &permissioned.SetFeeCreditAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, ownerPredicate, attr.OwnerPredicate)
	require.EqualValues(t, 5, attr.Amount)
	require.Equal(t, *fcr.Counter, *attr.Counter)
}

func TestFeeCreditRecordDeleteFeeCredit(t *testing.T) {
	fcrCounter := uint64(1)
	fcr := &FeeCreditRecord{
		NetworkID:   types.NetworkLocal,
		PartitionID: 2,
		ID:          money.NewFeeCreditRecordID(nil, []byte{3}),
		Counter:     &fcrCounter,
	}

	tx, err := fcr.DeleteFeeCredit()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, permissioned.TransactionTypeDeleteFeeCredit)
	require.Equal(t, fcr.PartitionID, tx.GetPartitionID())
	require.Equal(t, fcr.ID, tx.GetUnitID())

	attr := &permissioned.DeleteFeeCreditAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, *fcr.Counter, attr.Counter)
}
