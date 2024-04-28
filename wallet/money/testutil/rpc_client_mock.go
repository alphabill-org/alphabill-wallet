package testutil

import (
	"bytes"
	"context"
	"crypto"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/money"
	"github.com/alphabill-org/alphabill-go-sdk/types"

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

func (c *RpcClientMock) GetRoundNumber(ctx context.Context) (uint64, error) {
	if c.Err != nil {
		return 0, c.Err
	}
	return c.RoundNumber, nil
}

func (c *RpcClientMock) GetBill(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.Bill, error) {
	if c.Err != nil {
		return nil, c.Err
	}
	bill, ok := c.Bills[string(unitID)]
	if ok {
		return bill, nil
	}
	return nil, api.ErrNotFound
}

func (c *RpcClientMock) GetFeeCreditRecord(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.FeeCreditBill, error) {
	if c.Err != nil {
		return nil, c.Err
	}
	fcb, ok := c.FeeCreditBills[string(unitID)]
	if ok {
		return fcb, nil
	}
	return nil, api.ErrNotFound
}

func (c *RpcClientMock) GetUnitsByOwnerID(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error) {
	if c.Err != nil {
		return nil, c.Err
	}
	if c.OwnerUnits != nil {
		return c.OwnerUnits, nil
	}
	return nil, nil
}

func (c *RpcClientMock) SendTransaction(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
	if c.Err != nil {
		return nil, c.Err
	}
	c.RecordedTxs = append(c.RecordedTxs, tx)
	return tx.Hash(crypto.SHA256), nil
}

func (c *RpcClientMock) GetTransactionProof(ctx context.Context, txHash types.Bytes) (*types.TransactionRecord, *types.TxProof, error) {
	if c.Err != nil {
		return nil, nil, c.Err
	}
	txProofs, ok := c.TxProofs[string(txHash)]
	if ok {
		return txProofs.TxRecord, txProofs.TxProof, nil
	}
	// return proof for sent tx if one exists
	if len(c.RecordedTxs) > 0 {
		for _, tx := range c.RecordedTxs {
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

func (c *RpcClientMock) GetBlock(ctx context.Context, blockNumber uint64) (*types.Block, error) {
	if c.Err != nil {
		return nil, c.Err
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

func NewMoneyFCR(pubKeyHash []byte, fcrData *fc.FeeCreditRecord) *api.FeeCreditBill {
	fcrID := money.NewFeeCreditRecordID(nil, pubKeyHash)
	return &api.FeeCreditBill{
		ID:              fcrID,
		FeeCreditRecord: fcrData,
	}
}
