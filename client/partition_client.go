package client

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill-wallet/client/rpc"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
)

type (
	partitionClient struct {
		*rpc.AdminAPIClient
		*rpc.StateAPIClient
	}
)

// newPartitionClient creates a generic partition client for the given RPC URL.
func newPartitionClient(ctx context.Context, rpcUrl string) (*partitionClient, error) {
	// TODO: duplicate underlying rpc clients, could use one?
	stateApiClient, err := rpc.NewStateAPIClient(ctx, rpcUrl)
	if err != nil {
		return nil, err
	}
	adminApiClient, err := rpc.NewAdminAPIClient(ctx, rpcUrl)
	if err != nil {
		return nil, err
	}

	return &partitionClient{
		AdminAPIClient: adminApiClient,
		StateAPIClient: stateApiClient,
	}, nil
}

// GetFeeCreditRecordByOwnerID finds the first fee credit record in money partition for the given owner ID,
// returns nil,nil if fee credit record does not exist.
func (c *partitionClient) getFeeCreditRecordByOwnerID(ctx context.Context, ownerID, fcrUnitType []byte) (sdktypes.FeeCreditRecord, error) {
	unitIDs, err := c.GetUnitsByOwnerID(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch units: %w", err)
	}
	for _, unitID := range unitIDs {
		if unitID.HasType(fcrUnitType) {
			return c.getFeeCreditRecord(ctx, unitID)
		}
	}
	return nil, nil
}

// getFeeCreditRecord returns the fee credit record for the given unit ID.
// Returns nil,nil if the fee credit record does not exist.
func (c *partitionClient) getFeeCreditRecord(ctx context.Context, unitID types.UnitID) (sdktypes.FeeCreditRecord, error) {
	var u *sdktypes.Unit[fc.FeeCreditRecord]
	if err := c.RpcClient.CallContext(ctx, &u, "state_getUnit", unitID, false); err != nil {
		return nil, err
	}
	if u == nil {
		return nil, nil
	}

	counterCopy := u.Data.Counter
	return &feeCreditRecord{
		systemID:   0, // TODO:
		id:         u.UnitID,
		balance:    u.Data.Balance,
		counter:    &counterCopy,
		timeout:    u.Data.Timeout,
		lockStatus: u.Data.Locked,
	}, nil
}

func (c *partitionClient) Close() {
	c.AdminAPIClient.Close()
	c.StateAPIClient.Close()
}
