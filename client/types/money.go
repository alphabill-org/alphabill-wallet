package types

import (
	"context"

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
		ID   types.UnitID
		Data *money.BillData
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
