package types

import (
	"context"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill-wallet/client/tx"
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

		Transfer(ownerPredicate []byte, txOptions ...tx.Option) (*types.TransactionOrder, error)
		Split(targetUnits []*money.TargetUnit, txOptions ...tx.Option) (*types.TransactionOrder, error)
		TransferToDustCollector(targetBill Bill, txOptions ...tx.Option) (*types.TransactionOrder, error)
		SwapWithDustCollector(transDCProofs []*Proof, txOptions ...tx.Option) (*types.TransactionOrder, error)
		TransferToFeeCredit(fcr FeeCreditRecord, amount uint64, latestAdditionTime uint64, txOptions ...tx.Option) (*types.TransactionOrder, error)
		ReclaimFromFeeCredit(closeFCProof *Proof, txOptions ...tx.Option) (*types.TransactionOrder, error)
		Lock(lockStatus uint64, txOptions ...tx.Option) (*types.TransactionOrder, error)
		Unlock(txOptions ...tx.Option) (*types.TransactionOrder, error)
	}
)
