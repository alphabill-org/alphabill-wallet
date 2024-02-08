package testutil

import (
	"bytes"
	"context"
	"crypto"

	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"

	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/api"
)

type (
	RpcClientMock struct {
		Err            error
		Bills          map[string]*api.Bill
		FeeCreditBills map[string]*api.FeeCreditBill
		OwnerUnits     []types.UnitID
		RoundNumber    uint64
		TxProofs       map[string]*wallet.Proof

		RecordedTxs []*types.TransactionOrder
	}

	Options struct {
		Err            error
		RoundNumber    uint64
		TxProofs       map[string]*wallet.Proof
		Bills          map[string]*api.Bill
		FeeCreditBills map[string]*api.FeeCreditBill
		OwnerUnits     []types.UnitID
	}

	Option func(*Options)
)

func NewRpcClientMock(opts ...Option) *RpcClientMock {
	options := &Options{
		Bills:          map[string]*api.Bill{},
		FeeCreditBills: map[string]*api.FeeCreditBill{},
		TxProofs:       map[string]*wallet.Proof{},
	}
	for _, option := range opts {
		option(options)
	}
	return &RpcClientMock{
		Err:            options.Err,
		RoundNumber:    options.RoundNumber,
		Bills:          options.Bills,
		FeeCreditBills: options.FeeCreditBills,
		OwnerUnits:     options.OwnerUnits,
		TxProofs:       options.TxProofs,
	}
}

func WithOwnerBill(bill *api.Bill) Option {
	return func(o *Options) {
		o.Bills[string(bill.ID)] = bill
		o.OwnerUnits = append(o.OwnerUnits, bill.ID)
	}
}

func WithOwnerFeeCreditBill(fcb *api.FeeCreditBill) Option {
	return func(o *Options) {
		o.FeeCreditBills[string(fcb.ID)] = fcb
		o.OwnerUnits = append(o.OwnerUnits, fcb.ID)
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

func (b *RpcClientMock) GetRoundNumber(ctx context.Context) (uint64, error) {
	if b.Err != nil {
		return 0, b.Err
	}
	return b.RoundNumber, nil
}

func (b *RpcClientMock) GetBill(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.Bill, error) {
	if b.Err != nil {
		return nil, b.Err
	}
	bill, ok := b.Bills[string(unitID)]
	if ok {
		return bill, nil
	}
	return nil, api.ErrNotFound
}

func (b *RpcClientMock) GetFeeCreditRecord(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.FeeCreditBill, error) {
	if b.Err != nil {
		return nil, b.Err
	}
	fcb, ok := b.FeeCreditBills[string(unitID)]
	if ok {
		return fcb, nil
	}
	return nil, api.ErrNotFound
}

func (b *RpcClientMock) GetUnitsByOwnerID(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error) {
	if b.Err != nil {
		return nil, b.Err
	}
	if b.OwnerUnits != nil {
		return b.OwnerUnits, nil
	}
	return nil, nil
}

func (b *RpcClientMock) SendTransaction(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
	if b.Err != nil {
		return nil, b.Err
	}
	b.RecordedTxs = append(b.RecordedTxs, tx)
	return tx.Hash(crypto.SHA256), nil
}

func (b *RpcClientMock) GetTransactionProof(ctx context.Context, txHash types.Bytes) (*types.TransactionRecord, *types.TxProof, error) {
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

func (b *RpcClientMock) GetBlock(ctx context.Context, blockNumber uint64) (*types.Block, error) {
	if b.Err != nil {
		return nil, b.Err
	}
	return &types.Block{}, nil
}

func NewMoneyBill(unitIDPart []byte, billData *money.BillData) *api.Bill {
	billID := money.NewBillID(nil, unitIDPart)
	return &api.Bill{
		ID:       billID,
		BillData: billData,
	}
}

func NewMoneyFCR(pubKeyHash []byte, fcrData *unit.FeeCreditRecord) *api.FeeCreditBill {
	fcrID := money.NewFeeCreditRecordID(nil, pubKeyHash)
	return &api.FeeCreditBill{
		ID:              fcrID,
		FeeCreditRecord: fcrData,
	}
}
