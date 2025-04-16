package client

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-base/types"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
)

type (
	orchestrationPartitionClient struct {
		*partitionClient
	}
)

// NewOrchestrationPartitionClient creates an orchestration partition client for the given RPC URL.
func NewOrchestrationPartitionClient(ctx context.Context, rpcUrl string) (sdktypes.PartitionClient, error) {
	partitionClient, err := newPartitionClient(ctx, rpcUrl, orchestration.PartitionTypeID)
	if err != nil {
		return nil, err
	}
	return &orchestrationPartitionClient{
		partitionClient: partitionClient,
	}, nil
}

// GetFeeCreditRecordByOwnerID finds the first fee credit record in orchestration partition for the given owner ID.
// Returns nil if fee credit record does not exist.
func (c *orchestrationPartitionClient) GetFeeCreditRecordByOwnerID(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
	// No FeeCreditRecords in orchestration partition, yet?
	return nil, nil
}

func (c *orchestrationPartitionClient) ConfirmTransaction(ctx context.Context, tx *types.TransactionOrder, log *slog.Logger) (*types.TxRecordProof, error) {
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
