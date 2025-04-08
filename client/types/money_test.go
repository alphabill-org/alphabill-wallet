package types

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	moneyid "github.com/alphabill-org/alphabill-go-base/testutils/money"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/txsystem/nop"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils"
)

func TestBillTransfer(t *testing.T) {
	b := &Bill{
		NetworkID:   types.NetworkLocal,
		PartitionID: money.DefaultPartitionID,
		ID:          moneyid.NewBillID(t),
		Value:       2,
		Counter:     3,
	}
	refNo := []byte("refNo")
	timeout := uint64(4)
	newOwnerPredicate := []byte{5}
	fcrID := moneyid.NewFeeCreditRecordID(t)
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
	receiverPubKeyHash := testutils.RandomBytes(32)
	billID := moneyid.NewBillID(t)
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
		ID:          moneyid.NewBillID(t),
		Value:       2,
		Counter:     3,
	}
	targetBill := &Bill{
		ID:      moneyid.NewBillID(t),
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
		ID:          moneyid.NewBillID(t),
		Value:       2,
		Counter:     3,
	}
	db2 := &Bill{
		NetworkID:   types.NetworkLocal,
		PartitionID: money.DefaultPartitionID,
		ID:          moneyid.NewBillID(t),
		Value:       3,
		Counter:     3,
	}
	targetBill := &Bill{
		NetworkID:   types.NetworkLocal,
		PartitionID: money.DefaultPartitionID,
		ID:          moneyid.NewBillID(t),
		Counter:     4,
	}
	tx1, err := db1.TransferToDustCollector(targetBill)
	require.NoError(t, err)
	tx2, err := db2.TransferToDustCollector(targetBill)
	require.NoError(t, err)
	tx1Bytes, err := tx1.MarshalCBOR()
	require.NoError(t, err)
	tx2Bytes, err := tx2.MarshalCBOR()
	require.NoError(t, err)
	proofs := []*types.TxRecordProof{
		{
			TxRecord: &types.TransactionRecord{TransactionOrder: tx1Bytes},
			TxProof:  &types.TxProof{},
		},
		{
			TxRecord: &types.TransactionRecord{TransactionOrder: tx2Bytes},
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
		ID:          moneyid.NewBillID(t),
		Value:       2,
		Counter:     3,
	}
	fcrCounter := uint64(4)
	fcr := &FeeCreditRecord{
		NetworkID:   types.NetworkLocal,
		PartitionID: 5,
		ID:          moneyid.NewFeeCreditRecordID(t),
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
		ID:          moneyid.NewBillID(t),
		Value:       2,
		Counter:     3,
	}
	closeFCProof := &types.TxRecordProof{
		TxRecord: &types.TransactionRecord{Version: 1},
		TxProof:  &types.TxProof{Version: 1},
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
		ID:          moneyid.NewBillID(t),
		Value:       2,
		Counter:     3,
	}
	stateLock := &types.StateLock{
		ExecutionPredicate: []byte{1},
		RollbackPredicate:  []byte{1},
	}
	tx, err := b.Lock(stateLock)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, nop.TransactionTypeNOP)
	require.Equal(t, b.PartitionID, tx.GetPartitionID())
	require.Equal(t, b.ID, tx.GetUnitID())
	require.Equal(t, stateLock, tx.StateLock)

	attr := &nop.Attributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, b.Counter, *attr.Counter)
}

func TestBillUnlock(t *testing.T) {
	b := &Bill{
		NetworkID:   types.NetworkLocal,
		PartitionID: money.DefaultPartitionID,
		ID:          moneyid.NewBillID(t),
		Value:       2,
		Counter:     3,
	}
	tx, err := b.Unlock()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, nop.TransactionTypeNOP)
	require.Equal(t, b.PartitionID, tx.GetPartitionID())
	require.Equal(t, b.ID, tx.GetUnitID())

	attr := &nop.Attributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.EqualValues(t, 4, *attr.Counter)
}
