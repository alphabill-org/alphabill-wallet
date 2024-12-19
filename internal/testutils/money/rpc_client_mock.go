package money

import (
	"bytes"
	"context"
	"crypto"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	moneyid "github.com/alphabill-org/alphabill-go-base/testutils/money"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
)

const transferFCLatestAdditionTime = 65536 // relative timeout after which transferFC unit becomes unusable

type (
	RpcClientMock struct {
		pdr                   *types.PartitionDescriptionRecord
		Err                   error
		Bills                 map[string]*sdktypes.Bill
		OwnerBills            []*sdktypes.Bill
		FeeCreditRecords      map[string]*sdktypes.FeeCreditRecord
		OwnerFeeCreditRecords []*sdktypes.FeeCreditRecord
		RoundNumber           uint64
		TxProofs              map[string]*types.TxRecordProof

		RecordedTxs []*types.TransactionOrder
	}

	Options struct {
		pdr                   *types.PartitionDescriptionRecord
		Err                   error
		RoundNumber           uint64
		TxProofs              map[string]*types.TxRecordProof
		Bills                 map[string]*sdktypes.Bill
		OwnerBills            []*sdktypes.Bill
		FeeCreditRecords      map[string]*sdktypes.FeeCreditRecord
		OwnerFeeCreditRecords []*sdktypes.FeeCreditRecord
	}

	Option func(*Options)
)

func NewRpcClientMock(opts ...Option) *RpcClientMock {
	pdr := moneyid.PDR()
	options := &Options{
		pdr:              &pdr,
		Bills:            map[string]*sdktypes.Bill{},
		FeeCreditRecords: map[string]*sdktypes.FeeCreditRecord{},
		TxProofs:         map[string]*types.TxRecordProof{},
	}
	for _, option := range opts {
		option(options)
	}
	return &RpcClientMock{
		pdr:                   options.pdr,
		Err:                   options.Err,
		RoundNumber:           options.RoundNumber,
		Bills:                 options.Bills,
		OwnerBills:            options.OwnerBills,
		FeeCreditRecords:      options.FeeCreditRecords,
		OwnerFeeCreditRecords: options.OwnerFeeCreditRecords,
		TxProofs:              options.TxProofs,
	}
}

func WithPartition(pdr *types.PartitionDescriptionRecord) Option {
	return func(o *Options) {
		o.pdr = pdr
	}
}

func WithOwnerBill(bill *sdktypes.Bill) Option {
	return func(o *Options) {
		o.Bills[string(bill.ID)] = bill
		o.OwnerBills = append(o.OwnerBills, bill)
	}
}

func WithOwnerFeeCreditRecord(fcr *sdktypes.FeeCreditRecord) Option {
	return func(o *Options) {
		o.FeeCreditRecords[string(fcr.ID)] = fcr
		o.OwnerFeeCreditRecords = append(o.OwnerFeeCreditRecords, fcr)
	}
}

func WithTxProof(txHash []byte, txProof *types.TxRecordProof) Option {
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

func (c *RpcClientMock) PartitionDescription(ctx context.Context) (*types.PartitionDescriptionRecord, error) {
	return c.pdr, nil
}

func (c *RpcClientMock) GetNodeInfo(ctx context.Context) (*sdktypes.NodeInfoResponse, error) {
	return &sdktypes.NodeInfoResponse{
		PartitionID:     0,
		PartitionTypeID: money.PartitionTypeID,
	}, nil
}

func (c *RpcClientMock) GetRoundNumber(ctx context.Context) (uint64, error) {
	if c.Err != nil {
		return 0, c.Err
	}
	return c.RoundNumber, nil
}

func (c *RpcClientMock) GetBill(ctx context.Context, unitID types.UnitID) (*sdktypes.Bill, error) {
	if c.Err != nil {
		return nil, c.Err
	}
	bill, ok := c.Bills[string(unitID)]
	if ok {
		return bill, nil
	}
	return nil, nil
}

func (c *RpcClientMock) GetBills(ctx context.Context, ownerID []byte) ([]*sdktypes.Bill, error) {
	if c.Err != nil {
		return nil, c.Err
	}
	if c.OwnerBills != nil {
		return c.OwnerBills, nil
	}
	return nil, nil
}

func (c *RpcClientMock) GetFeeCreditRecordByOwnerID(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
	if c.Err != nil {
		return nil, c.Err
	}
	if len(c.OwnerFeeCreditRecords) > 0 {
		return c.OwnerFeeCreditRecords[0], nil
	}

	return nil, nil
}

func (c *RpcClientMock) SendTransaction(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
	if c.Err != nil {
		return nil, c.Err
	}
	c.RecordedTxs = append(c.RecordedTxs, tx)
	return tx.Hash(crypto.SHA256)
}

func (c *RpcClientMock) ConfirmTransaction(ctx context.Context, tx *types.TransactionOrder, log *slog.Logger) (*types.TxRecordProof, error) {
	if c.Err != nil {
		return nil, c.Err
	}
	c.RecordedTxs = append(c.RecordedTxs, tx)
	hash, err := tx.Hash(crypto.SHA256)
	if err != nil {
		return nil, err
	}
	return c.GetTransactionProof(ctx, hash)
}

func (c *RpcClientMock) GetTransactionProof(ctx context.Context, txHash hex.Bytes) (*types.TxRecordProof, error) {
	if c.Err != nil {
		return nil, c.Err
	}
	txProof, ok := c.TxProofs[string(txHash)]
	if ok {
		return txProof, nil
	}
	// return proof for sent tx if one exists
	if len(c.RecordedTxs) > 0 {
		for _, tx := range c.RecordedTxs {
			hash, err := tx.Hash(crypto.SHA256)
			if err != nil {
				return nil, err
			}
			if bytes.Equal(txHash, hash) {
				txBytes, err := tx.MarshalCBOR()
				if err != nil {
					return nil, err
				}
				txr := &types.TransactionRecord{
					TransactionOrder: txBytes,
					ServerMetadata:   &types.ServerMetadata{ActualFee: 1, SuccessIndicator: types.TxStatusSuccessful},
				}
				return &types.TxRecordProof{
					TxRecord: txr,
					TxProof:  &types.TxProof{},
				}, nil
			}
		}
	}
	return nil, nil
}

func (c *RpcClientMock) GetBlock(ctx context.Context, blockNumber uint64) (*types.Block, error) {
	if c.Err != nil {
		return nil, c.Err
	}
	return &types.Block{}, nil
}

func (c *RpcClientMock) Close() {
	// Nothing to close
}

func NewBill(t *testing.T, value, counter uint64) *sdktypes.Bill {
	return NewLockedBill(t, value, counter, 0)
}

func NewLockedBill(t *testing.T, value uint64, counter, lockStatus uint64) *sdktypes.Bill {
	return &sdktypes.Bill{
		PartitionID: money.DefaultPartitionID,
		ID:          moneyid.NewBillID(t),
		Value:       value,
		LockStatus:  lockStatus,
		Counter:     counter,
	}
}

func NewMoneyFCR(t *testing.T, pubKeyHash []byte, balance uint64, lockStatus uint64, counter uint64) *sdktypes.FeeCreditRecord {
	pdr := moneyid.PDR()
	id, err := money.NewFeeCreditRecordIDFromPublicKeyHash(&pdr, types.ShardID{}, pubKeyHash, 1000+transferFCLatestAdditionTime)
	require.NoError(t, err)
	return &sdktypes.FeeCreditRecord{
		PartitionID: money.DefaultPartitionID,
		ID:          id,
		Balance:     balance,
		LockStatus:  lockStatus,
		Counter:     &counter,
	}

}
