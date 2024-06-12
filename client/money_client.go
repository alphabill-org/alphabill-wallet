package client

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill-wallet/client/rpc"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
)

type (
	moneyPartitionClient struct {
		*rpc.AdminAPIClient
		*rpc.StateAPIClient
	}
)

// NewMoneyPartitionClient creates a money partition client for the given RPC URL.
func NewMoneyPartitionClient(ctx context.Context, rpcUrl string) (sdktypes.MoneyPartitionClient, error) {
	// TODO: duplicate underlying rpc clients, could use one?
	stateApiClient, err := rpc.NewStateAPIClient(ctx, rpcUrl)
	if err != nil {
		return nil, err
	}
	adminApiClient, err := rpc.NewAdminAPIClient(ctx, rpcUrl)
	if err != nil {
		return nil, err
	}

	return &moneyPartitionClient{
		AdminAPIClient: adminApiClient,
		StateAPIClient: stateApiClient,
	}, nil
}

// GetBill returns bill for the given bill id.
// Returns api.ErrNotFound if the bill does not exist.
func (c *moneyPartitionClient) GetBill(ctx context.Context, unitID types.UnitID) (*sdktypes.Bill, error) {
	var u *sdktypes.Unit[money.BillData]
	if err := c.RpcClient.CallContext(ctx, &u, "state_getUnit", unitID, false); err != nil {
		return nil, err
	}
	if u == nil {
		return nil, nil
	}

	return &sdktypes.Bill{
		ID:   u.UnitID,
		Data: &u.Data,
	}, nil
}

func (c *moneyPartitionClient) GetBills(ctx context.Context, ownerID []byte) ([]*sdktypes.Bill, error) {
	unitIDs, err := c.GetUnitsByOwnerID(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch owner units: %w", err)
	}
	var bills []*sdktypes.Bill
	for _, unitID := range unitIDs {
		if !unitID.HasType(money.BillUnitType) {
			continue
		}
		bill, err := c.GetBill(ctx, unitID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch unit: %w", err)
		}
		bills = append(bills, bill)
	}
	return bills, nil
}

// GetFeeCreditRecordByOwnerID finds the first fee credit record in money partition for the given owner ID,
// returns nil if fee credit record does not exist.
func (c *moneyPartitionClient) GetFeeCreditRecordByOwnerID(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
	return c.StateAPIClient.GetFeeCreditRecordByOwnerID(ctx, ownerID, money.FeeCreditRecordUnitType)
}

func (c *moneyPartitionClient) ConfirmTransaction(ctx context.Context, tx *types.TransactionOrder, log *slog.Logger) (*sdktypes.Proof, error) {
	txBatch := txsubmitter.New(tx).ToBatch(c, log)
	err := txBatch.SendTx(ctx, true)
	if err != nil {
		return nil, err
	}
	return txBatch.Submissions()[0].Proof, nil
}

func (c *moneyPartitionClient) Close() {
	c.AdminAPIClient.Close()
	c.StateAPIClient.Close()
}
