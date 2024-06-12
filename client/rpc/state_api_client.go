package rpc

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/ethereum/go-ethereum/rpc"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
)

type (
	// StateAPIClient defines typed wrappers for the Alphabill State RPC API.
	StateAPIClient struct {
		RpcClient *rpc.Client
	}
)

// NewStateAPIClient creates a new state API client connected to the given URL.
func NewStateAPIClient(ctx context.Context, url string) (*StateAPIClient, error) {
	rpcClient, err := rpc.DialContext(ctx, url)
	if err != nil {
		return nil, err
	}
	return &StateAPIClient{rpcClient}, nil
}

// Close closes the underlying RPC connection.
func (c *StateAPIClient) Close() {
	c.RpcClient.Close()
}

// GetRoundNumber returns the latest round number seen by the rpc node.
func (c *StateAPIClient) GetRoundNumber(ctx context.Context) (uint64, error) {
	var num types.Uint64
	err := c.RpcClient.CallContext(ctx, &num, "state_getRoundNumber")
	return uint64(num), err
}

// GetUnitsByOwnerID returns list of unit identifiers that belong to the given owner.
func (c *StateAPIClient) GetUnitsByOwnerID(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error) {
	var res []types.UnitID
	err := c.RpcClient.CallContext(ctx, &res, "state_getUnitsByOwnerID", ownerID)
	return res, err
}

// getFeeCreditRecord returns fee credit record for the given unit ID.
// Returns api.ErrNotFound if the fee credit bill does not exist.
func (c *StateAPIClient) getFeeCreditRecord(ctx context.Context, unitID types.UnitID) (*sdktypes.FeeCreditRecord, error) {
	var u *sdktypes.Unit[fc.FeeCreditRecord]
	if err := c.RpcClient.CallContext(ctx, &u, "state_getUnit", unitID, false); err != nil {
		return nil, err
	}
	if u == nil {
		return nil, nil
	}
	return &sdktypes.FeeCreditRecord{
		ID:   u.UnitID,
		Data: &u.Data,
	}, nil
}

// GetFeeCreditRecordByOwnerID finds the first fee credit record that belongs to the given owner identifier,
// returns nil if not found.
func (c *StateAPIClient) GetFeeCreditRecordByOwnerID(ctx context.Context, ownerID, fcrUnitType []byte) (*sdktypes.FeeCreditRecord, error) {
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

// SendTransaction sends the given transaction to the connected node.
// Returns the submitted transaction hash.
func (c *StateAPIClient) SendTransaction(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
	txCbor, err := encodeCbor(tx)
	if err != nil {
		return nil, fmt.Errorf("failed to encode transaction to cbor: %w", err)
	}
	var res types.Bytes
	err = c.RpcClient.CallContext(ctx, &res, "state_sendTransaction", txCbor)

	return res, err
}

// GetTransactionProof returns transaction record and proof for the given transaction hash.
// Returns ErrNotFound if proof was not found.
func (c *StateAPIClient) GetTransactionProof(ctx context.Context, txHash types.Bytes) (*sdktypes.Proof, error) {
	var res *sdktypes.TransactionRecordAndProof
	err := c.RpcClient.CallContext(ctx, &res, "state_getTransactionProof", txHash)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	var txRecord *types.TransactionRecord
	if err = types.Cbor.Unmarshal(res.TxRecord, &txRecord); err != nil {
		return nil, fmt.Errorf("failed to decode tx record: %w", err)
	}
	var txProof *types.TxProof
	if err = types.Cbor.Unmarshal(res.TxProof, &txProof); err != nil {
		return nil, fmt.Errorf("failed to decode tx proof: %w", err)
	}
	return &sdktypes.Proof{
		TxRecord: txRecord,
		TxProof: txProof,
	}, nil
}

// GetBlock returns block for the given round number.
// Returns ErrNotFound if the block does not exist.
func (c *StateAPIClient) GetBlock(ctx context.Context, roundNumber uint64) (*types.Block, error) {
	var res types.Bytes
	if err := c.RpcClient.CallContext(ctx, &res, "state_getBlock", types.Uint64(roundNumber)); err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	var block *types.Block
	if err := types.Cbor.Unmarshal(res, &block); err != nil {
		return nil, fmt.Errorf("failed to decode block: %w", err)
	}
	return block, nil
}

func encodeCbor(v interface{}) (types.Bytes, error) {
	data, err := types.Cbor.Marshal(v)
	if err != nil {
		return nil, err
	}
	return data, nil
}
