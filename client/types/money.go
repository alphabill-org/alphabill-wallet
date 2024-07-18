package types

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
)

type (
	MoneyPartitionClient interface {
		PartitionClient

		GetBill(ctx context.Context, unitID types.UnitID) (*Bill, error)
		GetBills(ctx context.Context, ownerID []byte) ([]*Bill, error)
	}

	Bill struct {
		SystemID types.SystemID
		ID       types.UnitID
		Data     *money.BillData
	}
)

func (b *Bill) IsLocked() bool {
	if b == nil {
		return false
	}
	if b.Data == nil {
		return false
	}
	return b.Data.IsLocked()
}

func (b *Bill) Counter() uint64 {
	if b == nil {
		return 0
	}
	if b.Data == nil {
		return 0
	}
	return b.Data.Counter
}

func (b *Bill) Value() uint64 {
	if b == nil {
		return 0
	}
	if b.Data == nil {
		return 0
	}
	return b.Data.V
}

func (b *Bill) Transfer(ownerPredicate []byte, txOptions ...TxOption) (*types.TransactionOrder, error) {
	opts := TxOptionsWithDefaults(txOptions)
	attr := &money.TransferAttributes{
		NewBearer:   ownerPredicate,
		TargetValue: b.Value(),
		Counter:     b.Counter(),
	}
	txPayload, err := NewPayload(b.SystemID, b.ID, money.PayloadTypeTransfer, attr, opts)
	if err != nil {
		return nil, err
	}

	tx := NewTransactionOrder(txPayload)
	GenerateAndSetProofs(tx, nil, nil, opts)
	return tx, nil
}

func (b *Bill) Split(targetUnits []*money.TargetUnit, txOptions ...TxOption) (*types.TransactionOrder, error) {
	opts := TxOptionsWithDefaults(txOptions)

	var totalAmount uint64
	for _, tu := range targetUnits {
		totalAmount += tu.Amount
	}
	remainingValue := b.Value() - totalAmount
	attr := &money.SplitAttributes{
		TargetUnits:    targetUnits,
		RemainingValue: remainingValue,
		Counter:        b.Counter(),
	}
	txPayload, err := NewPayload(b.SystemID, b.ID, money.PayloadTypeSplit, attr, opts)
	if err != nil {
		return nil, err
	}

	tx := NewTransactionOrder(txPayload)
	GenerateAndSetProofs(tx, nil, nil, opts)
	return tx, nil
}

func (b *Bill) TransferToDustCollector(targetBill *Bill, txOptions ...TxOption) (*types.TransactionOrder, error) {
	opts := TxOptionsWithDefaults(txOptions)

	attr := &money.TransferDCAttributes{
		TargetUnitID:      targetBill.ID,
		TargetUnitCounter: targetBill.Counter(),
		Value:             b.Value(),
		Counter:           b.Counter(),
	}
	txPayload, err := NewPayload(b.SystemID, b.ID, money.PayloadTypeTransDC, attr, opts)
	if err != nil {
		return nil, err
	}

	tx := NewTransactionOrder(txPayload)
	GenerateAndSetProofs(tx, nil, nil, opts)
	return tx, nil
}

