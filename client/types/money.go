package types

import (
	"context"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
)

type (
	MoneyPartitionClient interface {
		PartitionClient

		GetBill(ctx context.Context, unitID types.UnitID) (Bill, error)
		GetBills(ctx context.Context, ownerID []byte) ([]Bill, error)
	}

	Bill interface {
		SystemID() types.SystemID
		ID() types.UnitID
		Value() uint64
		Counter() uint64
		IncreaseCounter()
		LockStatus() uint64

		Transfer(ownerPredicate []byte, txOptions ...TxOption) (*types.TransactionOrder, error)
		Split(targetUnits []*money.TargetUnit, txOptions ...TxOption) (*types.TransactionOrder, error)
		TransferToDustCollector(targetBill Bill, txOptions ...TxOption) (*types.TransactionOrder, error)
		SwapWithDustCollector(transDCProofs []*Proof, txOptions ...TxOption) (*types.TransactionOrder, error)
		TransferToFeeCredit(fcr FeeCreditRecord, amount uint64, latestAdditionTime uint64, txOptions ...TxOption) (*types.TransactionOrder, error)
		ReclaimFromFeeCredit(closeFCProof *Proof, txOptions ...TxOption) (*types.TransactionOrder, error)
		Lock(lockStatus uint64, txOptions ...TxOption) (*types.TransactionOrder, error)
		Unlock(txOptions ...TxOption) (*types.TransactionOrder, error)
	}
)
