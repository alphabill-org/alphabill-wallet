package rpc

import (
	"context"

	"github.com/ethereum/go-ethereum/rpc"
)

// Client defines typed wrappers for the Alphabill RPC API.
type Client struct {
	c *rpc.Client
}

// DialContext connects a client to the given URL with context.
func DialContext(ctx context.Context, url string) (*Client, error) {
	c, err := rpc.DialContext(ctx, url)
	if err != nil {
		return nil, err
	}
	return NewClient(c), nil
}

// NewClient creates a client that uses the given RPC client.
func NewClient(c *rpc.Client) *Client {
	return &Client{c}
}

// Close closes the underlying RPC connection.
func (c *Client) Close() {
	c.c.Close()
}

// Client gets the underlying RPC client.
func (c *Client) Client() *rpc.Client {
	return c.c
}

// GetRoundNumber returns the latest round number seen by the rpc node.
func (c *Client) GetRoundNumber(ctx context.Context) (uint64, error) {
	var num uint64
	err := c.c.CallContext(ctx, &num, "state_getRoundNumber")
	return num, err
}
