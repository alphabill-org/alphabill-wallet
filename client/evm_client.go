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
	evmPartitionClient struct {
		*rpc.AdminAPIClient
		*rpc.StateAPIClient
	}
)

// NewEvmPartitionClient creates an evm partition client for the given RPC URL.
func NewEvmPartitionClient(ctx context.Context, rpcUrl string) (sdktypes.PartitionClient, error) {
	adminApiClient, err := rpc.NewAdminAPIClient(ctx, rpcUrl)
	if err != nil {
		return nil, err
	}
	stateApiClient, err := rpc.NewStateAPIClient(ctx, rpcUrl)
	if err != nil {
		return nil, err
	}
	return &evmPartitionClient{
		AdminAPIClient: adminApiClient,
		StateAPIClient: stateApiClient,
	}, nil
}

// GetFeeCreditRecordByOwnerID finds the first fee credit record in evm partition for the given owner ID,
// returns nil if fee credit record does not exist.
func (c *evmPartitionClient) GetFeeCreditRecordByOwnerID(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
	return nil, nil
}

func (c *evmPartitionClient) ConfirmTransaction(ctx context.Context, tx *types.TransactionOrder, log *slog.Logger) (*sdktypes.Proof, error) {
	txBatch := txsubmitter.New(tx).ToBatch(c, log)
	err := txBatch.SendTx(ctx, true)
	if err != nil {
		return nil, err
	}
	return txBatch.Submissions()[0].Proof, nil
}

func (c *evmPartitionClient) Close() {
	c.AdminAPIClient.Close()
	c.StateAPIClient.Close()
}
