package rpc

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
)

type (
	// StateAPIClient defines typed wrappers for the Alphabill State RPC API.
	StateAPIClient struct {
		rpcClient *Client
	}
)

// NewStateAPIClient creates a new state API client connected to the given URL.
func NewStateAPIClient(ctx context.Context, rpcClient *Client) (*StateAPIClient, error) {
	return &StateAPIClient{rpcClient}, nil
}

// GetRoundInfo returns the latest round number and epoch seen by the rpc node.
func (c *StateAPIClient) GetRoundInfo(ctx context.Context) (*sdktypes.RoundInfo, error) {
	var res *sdktypes.RoundInfo
	err := c.rpcClient.CallContext(ctx, &res, "state_getRoundInfo")
	return res, err
}

// GetUnitsByOwnerID returns list of unit identifiers that belong to the given owner.
func (c *StateAPIClient) GetUnitsByOwnerID(ctx context.Context, ownerID hex.Bytes) ([]types.UnitID, error) {
	var res []types.UnitID
	err := c.rpcClient.CallContext(ctx, &res, "state_getUnitsByOwnerID", ownerID)
	return res, err
}

func (c *StateAPIClient) GetUnit(ctx context.Context, res interface{}, unitID types.UnitID, includeStateProof bool) error {
	return c.rpcClient.CallContext(ctx, res, "state_getUnit", unitID, includeStateProof)
}

// SendTransaction sends the given transaction order to the connected node.
// Returns the submitted transaction order hash.
func (c *StateAPIClient) SendTransaction(ctx context.Context, txo *types.TransactionOrder) ([]byte, error) {
	txoCBOR, err := encodeCbor(txo)
	if err != nil {
		return nil, fmt.Errorf("failed to encode transaction order to cbor: %w", err)
	}
	var res hex.Bytes
	err = c.rpcClient.CallContext(ctx, &res, "state_sendTransaction", txoCBOR)

	return res, err
}

// GetTransactionProof returns transaction record and proof for the given transaction hash.
// Returns ErrNotFound if proof was not found.
func (c *StateAPIClient) GetTransactionProof(ctx context.Context, txHash hex.Bytes) (*types.TxRecordProof, error) {
	var res *sdktypes.TransactionRecordAndProof
	err := c.rpcClient.CallContext(ctx, &res, "state_getTransactionProof", txHash)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	return res.ToBaseType()
}

// GetBlock returns block for the given round number.
// Returns ErrNotFound if the block does not exist.
func (c *StateAPIClient) GetBlock(ctx context.Context, roundNumber uint64) (*types.Block, error) {
	var res hex.Bytes
	if err := c.rpcClient.CallContext(ctx, &res, "state_getBlock", hex.Uint64(roundNumber)); err != nil {
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

// GetUnits returns list of all unit identifiers optionally filtered by type identifier.
// This request needs to be explicitly enabled on the validator node.
func (c *StateAPIClient) GetUnits(ctx context.Context, unitTypeID *uint32) ([]types.UnitID, error) {
	var res []types.UnitID
	err := c.rpcClient.CallContext(ctx, &res, "state_getUnits", unitTypeID)
	return res, err
}

func encodeCbor(v interface{}) (hex.Bytes, error) {
	data, err := types.Cbor.Marshal(v)
	if err != nil {
		return nil, err
	}
	return data, nil
}
