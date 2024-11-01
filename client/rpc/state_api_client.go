package rpc

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
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
	var num hex.Uint64
	err := c.RpcClient.CallContext(ctx, &num, "state_getRoundNumber")
	return uint64(num), err
}

// GetUnitsByOwnerID returns list of unit identifiers that belong to the given owner.
func (c *StateAPIClient) GetUnitsByOwnerID(ctx context.Context, ownerID hex.Bytes) ([]types.UnitID, error) {
	var res []types.UnitID
	err := c.RpcClient.CallContext(ctx, &res, "state_getUnitsByOwnerID", ownerID)
	return res, err
}

// SendTransaction sends the given transaction order to the connected node.
// Returns the submitted transaction order hash.
func (c *StateAPIClient) SendTransaction(ctx context.Context, txo *types.TransactionOrder) ([]byte, error) {
	txoCBOR, err := encodeCbor(txo)
	if err != nil {
		return nil, fmt.Errorf("failed to encode transaction order to cbor: %w", err)
	}
	var res hex.Bytes
	err = c.RpcClient.CallContext(ctx, &res, "state_sendTransaction", txoCBOR)

	return res, err
}

// GetTransactionProof returns transaction record and proof for the given transaction hash.
// Returns ErrNotFound if proof was not found.
func (c *StateAPIClient) GetTransactionProof(ctx context.Context, txHash hex.Bytes) (*types.TxRecordProof, error) {
	var res *sdktypes.TransactionRecordAndProof
	err := c.RpcClient.CallContext(ctx, &res, "state_getTransactionProof", txHash)
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
	if err := c.RpcClient.CallContext(ctx, &res, "state_getBlock", hex.Uint64(roundNumber)); err != nil {
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

func encodeCbor(v interface{}) (hex.Bytes, error) {
	data, err := types.Cbor.Marshal(v)
	if err != nil {
		return nil, err
	}
	return data, nil
}