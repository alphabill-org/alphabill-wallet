package api

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"

	"github.com/alphabill-org/alphabill-wallet/wallet"
)

// code extracted from backend->node refactor
// TODO organize and write unit tests

// ErrNotFound is returned by API methods if the requested item does not exist.
var ErrNotFound = errors.New("not found")

type (
	Bill struct {
		ID       types.UnitID
		BillData *money.BillData
	}

	FeeCreditBill struct {
		ID              types.UnitID
		FeeCreditRecord *unit.FeeCreditRecord
	}

	RpcClient interface {
		GetRoundNumber(ctx context.Context) (uint64, error)
		GetBill(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*Bill, error)
		GetFeeCreditRecord(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*FeeCreditBill, error)
		GetUnitsByOwnerID(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error)
		GetTransactionProof(ctx context.Context, txHash types.Bytes) (*types.TransactionRecord, *types.TxProof, error)
		SendTransaction(ctx context.Context, tx *types.TransactionOrder) ([]byte, error)
	}
)

func (b *FeeCreditBill) IsLocked() bool {
	if b == nil {
		return false
	}
	return b.FeeCreditRecord.IsLocked()
}

func (b *FeeCreditBill) Backlink() []byte {
	if b == nil {
		return nil
	}
	return b.FeeCreditRecord.GetBacklink()
}

func (b *FeeCreditBill) Balance() uint64 {
	if b == nil {
		return 0
	}
	if b.FeeCreditRecord == nil {
		return 0
	}
	return b.FeeCreditRecord.Balance
}

func (b *Bill) IsLocked() bool {
	if b == nil {
		return false
	}
	if b.BillData == nil {
		return false
	}
	return b.BillData.IsLocked()
}

func (b *Bill) Backlink() []byte {
	if b == nil {
		return nil
	}
	if b.BillData == nil {
		return nil
	}
	return b.BillData.Backlink
}

func (b *Bill) Value() uint64 {
	if b == nil {
		return 0
	}
	if b.BillData == nil {
		return 0
	}
	return b.BillData.V
}

func FetchBills(ctx context.Context, c RpcClient, ownerID []byte) ([]*Bill, error) {
	unitIDs, err := c.GetUnitsByOwnerID(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch owner units: %w", err)
	}
	var bills []*Bill
	for _, unitID := range unitIDs {
		if !unitID.HasType(money.BillUnitType) {
			continue
		}
		bill, err := c.GetBill(ctx, unitID, false)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch unit: %w", err)
		}
		bills = append(bills, bill)
	}
	return bills, nil
}

func FetchBill(ctx context.Context, c RpcClient, unitID types.UnitID) (*Bill, error) {
	bill, err := c.GetBill(ctx, unitID, false)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return bill, nil
}

func FetchFeeCreditBill(ctx context.Context, c RpcClient, fcrID types.UnitID) (*FeeCreditBill, error) {
	fcr, err := c.GetFeeCreditRecord(ctx, fcrID, false)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return fcr, nil
}

func WaitForConf(ctx context.Context, c RpcClient, tx *types.TransactionOrder) (*wallet.Proof, error) {
	txHash := tx.Hash(crypto.SHA256)
	for {
		// fetch round number before proof to ensure that we cannot miss the proof
		roundNumber, err := c.GetRoundNumber(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch target partition round number: %w", err)
		}
		txRecord, txProof, err := c.GetTransactionProof(ctx, txHash)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("failed to fetch tx proof: %w", err)
		}
		if txRecord != nil && txProof != nil {
			return &wallet.Proof{TxRecord: txRecord, TxProof: txProof}, nil
		}
		if roundNumber >= tx.Timeout() {
			break
		}
		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			return nil, errors.New("context canceled")
		}
	}
	return nil, nil
}
