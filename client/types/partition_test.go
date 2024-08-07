package types

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/stretchr/testify/require"
)

func TestFeeCreditRecordAddFeeCredit(t *testing.T) {
	fcrCounter := uint64(1)
	fcr := &FeeCreditRecord{
		SystemID: 2,
		ID:       money.NewFeeCreditRecordID(nil, []byte{3}),
		Counter:  &fcrCounter,
	}

	ownerPredicate := []byte{4}
	transFCProof := &Proof{
		TxRecord: &types.TransactionRecord{},
		TxProof: &types.TxProof{},
	}

	tx, err := fcr.AddFeeCredit(ownerPredicate, transFCProof)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.PayloadType(), fc.PayloadTypeAddFeeCredit)
	require.Equal(t, fcr.SystemID, tx.SystemID())
	require.Equal(t, fcr.ID, tx.UnitID())

	attr := &fc.AddFeeCreditAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, transFCProof.TxRecord, attr.FeeCreditTransfer)
	require.Equal(t, transFCProof.TxProof, attr.FeeCreditTransferProof)
	require.Equal(t, ownerPredicate, attr.FeeCreditOwnerCondition)
}

func TestFeeCreditRecordCloseFeeCredit(t *testing.T) {
	fcrCounter := uint64(1)
	fcr := &FeeCreditRecord{
		SystemID: 1,
		ID:       money.NewFeeCreditRecordID(nil, []byte{2}),
		Balance:  3,
		Counter:  &fcrCounter,
	}

	targetBillID := money.NewBillID(nil, []byte{4})
	targetBillCounter := uint64(5)

	tx, err := fcr.CloseFeeCredit(targetBillID, targetBillCounter)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.PayloadType(), fc.PayloadTypeCloseFeeCredit)
	require.Equal(t, fcr.SystemID, tx.SystemID())
	require.Equal(t, fcr.ID, tx.UnitID())

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
		SystemID: 2,
		ID:       money.NewFeeCreditRecordID(nil, []byte{3}),
		Counter:  &fcrCounter,
	}
	lockStatus := uint64(4)
	tx, err := fcr.Lock(lockStatus)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.PayloadType(), fc.PayloadTypeLockFeeCredit)
	require.Equal(t, fcr.SystemID, tx.SystemID())
	require.Equal(t, fcr.ID, tx.UnitID())

	attr := &fc.LockFeeCreditAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, lockStatus, attr.LockStatus)
	require.Equal(t, *fcr.Counter, attr.Counter)	
}

func TestFeeCreditRecordUnlock(t *testing.T) {
	fcrCounter := uint64(1)
	fcr := &FeeCreditRecord{
		SystemID: 2,
		ID:       money.NewFeeCreditRecordID(nil, []byte{3}),
		Counter:  &fcrCounter,
	}
	tx, err := fcr.Unlock()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.PayloadType(), fc.PayloadTypeUnlockFeeCredit)
	require.Equal(t, fcr.SystemID, tx.SystemID())
	require.Equal(t, fcr.ID, tx.UnitID())

	attr := &fc.UnlockFeeCreditAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, *fcr.Counter, attr.Counter)	
}
