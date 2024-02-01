package rpc

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/types"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/fxamacker/cbor/v2"
)

// Client defines typed wrappers for the Alphabill RPC API.
type Client struct {
	c *ethrpc.Client
}

// DialContext connects a client to the given URL with context.
func DialContext(ctx context.Context, url string) (*Client, error) {
	c, err := ethrpc.DialContext(ctx, url)
	if err != nil {
		return nil, err
	}
	return NewClient(c), nil
}

// NewClient creates a client that uses the given RPC client.
func NewClient(c *ethrpc.Client) *Client {
	return &Client{c}
}

// Close closes the underlying RPC connection.
func (c *Client) Close() {
	c.c.Close()
}

// Client gets the underlying RPC client.
func (c *Client) Client() *ethrpc.Client {
	return c.c
}

// GetRoundNumber returns the latest round number seen by the rpc node.
func (c *Client) GetRoundNumber(ctx context.Context) (uint64, error) {
	var num uint64
	err := c.c.CallContext(ctx, &num, "state_getRoundNumber")
	return num, err
}

// GetUnit returns the unit data for given unitID.
func (c *Client) GetUnit(ctx context.Context, unitID []byte, returnProof, returnData bool) (*types.UnitDataAndProof, error) {
	var res *rpc.UnitData
	if err := c.c.CallContext(ctx, &res, "state_getUnit", unitID, returnProof, returnData); err != nil {
		return nil, err
	}
	var unitData *types.UnitDataAndProof
	if err := cbor.Unmarshal(res.DataAndProofCBOR, &unitData); err != nil {
		return nil, fmt.Errorf("failed to decode unit data cbor: %w", err)
	}
	return unitData, nil
}

// GetUnitsByOwnerID returns list of unit identifiers that belong to the given owner.
func (c *Client) GetUnitsByOwnerID(ctx context.Context, ownerID []byte) ([]types.UnitID, error) {
	var res []types.UnitID
	err := c.c.CallContext(ctx, &res, "state_getUnitsByOwnerID", ownerID)
	return res, err
}

// SendTransaction broadcasts the given transaction to the network, returns the submitted transaction hash.
func (c *Client) SendTransaction(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
	txCbor, err := encodeCbor(tx)
	if err != nil {
		return nil, fmt.Errorf("failed to encode transaction to cbor: %w", err)
	}
	var res []byte
	err = c.c.CallContext(ctx, &res, "state_sendTransaction", &rpc.Transaction{TxOrderCbor: txCbor})
	return res, err
}

// GetTransactionProof returns transaction record and proof for the given transaction hash.
func (c *Client) GetTransactionProof(ctx context.Context, txHash []byte) (*types.TransactionRecord, *types.TxProof, error) {
	var res *rpc.TransactionRecordAndProof
	err := c.c.CallContext(ctx, &res, "state_getTransactionProof", txHash)
	if err != nil {
		return nil, nil, err
	}
	var txRecord *types.TransactionRecord
	if err = cbor.Unmarshal(res.TxRecordCbor, &txRecord); err != nil {
		return nil, nil, fmt.Errorf("failed to decode tx record: %w", err)
	}
	var txProof *types.TxProof
	if err = cbor.Unmarshal(res.TxProofCbor, &txProof); err != nil {
		return nil, nil, fmt.Errorf("failed to decode tx proof: %w", err)
	}
	return txRecord, txProof, nil
}

// GetBlock returns block for given round number.
func (c *Client) GetBlock(ctx context.Context, roundNumber uint64) (*types.Block, error) {
	var res *rpc.Block
	if err := c.c.CallContext(ctx, &res, "state_getBlock", roundNumber); err != nil {
		return nil, err
	}
	var block *types.Block
	if err := cbor.Unmarshal(res.BlockCbor, &block); err != nil {
		return nil, fmt.Errorf("failed to decode block: %w", err)
	}
	return block, nil
}

func encodeCbor(v interface{}) ([]byte, error) {
	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return nil, err
	}
	data, err := enc.Marshal(v)
	if err != nil {
		return nil, err
	}
	return data, nil
}
