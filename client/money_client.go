package client

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/ethereum/go-ethereum/rpc"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
)

type moneyPartitionClient struct {
	*partitionClient
}

// NewMoneyPartitionClient creates a money partition client for the given RPC URL.
func NewMoneyPartitionClient(ctx context.Context, rpcUrl string, opts ...Option) (sdktypes.MoneyPartitionClient, error) {
	partitionClient, err := newPartitionClient(ctx, rpcUrl, money.PartitionTypeID, opts...)
	if err != nil {
		return nil, err
	}

	return &moneyPartitionClient{
		partitionClient: partitionClient,
	}, nil
}

// GetBill returns bill for the given bill id.
// Returns nil,nil if the bill does not exist.
func (c *moneyPartitionClient) GetBill(ctx context.Context, unitID types.UnitID) (*sdktypes.Bill, error) {
	var u *sdktypes.Unit[money.BillData]
	if err := c.GetUnit(ctx, &u, unitID, false); err != nil {
		return nil, err
	}
	if u == nil {
		return nil, nil
	}
	return &sdktypes.Bill{
		NetworkID:   u.NetworkID,
		PartitionID: u.PartitionID,
		ID:          u.UnitID,
		Value:       u.Data.Value,
		Counter:     u.Data.Counter,
		StateLockTx: u.StateLockTx,
	}, nil
}

func (c *moneyPartitionClient) GetBills(ctx context.Context, ownerID []byte) ([]*sdktypes.Bill, error) {
	unitIDs, err := c.GetUnitsByOwnerID(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch owner units: %w", err)
	}
	var bills []*sdktypes.Bill
	var batch []*rpc.BatchElem
	for _, unitID := range unitIDs {
		if unitID.TypeMustBe(money.BillUnitType, c.pdr) != nil {
			continue
		}

		var u sdktypes.Unit[money.BillData]
		batch = append(batch, &rpc.BatchElem{
			Method: "state_getUnit",
			Args:   []any{unitID, false},
			Result: &u,
		})
	}
	if len(batch) == 0 {
		return bills, nil
	}
	if err := c.rpcClient.BatchCall(ctx, batch); err != nil {
		return nil, fmt.Errorf("failed to fetch bills: %w", err)
	}
	for _, batchElem := range batch {
		if batchElem.Error != nil {
			return nil, fmt.Errorf("failed to fetch bill: %w", batchElem.Error)
		}
		u := batchElem.Result.(*sdktypes.Unit[money.BillData])
		bills = append(bills, &sdktypes.Bill{
			NetworkID:   u.NetworkID,
			PartitionID: u.PartitionID,
			ID:          u.UnitID,
			Value:       u.Data.Value,
			Counter:     u.Data.Counter,
			StateLockTx: u.StateLockTx,
		})
	}

	return bills, nil
}

// GetFeeCreditRecordByOwnerID finds the first fee credit record in money partition for the given owner ID,
// returns nil,nil if fee credit record does not exist.
func (c *moneyPartitionClient) GetFeeCreditRecordByOwnerID(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
	return c.getFeeCreditRecordByOwnerID(ctx, ownerID, money.FeeCreditRecordUnitType)
}

func (c *moneyPartitionClient) ConfirmTransaction(ctx context.Context, tx *types.TransactionOrder, log *slog.Logger) (*types.TxRecordProof, error) {
	sub, err := txsubmitter.New(tx)
	if err != nil {
		return nil, fmt.Errorf("failed to create tx submission: %w", err)
	}
	txBatch := sub.ToBatch(c, log)

	if err := txBatch.SendTx(ctx, true); err != nil {
		return nil, err
	}
	return txBatch.Submissions()[0].Proof, nil
}