func (b *Bill) SwapWithDustCollector(transDCProofs []*Proof, txOptions ...TxOption) (*types.TransactionOrder, error) {
	opts := TxOptionsWithDefaults(txOptions)

	if len(transDCProofs) == 0 {
		return nil, errors.New("cannot create swap transaction as no dust transfer proofs exist")
	}
	// sort proofs by ids smallest first
	sort.Slice(transDCProofs, func(i, j int) bool {
		return bytes.Compare(transDCProofs[i].TxRecord.TransactionOrder.UnitID(), transDCProofs[j].TxRecord.TransactionOrder.UnitID()) < 0
	})
	var dustTransferProofs []*types.TxProof
	var dustTransferRecords []*types.TransactionRecord
	var billValueSum uint64
	for _, p := range transDCProofs {
		dustTransferRecords = append(dustTransferRecords, p.TxRecord)
		dustTransferProofs = append(dustTransferProofs, p.TxProof)
		var attr *money.TransferDCAttributes
		if err := p.TxRecord.TransactionOrder.UnmarshalAttributes(&attr); err != nil {
			return nil, fmt.Errorf("failed to unmarshal dust transfer tx: %w", err)
		}
		billValueSum += attr.Value
	}
	attr := &money.SwapDCAttributes{
		DcTransfers:      dustTransferRecords,
		DcTransferProofs: dustTransferProofs,
		TargetValue:      billValueSum,
	}
	txPayload, err := NewPayload(b.SystemID, b.ID, money.PayloadTypeSwapDC, attr, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to build swap transaction: %w", err)
	}

	tx := NewTransactionOrder(txPayload)
	GenerateAndSetProofs(tx, nil, nil, opts)
	return tx, nil
}

func (b *Bill) Lock(lockStatus uint64, txOptions ...TxOption) (*types.TransactionOrder, error) {
	opts := TxOptionsWithDefaults(txOptions)

	attr := &money.LockAttributes{
		LockStatus: lockStatus,
		Counter:    b.Counter(),
	}
	txPayload, err := NewPayload(b.SystemID, b.ID, money.PayloadTypeLock, attr, opts)
	if err != nil {
		return nil, err
	}

	tx := NewTransactionOrder(txPayload)
	GenerateAndSetProofs(tx, nil, nil, opts)
	return tx, nil
}

func (b *Bill) Unlock(txOptions ...TxOption) (*types.TransactionOrder, error) {
	opts := TxOptionsWithDefaults(txOptions)

	attr := &money.UnlockAttributes{
		Counter: b.Counter(),
	}
	txPayload, err := NewPayload(b.SystemID, b.ID, money.PayloadTypeUnlock, attr, opts)
	if err != nil {
		return nil, err
	}

	tx := NewTransactionOrder(txPayload)
	GenerateAndSetProofs(tx, nil, nil, opts)
	return tx, nil
}

func (b *Bill) TransferToFeeCredit(fcr *FeeCreditRecord, amount uint64, latestAdditionTime uint64, txOptions ...TxOption) (*types.TransactionOrder, error) {
	opts := TxOptionsWithDefaults(txOptions)

	var targetUnitCounter *uint64
	if fcr.Data != nil {
		c := fcr.Counter()
		targetUnitCounter = &c
	}

	attr := &fc.TransferFeeCreditAttributes{
		Amount:                 amount,
		TargetSystemIdentifier: fcr.SystemID,
		TargetRecordID:         fcr.ID,
		LatestAdditionTime:     latestAdditionTime,
		// TODO: rename to TargetRecordCounter? or TargetUnitID above?
		TargetUnitCounter:      targetUnitCounter,
		Counter:                b.Counter(),
	}
	txPayload, err := NewPayload(b.SystemID, b.ID, fc.PayloadTypeTransferFeeCredit, attr, opts)
	if err != nil {
		return nil, err
	}

	tx := NewTransactionOrder(txPayload)
	GenerateAndSetProofs(tx, nil, nil, opts)
	return tx, nil
}

func (b *Bill) ReclaimFeeCredit(closeFCProof *Proof, txOptions ...TxOption) (*types.TransactionOrder, error) {
	opts := TxOptionsWithDefaults(txOptions)

	attr := &fc.ReclaimFeeCreditAttributes{
		CloseFeeCreditTransfer: closeFCProof.TxRecord,
		CloseFeeCreditProof:    closeFCProof.TxProof,
		Counter:                b.Counter(),
	}
	txPayload, err := NewPayload(b.SystemID, b.ID, fc.PayloadTypeReclaimFeeCredit, attr, opts)
	if err != nil {
		return nil, err
	}

	tx := NewTransactionOrder(txPayload)
	GenerateAndSetProofs(tx, nil, nil, opts)
	return tx, nil
}
