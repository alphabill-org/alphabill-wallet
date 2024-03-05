package rpc

import (
	"context"

	"github.com/alphabill-org/alphabill/rpc"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
)

// AdminClient defines typed wrappers for the Alphabill RPC admin service API.
type AdminClient struct {
	c *ethrpc.Client
}

// NewAdminClient creates admin client that uses the given RPC client.
func NewAdminClient(c *ethrpc.Client) *AdminClient {
	return &AdminClient{c}
}

// Close closes the underlying RPC connection.
func (c *AdminClient) Close() {
	c.c.Close()
}

// Client gets the underlying RPC client.
func (c *AdminClient) Client() *ethrpc.Client {
	return c.c
}

// GetNodeInfo returns status info of the rpc node.
func (c *AdminClient) GetNodeInfo(ctx context.Context) (*rpc.NodeInfoResponse, error) {
	var res *rpc.NodeInfoResponse
	err := c.c.CallContext(ctx, &res, "admin_getNodeInfo")
	return res, err
}
