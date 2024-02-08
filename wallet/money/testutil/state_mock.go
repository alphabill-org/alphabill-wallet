package testutil

import (
	"bytes"
	"context"
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"

	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/api"
)

type (
	StateAPIMock struct {
		Err         error
		Units       map[string]*rpc.Unit[any]
		OwnerUnits  []types.UnitID
		RoundNumber uint64
		TxProofs    map[string]*wallet.Proof

		RecordedTxs []*types.TransactionOrder
	}

	Options struct {
		Err         error
		RoundNumber uint64
		TxProofs    map[string]*wallet.Proof
		Units       map[string]*rpc.Unit[any]
		OwnerUnits  []types.UnitID
	}

	Option func(*Options)
)

func NewStateAPIMock(opts ...Option) *StateAPIMock {
	options := &Options{
		Units:    map[string]*rpc.Unit[any]{},
		TxProofs: map[string]*wallet.Proof{},
	}
	for _, option := range opts {
		option(options)
	}
	return &StateAPIMock{
		Err:         options.Err,
		RoundNumber: options.RoundNumber,
		Units:       options.Units,
		OwnerUnits:  options.OwnerUnits,
		TxProofs:    options.TxProofs,
	}
}

func WithOwnerUnit(unit *rpc.Unit[any]) Option {
	return func(o *Options) {
		o.Units[string(unit.UnitID)] = unit
		o.OwnerUnits = append(o.OwnerUnits, unit.UnitID)
	}
}

func WithTxProof(txHash []byte, txProof *wallet.Proof) Option {
	return func(o *Options) {
		o.TxProofs[string(txHash)] = txProof
	}
}

func WithRoundNumber(roundNumber uint64) Option {
	return func(o *Options) {
		o.RoundNumber = roundNumber
	}
}

func WithError(err error) Option {
	return func(o *Options) {
		o.Err = err
	}
}

func (b *StateAPIMock) GetRoundNumber(ctx context.Context) (uint64, error) {
	if b.Err != nil {
		return 0, b.Err
	}
	return b.RoundNumber, nil
}

func (b *StateAPIMock) GetUnit(ctx context.Context, unitID []byte, includeStateProof bool) (*rpc.Unit[any], error) {
	if b.Err != nil {
		return nil, b.Err
	}
	unitData, ok := b.Units[string(unitID)]
	if ok {
		return unitData, nil
	}
	return nil, api.ErrNotFound
}

func (b *StateAPIMock) GetUnitsByOwnerID(ctx context.Context, ownerID []byte) ([]types.UnitID, error) {
	if b.Err != nil {
		return nil, b.Err
	}
	if b.OwnerUnits != nil {
		return b.OwnerUnits, nil
	}
	return nil, nil
}

func (b *StateAPIMock) SendTransaction(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
	if b.Err != nil {
		return nil, b.Err
	}
	b.RecordedTxs = append(b.RecordedTxs, tx)
	return tx.Hash(crypto.SHA256), nil
}

func (b *StateAPIMock) GetTransactionProof(ctx context.Context, txHash []byte) (*types.TransactionRecord, *types.TxProof, error) {
	if b.Err != nil {
		return nil, nil, b.Err
	}
	txProofs, ok := b.TxProofs[string(txHash)]
	if ok {
		return txProofs.TxRecord, txProofs.TxProof, nil
	}
	// return proof for sent tx if one exists
	if len(b.RecordedTxs) > 0 {
		for _, tx := range b.RecordedTxs {
			if bytes.Equal(txHash, tx.Hash(crypto.SHA256)) {
				txr := &types.TransactionRecord{
					TransactionOrder: tx,
					ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
				}
				return txr, &types.TxProof{}, nil
			}
		}
	}
	return nil, nil, api.ErrNotFound
}

func (b *StateAPIMock) GetBlock(ctx context.Context, blockNumber uint64) (*types.Block, error) {
	if b.Err != nil {
		return nil, b.Err
	}
	return &types.Block{}, nil
}

func NewMoneyBill(t *testing.T, unitIDPart []byte, billData *money.BillData) *rpc.Unit[any] {
	billID := money.NewBillID(nil, unitIDPart)
	return &rpc.Unit[any]{
		UnitID: billID,
		Data: *billData,
	}
}

func NewMoneyFCR(t *testing.T, pubKeyHash []byte, fcrData *unit.FeeCreditRecord) *rpc.Unit[any] {
	fcrID := money.NewFeeCreditRecordID(nil, pubKeyHash)
	return &rpc.Unit[any]{
		UnitID: fcrID,
		Data: *fcrData,
	}
}
