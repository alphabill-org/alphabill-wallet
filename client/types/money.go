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
		SystemID   types.SystemID
		ID         types.UnitID
		Value      uint64
		LastUpdate uint64
		LockStatus uint64
		Counter    uint64
	}
)

func (b *Bill) Transfer(newOwnerPredicate []byte, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &money.TransferAttributes{
		NewOwnerPredicate: newOwnerPredicate,
		TargetValue:       b.Value,
		Counter:           b.Counter,
	}
	txPayload, err := NewPayload(b.SystemID, b.ID, money.PayloadTypeTransfer, attr, txOptions...)
	if err != nil {
		return nil, err
	}
	return NewTransactionOrder(txPayload), nil
}

func (b *Bill) Split(targetUnits []*money.TargetUnit, txOptions ...Option) (*types.TransactionOrder, error) {
	var totalAmount uint64
	for _, tu := range targetUnits {
		totalAmount += tu.Amount
	}
	remainingValue := b.Value - totalAmount
	attr := &money.SplitAttributes{
		TargetUnits:    targetUnits,
		RemainingValue: remainingValue,
		Counter:        b.Counter,
	}
	txPayload, err := NewPayload(b.SystemID, b.ID, money.PayloadTypeSplit, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	return NewTransactionOrder(txPayload), nil
}

func (b *Bill) TransferToDustCollector(targetBill *Bill, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &money.TransferDCAttributes{
		TargetUnitID:      targetBill.ID,
		TargetUnitCounter: targetBill.Counter,
		Value:             b.Value,
		Counter:           b.Counter,
	}
	txPayload, err := NewPayload(b.SystemID, b.ID, money.PayloadTypeTransDC, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	return NewTransactionOrder(txPayload), nil
}

func (b *Bill) SwapWithDustCollector(transDCProofs []*Proof, txOptions ...Option) (*types.TransactionOrder, error) {
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
	txPayload, err := NewPayload(b.SystemID, b.ID, money.PayloadTypeSwapDC, attr, txOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to build swap transaction: %w", err)
	}

	return NewTransactionOrder(txPayload), nil
}

func (b *Bill) TransferToFeeCredit(fcr *FeeCreditRecord, amount uint64, latestAdditionTime uint64, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &fc.TransferFeeCreditAttributes{
		Amount:                 amount,
		TargetSystemIdentifier: fcr.SystemID,
		TargetRecordID:         fcr.ID,
		LatestAdditionTime:     latestAdditionTime,
		TargetUnitCounter:      fcr.Counter,
		Counter:                b.Counter,
	}
	txPayload, err := NewPayload(b.SystemID, b.ID, fc.PayloadTypeTransferFeeCredit, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	return NewTransactionOrder(txPayload), nil
}

func (b *Bill) ReclaimFromFeeCredit(closeFCProof *Proof, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &fc.ReclaimFeeCreditAttributes{
		CloseFeeCreditTransfer: closeFCProof.TxRecord,
		CloseFeeCreditProof:    closeFCProof.TxProof,
		Counter:                b.Counter,
	}
	txPayload, err := NewPayload(b.SystemID, b.ID, fc.PayloadTypeReclaimFeeCredit, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	return NewTransactionOrder(txPayload), nil
}

func (b *Bill) Lock(lockStatus uint64, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &money.LockAttributes{
		LockStatus: lockStatus,
		Counter:    b.Counter,
	}
	txPayload, err := NewPayload(b.SystemID, b.ID, money.PayloadTypeLock, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	return NewTransactionOrder(txPayload), nil
}

func (b *Bill) Unlock(txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &money.UnlockAttributes{
		Counter: b.Counter,
	}
	txPayload, err := NewPayload(b.SystemID, b.ID, money.PayloadTypeUnlock, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	return NewTransactionOrder(txPayload), nil
}
