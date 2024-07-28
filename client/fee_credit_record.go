package client

import (
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
)

type (
	feeCreditRecord struct {
		systemID   types.SystemID
		id         types.UnitID
		balance    uint64
		counter    *uint64
		timeout    uint64
		lockStatus uint64
	}
)

func NewFeeCreditRecord(systemID types.SystemID, id types.UnitID) sdktypes.FeeCreditRecord {
	return &feeCreditRecord{
		systemID: systemID,
		id:       id,
		counter:  nil,
	}
}

func (f *feeCreditRecord) AddFreeCredit(ownerPredicate []byte, transFCProof *sdktypes.Proof, txOptions ...sdktypes.TxOption) (*types.TransactionOrder, error) {
	opts := sdktypes.TxOptionsWithDefaults(txOptions)
	attr := &fc.AddFeeCreditAttributes{
		FeeCreditOwnerCondition: ownerPredicate,
		FeeCreditTransfer:       transFCProof.TxRecord,
		FeeCreditTransferProof:  transFCProof.TxProof,
	}
	txPayload, err := sdktypes.NewPayload(f.systemID, f.id, fc.PayloadTypeAddFeeCredit, attr, opts)
	if err != nil {
		return nil, err
	}
	tx := sdktypes.NewTransactionOrder(txPayload)
	sdktypes.GenerateAndSetProofs(tx, nil, nil, opts)
	return tx, nil
}

func (f *feeCreditRecord) CloseFreeCredit(targetBill sdktypes.Bill, txOptions ...sdktypes.TxOption) (*types.TransactionOrder, error) {
	opts := sdktypes.TxOptionsWithDefaults(txOptions)
	attr := &fc.CloseFeeCreditAttributes{
		Amount:            f.balance,
		TargetUnitID:      targetBill.ID(),
		TargetUnitCounter: targetBill.Counter(),
		Counter:           *f.counter,
	}
	txPayload, err := sdktypes.NewPayload(f.systemID, f.id, fc.PayloadTypeCloseFeeCredit, attr, opts)
	if err != nil {
		return nil, err
	}
	tx := sdktypes.NewTransactionOrder(txPayload)
	sdktypes.GenerateAndSetProofs(tx, nil, nil, opts)
	return tx, nil
}

func (f *feeCreditRecord) Lock(lockStatus uint64, txOptions ...sdktypes.TxOption) (*types.TransactionOrder, error) {
	opts := sdktypes.TxOptionsWithDefaults(txOptions)
	attr := &fc.LockFeeCreditAttributes{
		LockStatus: lockStatus,
		Counter:    *f.counter,
	}
	txPayload, err := sdktypes.NewPayload(f.systemID, f.id, fc.PayloadTypeLockFeeCredit, attr, opts)
	if err != nil {
		return nil, err
	}
	tx := sdktypes.NewTransactionOrder(txPayload)
	sdktypes.GenerateAndSetProofs(tx, nil, nil, opts)
	return tx, nil
}

func (f *feeCreditRecord) Unlock(txOptions ...sdktypes.TxOption) (*types.TransactionOrder, error) {
	opts := sdktypes.TxOptionsWithDefaults(txOptions)
	attr := &fc.UnlockFeeCreditAttributes{
		Counter: *f.counter,
	}
	txPayload, err := sdktypes.NewPayload(f.systemID, f.id, fc.PayloadTypeUnlockFeeCredit, attr, opts)
	if err != nil {
		return nil, err
	}
	tx := sdktypes.NewTransactionOrder(txPayload)
	sdktypes.GenerateAndSetProofs(tx, nil, nil, opts)
	return tx, nil
}

func (f *feeCreditRecord) SystemID() types.SystemID {
	return f.systemID
}

func (f *feeCreditRecord) ID() types.UnitID {
	return f.id
}

func (f *feeCreditRecord) Balance() uint64 {
	return f.balance
}

func (f *feeCreditRecord) Counter() *uint64 {
	return f.counter
}

func (f *feeCreditRecord) Timeout() uint64 {
	return f.timeout
}

func (f *feeCreditRecord) LockStatus() uint64 {
	return f.lockStatus
}
