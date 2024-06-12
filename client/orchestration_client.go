package client

import (
	"context"
	"log/slog"

	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill-wallet/client/rpc"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
)

type (
	orchestrationPartitionClient struct {
		*rpc.AdminAPIClient
		*rpc.StateAPIClient
	}
)

// NewOrchestrationPartitionClient creates an orchestration partition client for the given RPC URL.
func NewOrchestrationPartitionClient(ctx context.Context, rpcUrl string) (sdktypes.PartitionClient, error) {
	adminApiClient, err := rpc.NewAdminAPIClient(ctx, rpcUrl)
	if err != nil {
		return nil, err
	}
	stateApiClient, err := rpc.NewStateAPIClient(ctx, rpcUrl)
	if err != nil {
		return nil, err
	}
	return &orchestrationPartitionClient{
		AdminAPIClient: adminApiClient,
		StateAPIClient: stateApiClient,
	}, nil
}

// GetFeeCreditRecord finds the first fee credit record in orchestration partition for the given owner ID.
// Returns nil if fee credit record does not exist.
func (c *orchestrationPartitionClient) GetFeeCreditRecordByOwnerID(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
	// No FeeCreditRecords in orchestration partion, yet?
	return nil, nil
}

func (c *orchestrationPartitionClient) ConfirmTransaction(ctx context.Context, tx *types.TransactionOrder, log *slog.Logger) (*sdktypes.Proof, error) {
	txBatch := txsubmitter.New(tx).ToBatch(c, log)
	err := txBatch.SendTx(ctx, true)
	if err != nil {
		return nil, err
	}
	return txBatch.Submissions()[0].Proof, nil
}

func (c *orchestrationPartitionClient) Close() {
	c.AdminAPIClient.Close()
	c.StateAPIClient.Close()
}
