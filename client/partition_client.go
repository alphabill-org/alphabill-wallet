package client

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	ethrpc "github.com/ethereum/go-ethereum/rpc"

	"github.com/alphabill-org/alphabill-wallet/client/rpc"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
)

const defaultBatchItemLimit int = 100

type (
	partitionClient struct {
		*rpc.AdminAPIClient
		*rpc.StateAPIClient
		pdr *types.PartitionDescriptionRecord

		batchItemLimit int
	}

	Options struct {
		BatchItemLimit int
	}

	Option func(*Options)
)

func WithBatchItemLimit(batchItemLimit int) Option {
	return func(os *Options) {
		os.BatchItemLimit = max(batchItemLimit, 1)
	}
}

// newPartitionClient creates a generic partition client for the given RPC URL.
func newPartitionClient(ctx context.Context, rpcUrl string, kind types.PartitionTypeID, opts ...Option) (*partitionClient, error) {
	// TODO: duplicate underlying rpc clients, could use one?
	stateApiClient, err := rpc.NewStateAPIClient(ctx, rpcUrl)
	if err != nil {
		return nil, err
	}
	adminApiClient, err := rpc.NewAdminAPIClient(ctx, rpcUrl)
	if err != nil {
		return nil, err
	}
	// TODO: load PDR from backend! (AB-1800)
	info, err := adminApiClient.GetNodeInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("requesting node info: %w", err)
	}
	if info.PartitionTypeID != kind {
		return nil, fmt.Errorf("expected node partition type %x but it is %x", kind, info.PartitionTypeID)
	}

	o := optionsWithDefaults(opts)
	return &partitionClient{
		AdminAPIClient: adminApiClient,
		StateAPIClient: stateApiClient,
		// TODO: load full PDR from backend! (AB-1800)
		pdr: &types.PartitionDescriptionRecord{
			NetworkID:       info.NetworkID,
			PartitionID:     info.PartitionID,
			PartitionTypeID: info.PartitionTypeID,
			UnitIDLen:       256,
			TypeIDLen:       8,
		},

		batchItemLimit: o.BatchItemLimit,
	}, nil
}

func (c *partitionClient) PartitionDescription(ctx context.Context) (*types.PartitionDescriptionRecord, error) {
	return c.pdr, nil
}

// GetFeeCreditRecordByOwnerID finds the first fee credit record in money partition for the given owner ID,
// returns nil,nil if fee credit record does not exist.
func (c *partitionClient) getFeeCreditRecordByOwnerID(ctx context.Context, ownerID []byte, fcrUnitType uint32) (*sdktypes.FeeCreditRecord, error) {
	unitIDs, err := c.GetUnitsByOwnerID(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch units: %w", err)
	}
	for _, unitID := range unitIDs {
		if unitID.TypeMustBe(fcrUnitType, c.pdr) == nil {
			return c.GetFeeCreditRecord(ctx, unitID)
		}
	}
	return nil, nil
}

// GetFeeCreditRecord returns the fee credit record for the given unit ID.
// Returns nil, nil if the fee credit record does not exist.
func (c *partitionClient) GetFeeCreditRecord(ctx context.Context, unitID types.UnitID) (*sdktypes.FeeCreditRecord, error) {
	var u *sdktypes.Unit[fc.FeeCreditRecord]
	if err := c.RpcClient.CallContext(ctx, &u, "state_getUnit", unitID, false); err != nil {
		return nil, err
	}
	if u == nil {
		return nil, nil
	}

	counterCopy := u.Data.Counter
	return &sdktypes.FeeCreditRecord{
		NetworkID:      u.NetworkID,
		PartitionID:    u.PartitionID,
		ID:             u.UnitID,
		Balance:        u.Data.Balance,
		OwnerPredicate: u.Data.OwnerPredicate,
		MinLifetime:    u.Data.MinLifetime,
		StateLockTx:    u.StateLockTx,
		Counter:        &counterCopy,
	}, nil
}

func (c *partitionClient) batchCallWithLimit(ctx context.Context, batch []ethrpc.BatchElem) error {
	start, end := 0, 0
	for len(batch) > end {
		end = min(len(batch), start+c.batchItemLimit)
		if err := c.RpcClient.BatchCallContext(ctx, batch[start:end]); err != nil {
			return fmt.Errorf("failed to send batch request: %w", err)
		}
		start = end
	}
	return nil
}

func (c *partitionClient) Close() {
	c.AdminAPIClient.Close()
	c.StateAPIClient.Close()
}

func optionsWithDefaults(opts []Option) *Options {
	res := &Options{
		BatchItemLimit: defaultBatchItemLimit,
	}
	for _, opt := range opts {
		opt(res)
	}
	return res
}
