package types

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/txsystem/nop"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
)

type (
	MoneyPartitionClient interface {
		PartitionClient

		GetBill(ctx context.Context, unitID types.UnitID) (*Bill, error)
		GetBills(ctx context.Context, ownerID []byte) ([]*Bill, error)
	}

	Bill struct {
		NetworkID   types.NetworkID
		PartitionID types.PartitionID
		ID          types.UnitID
		Value       uint64
		StateLockTx hex.Bytes
		Counter     uint64
	}
)

func (b *Bill) Transfer(newOwnerPredicate []byte, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &money.TransferAttributes{
		NewOwnerPredicate: newOwnerPredicate,
		TargetValue:       b.Value,
		Counter:           b.Counter,
	}
	return NewTransactionOrder(b.NetworkID, b.PartitionID, b.ID, money.TransactionTypeTransfer, attr, txOptions...)
}

func (b *Bill) Split(targetUnits []*money.TargetUnit, txOptions ...Option) (*types.TransactionOrder, error) {
	var totalAmount uint64
	for _, tu := range targetUnits {
		totalAmount += tu.Amount
	}
	attr := &money.SplitAttributes{
		TargetUnits: targetUnits,
		Counter:     b.Counter,
	}
	return NewTransactionOrder(b.NetworkID, b.PartitionID, b.ID, money.TransactionTypeSplit, attr, txOptions...)
}

func (b *Bill) TransferToDustCollector(targetBill *Bill, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &money.TransferDCAttributes{
		TargetUnitID:      targetBill.ID,
		TargetUnitCounter: targetBill.Counter,
		Value:             b.Value,
		Counter:           b.Counter,
	}
	return NewTransactionOrder(b.NetworkID, b.PartitionID, b.ID, money.TransactionTypeTransDC, attr, txOptions...)
}

func (b *Bill) SwapWithDustCollector(transDCProofs []*types.TxRecordProof, txOptions ...Option) (*types.TransactionOrder, error) {
	if len(transDCProofs) == 0 {
		return nil, errors.New("cannot create swap transaction as no dust transfer proofs exist")
	}
	var extErr error
	// sort proofs by ids smallest first
	sort.Slice(transDCProofs, func(i, j int) bool {
		txoI, err := transDCProofs[i].TxRecord.GetTransactionOrderV1()
		if err != nil {
			extErr = fmt.Errorf("failed to get transaction order from proof: %w", err)
			return false
		}
		txoJ, err := transDCProofs[j].TxRecord.GetTransactionOrderV1()
		if err != nil {
			extErr = fmt.Errorf("failed to get transaction order from proof: %w", err)
			return false
		}
		return bytes.Compare(txoI.GetUnitID(), txoJ.GetUnitID()) < 0
	})
	if extErr != nil {
		return nil, extErr
	}
	attr := &money.SwapDCAttributes{DustTransferProofs: transDCProofs}
	txo, err := NewTransactionOrder(b.NetworkID, b.PartitionID, b.ID, money.TransactionTypeSwapDC, attr, txOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to build swap transaction: %w", err)
	}
	return txo, nil
}

func (b *Bill) TransferToFeeCredit(fcr *FeeCreditRecord, amount uint64, latestAdditionTime uint64, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &fc.TransferFeeCreditAttributes{
		Amount:             amount,
		TargetPartitionID:  fcr.PartitionID,
		TargetRecordID:     fcr.ID,
		LatestAdditionTime: latestAdditionTime,
		TargetUnitCounter:  fcr.Counter,
		Counter:            b.Counter,
	}
	return NewTransactionOrder(b.NetworkID, b.PartitionID, b.ID, fc.TransactionTypeTransferFeeCredit, attr, txOptions...)
}

func (b *Bill) ReclaimFromFeeCredit(closeFCProof *types.TxRecordProof, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &fc.ReclaimFeeCreditAttributes{CloseFeeCreditProof: closeFCProof}
	return NewTransactionOrder(b.NetworkID, b.PartitionID, b.ID, fc.TransactionTypeReclaimFeeCredit, attr, txOptions...)
}

func (b *Bill) Lock(stateLock *types.StateLock, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &nop.Attributes{
		Counter: &(b.Counter),
	}
	txOptions = append(txOptions, WithStateLock(stateLock))
	return NewTransactionOrder(b.NetworkID, b.PartitionID, b.ID, nop.TransactionTypeNOP, attr, txOptions...)
}

func (b *Bill) Unlock(txOptions ...Option) (*types.TransactionOrder, error) {
	counter := b.Counter + 1 // the lock transaction has not been executed yet
	attr := &nop.Attributes{
		Counter: &counter,
	}
	return NewTransactionOrder(b.NetworkID, b.PartitionID, b.ID, nop.TransactionTypeNOP, attr, txOptions...)
}
