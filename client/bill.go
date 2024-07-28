package client

import (
	"bytes"
	"errors"
	"fmt"
	"sort"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
)

type (
	bill struct {
		systemID   types.SystemID
		id         types.UnitID
		value      uint64
		counter    uint64
		lockStatus uint64
	}
)

func (b *bill) SystemID() types.SystemID {
	return b.systemID
}

func (b *bill) ID() types.UnitID {
	return b.id
}

func (b *bill) Value() uint64 {
	return b.value
}

func (b *bill) Counter() uint64 {
	// TODO: need to return 0 if b == nil?
	return b.counter
}

func (b *bill) IncreaseCounter() {
	b.counter += 1
}

func (b *bill) LockStatus() uint64 {
	return b.lockStatus
}

func (b *bill) Transfer(ownerPredicate []byte, txOptions ...sdktypes.TxOption) (*types.TransactionOrder, error) {
	opts := sdktypes.TxOptionsWithDefaults(txOptions)
	attr := &money.TransferAttributes{
		NewBearer:   ownerPredicate,
		TargetValue: b.value,
		Counter:     b.counter,
	}
	txPayload, err := sdktypes.NewPayload(b.systemID, b.id, money.PayloadTypeTransfer, attr, opts)
	if err != nil {
		return nil, err
	}

	tx := sdktypes.NewTransactionOrder(txPayload)
	sdktypes.GenerateAndSetProofs(tx, nil, nil, opts)
	return tx, nil
}

func (b *bill) Split(targetUnits []*money.TargetUnit, txOptions ...sdktypes.TxOption) (*types.TransactionOrder, error) {
	opts := sdktypes.TxOptionsWithDefaults(txOptions)

	var totalAmount uint64
	for _, tu := range targetUnits {
		totalAmount += tu.Amount
	}
	remainingValue := b.value - totalAmount
	attr := &money.SplitAttributes{
		TargetUnits:    targetUnits,
		RemainingValue: remainingValue,
		Counter:        b.counter,
	}
	txPayload, err := sdktypes.NewPayload(b.systemID, b.id, money.PayloadTypeSplit, attr, opts)
	if err != nil {
		return nil, err
	}

	tx := sdktypes.NewTransactionOrder(txPayload)
	sdktypes.GenerateAndSetProofs(tx, nil, nil, opts)
	return tx, nil
}

func (b *bill) TransferToDustCollector(targetBill sdktypes.Bill, txOptions ...sdktypes.TxOption) (*types.TransactionOrder, error) {
	opts := sdktypes.TxOptionsWithDefaults(txOptions)

	attr := &money.TransferDCAttributes{
		TargetUnitID:      targetBill.ID(),
		TargetUnitCounter: targetBill.Counter(),
		Value:             b.value,
		Counter:           b.counter,
	}
	txPayload, err := sdktypes.NewPayload(b.systemID, b.id, money.PayloadTypeTransDC, attr, opts)
	if err != nil {
		return nil, err
	}

	tx := sdktypes.NewTransactionOrder(txPayload)
	sdktypes.GenerateAndSetProofs(tx, nil, nil, opts)
	return tx, nil
}

func (b *bill) SwapWithDustCollector(transDCProofs []*sdktypes.Proof, txOptions ...sdktypes.TxOption) (*types.TransactionOrder, error) {
	opts := sdktypes.TxOptionsWithDefaults(txOptions)

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
	txPayload, err := sdktypes.NewPayload(b.systemID, b.id, money.PayloadTypeSwapDC, attr, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to build swap transaction: %w", err)
	}

	tx := sdktypes.NewTransactionOrder(txPayload)
	sdktypes.GenerateAndSetProofs(tx, nil, nil, opts)
	return tx, nil
}

func (b *bill) TransferToFeeCredit(fcr sdktypes.FeeCreditRecord, amount uint64, latestAdditionTime uint64, txOptions ...sdktypes.TxOption) (*types.TransactionOrder, error) {
	opts := sdktypes.TxOptionsWithDefaults(txOptions)

	attr := &fc.TransferFeeCreditAttributes{
		Amount:                 amount,
		TargetSystemIdentifier: fcr.SystemID(),
		TargetRecordID:         fcr.ID(),
		LatestAdditionTime:     latestAdditionTime,
		// TODO: rename to TargetRecordCounter? or TargetUnitID above?
		TargetUnitCounter:      fcr.Counter(),
		Counter:                b.counter,
	}
	txPayload, err := sdktypes.NewPayload(b.systemID, b.id, fc.PayloadTypeTransferFeeCredit, attr, opts)
	if err != nil {
		return nil, err
	}

	tx := sdktypes.NewTransactionOrder(txPayload)
	sdktypes.GenerateAndSetProofs(tx, nil, nil, opts)
	return tx, nil
}

func (b *bill) ReclaimFromFeeCredit(closeFCProof *sdktypes.Proof, txOptions ...sdktypes.TxOption) (*types.TransactionOrder, error) {
	opts := sdktypes.TxOptionsWithDefaults(txOptions)

	attr := &fc.ReclaimFeeCreditAttributes{
		CloseFeeCreditTransfer: closeFCProof.TxRecord,
		CloseFeeCreditProof:    closeFCProof.TxProof,
		Counter:                b.counter,
	}
	txPayload, err := sdktypes.NewPayload(b.systemID, b.id, fc.PayloadTypeReclaimFeeCredit, attr, opts)
	if err != nil {
		return nil, err
	}

	tx := sdktypes.NewTransactionOrder(txPayload)
	sdktypes.GenerateAndSetProofs(tx, nil, nil, opts)
	return tx, nil
}

func (b *bill) Lock(lockStatus uint64, txOptions ...sdktypes.TxOption) (*types.TransactionOrder, error) {
	opts := sdktypes.TxOptionsWithDefaults(txOptions)

	attr := &money.LockAttributes{
		LockStatus: lockStatus,
		Counter:    b.counter,
	}
	txPayload, err := sdktypes.NewPayload(b.systemID, b.id, money.PayloadTypeLock, attr, opts)
	if err != nil {
		return nil, err
	}

	tx := sdktypes.NewTransactionOrder(txPayload)
	sdktypes.GenerateAndSetProofs(tx, nil, nil, opts)
	return tx, nil
}

func (b *bill) Unlock(txOptions ...sdktypes.TxOption) (*types.TransactionOrder, error) {
	opts := sdktypes.TxOptionsWithDefaults(txOptions)

	attr := &money.UnlockAttributes{
		Counter: b.counter,
	}
	txPayload, err := sdktypes.NewPayload(b.systemID, b.id, money.PayloadTypeUnlock, attr, opts)
	if err != nil {
		return nil, err
	}

	tx := sdktypes.NewTransactionOrder(txPayload)
	sdktypes.GenerateAndSetProofs(tx, nil, nil, opts)
	return tx, nil
}
