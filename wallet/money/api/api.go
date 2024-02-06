package api

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
)

// code extracted from backend->node refactor
// TODO organize and write unit tests

// TODO type safe error check
var ErrNotFound = errors.New("not found") // error if unit does not exist

type (
	Bill struct {
		ID       types.UnitID
		BillData *money.BillData
	}

	FeeCreditBill struct {
		ID              types.UnitID
		FeeCreditRecord *unit.FeeCreditRecord
	}

	StateAPI interface {
		GetRoundNumber(ctx context.Context) (uint64, error)
		GetUnit(ctx context.Context, unitID []byte, returnProof, returnData bool) (*types.UnitDataAndProof, error)
		GetUnitsByOwnerID(ctx context.Context, ownerID []byte) ([]types.UnitID, error)
		GetTransactionProof(ctx context.Context, txHash []byte) (*types.TransactionRecord, *types.TxProof, error)
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

func FetchBills(ctx context.Context, c StateAPI, ownerID []byte) ([]*Bill, error) {
	unitIDs, err := c.GetUnitsByOwnerID(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch owner units: %w", err)
	}
	var bills []*Bill
	for _, unitID := range unitIDs {
		dataAndProof, err := c.GetUnit(ctx, unitID, false, true)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch unit: %w", err)
		}
		if unitID.HasType(money.BillUnitType) {
			var bill *money.BillData
			if err := dataAndProof.UnmarshalUnitData(&bill); err != nil {
				return nil, fmt.Errorf("failed to decode unit data: %w", err)
			}
			bills = append(bills, &Bill{ID: unitID, BillData: bill})
		}
	}
	return bills, nil
}

func FetchBill(ctx context.Context, c StateAPI, unitID types.UnitID) (*Bill, error) {
	unitData, err := c.GetUnit(ctx, unitID, false, true)
	if err != nil {
		// TODO type safe error check
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		return nil, err
	}
	var billData *money.BillData
	if err := unitData.UnmarshalUnitData(&billData); err != nil {
		return nil, fmt.Errorf("failed to decode unit: %w", err)
	}
	return &Bill{ID: unitID, BillData: billData}, nil
}

func FetchFeeCreditBill(ctx context.Context, c StateAPI, fcrID types.UnitID) (*FeeCreditBill, error) {
	unitData, err := c.GetUnit(ctx, fcrID, false, true)
	if err != nil {
		// TODO type safe error check
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		return nil, err
	}
	var fcr *unit.FeeCreditRecord
	if err := unitData.UnmarshalUnitData(&fcr); err != nil {
		return nil, fmt.Errorf("failed to decode fee credit record: %w", err)
	}
	return &FeeCreditBill{ID: fcrID, FeeCreditRecord: fcr}, nil
}

func WaitForConf(ctx context.Context, c StateAPI, tx *types.TransactionOrder) (*wallet.Proof, error) {
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
