package rpc

import (
	"context"

	"github.com/alphabill-org/alphabill-wallet/client/types"
)

// AdminAPIClient defines typed wrappers for the Alphabill admin RPC API.
type AdminAPIClient struct {
	rpcClient *Client
}

// NewAdminAPIClient creates a new admin API client connected to the given URL.
func NewAdminAPIClient(ctx context.Context, rpcClient *Client) (*AdminAPIClient, error) {
	return &AdminAPIClient{rpcClient}, nil
}

// GetNodeInfo returns status info of the rpc node.
func (c *AdminAPIClient) GetNodeInfo(ctx context.Context) (*types.NodeInfoResponse, error) {
	var res *types.NodeInfoResponse
	err := c.rpcClient.CallContext(ctx, &res, "admin_getNodeInfo")
	return res, err
}
