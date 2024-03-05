package rpc

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill-wallet/wallet/money/api"
	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/txsystem/money"
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

// GetBill returns bill for the given bill id.
// Returns api.ErrNotFound if the bill does not exist.
func (c *Client) GetBill(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.Bill, error) {
	var u *rpc.Unit[money.BillData]
	if err := c.c.CallContext(ctx, &u, "state_getUnit", unitID, includeStateProof); err != nil {
		return nil, err
	}
	if u == nil {
		return nil, api.ErrNotFound
	}

	return &api.Bill{
		ID:       u.UnitID,
		BillData: &u.Data,
	}, nil
}

// GetFeeCreditRecord returns fee credit bill for the given bill id.
// Returns api.ErrNotFound if the fee credit bill does not exist.
func (c *Client) GetFeeCreditRecord(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.FeeCreditBill, error) {
	var u *rpc.Unit[unit.FeeCreditRecord]
	if err := c.c.CallContext(ctx, &u, "state_getUnit", unitID, includeStateProof); err != nil {
		return nil, err
	}
	if u == nil {
		return nil, api.ErrNotFound
	}
	return &api.FeeCreditBill{
		ID:              u.UnitID,
		FeeCreditRecord: &u.Data,
	}, nil
}

// GetUnitsByOwnerID returns list of unit identifiers that belong to the given owner.
func (c *Client) GetUnitsByOwnerID(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error) {
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
	var res types.Bytes
	err = c.c.CallContext(ctx, &res, "state_sendTransaction", txCbor)
	return res, err
}

// GetTransactionProof returns transaction record and proof for the given transaction hash.
// Returns api.ErrNotFound if proof was not found.
func (c *Client) GetTransactionProof(ctx context.Context, txHash types.Bytes) (*types.TransactionRecord, *types.TxProof, error) {
	var res *rpc.TransactionRecordAndProof
	err := c.c.CallContext(ctx, &res, "state_getTransactionProof", txHash)
	if err != nil {
		return nil, nil, err
	}
	if res == nil {
		return nil, nil, api.ErrNotFound
	}
	var txRecord *types.TransactionRecord
	if err = cbor.Unmarshal(res.TxRecord, &txRecord); err != nil {
		return nil, nil, fmt.Errorf("failed to decode tx record: %w", err)
	}
	var txProof *types.TxProof
	if err = cbor.Unmarshal(res.TxProof, &txProof); err != nil {
		return nil, nil, fmt.Errorf("failed to decode tx proof: %w", err)
	}
	return txRecord, txProof, nil
}

// GetBlock returns block for the given round number.
// Returns ErrNotFound if the block does not exist.
func (c *Client) GetBlock(ctx context.Context, roundNumber uint64) (*types.Block, error) {
	var res types.Bytes
	if err := c.c.CallContext(ctx, &res, "state_getBlock", roundNumber); err != nil {
		return nil, err
	}
	if res == nil {
		return nil, api.ErrNotFound
	}
	var block *types.Block
	if err := cbor.Unmarshal(res, &block); err != nil {
		return nil, fmt.Errorf("failed to decode block: %w", err)
	}
	return block, nil
}

func encodeCbor(v interface{}) (types.Bytes, error) {
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
