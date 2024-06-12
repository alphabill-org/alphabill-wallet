package rpc

import (
	"context"

	"github.com/ethereum/go-ethereum/rpc"

	"github.com/alphabill-org/alphabill-wallet/client/types"
)

// AdminAPIClient defines typed wrappers for the Alphabill admin RPC API.
type AdminAPIClient struct {
	rpcClient *rpc.Client
}

// NewAdminAPIClient creates a new admin API client connected to the given URL.
func NewAdminAPIClient(ctx context.Context, url string) (*AdminAPIClient, error) {
	rpcClient, err := rpc.DialContext(ctx, url)
	if err != nil {
		return nil, err
	}

	return &AdminAPIClient{rpcClient}, nil
}

// Close closes the underlying RPC connection.
func (c *AdminAPIClient) Close() {
	c.rpcClient.Close()
}

// GetNodeInfo returns status info of the rpc node.
func (c *AdminAPIClient) GetNodeInfo(ctx context.Context) (*types.NodeInfoResponse, error) {
	var res *types.NodeInfoResponse
	err := c.rpcClient.CallContext(ctx, &res, "admin_getNodeInfo")
	return res, err
}
