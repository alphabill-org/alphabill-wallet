package rpc

import (
	"context"
	"testing"

	"github.com/alphabill-org/alphabill-go-sdk/types"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/client/rpc/mocksrv"
)

func TestAdminClient(t *testing.T) {
	service := mocksrv.NewAdminServiceMock()
	client := startAdminServer(t, service)

	t.Run("GetNodeInfo_OK", func(t *testing.T) {
		infoResponse, err := client.GetNodeInfo(context.Background())
		require.NoError(t, err)
		require.Equal(t, types.SystemID(1), infoResponse.SystemID)
		require.Equal(t, "money node", infoResponse.Name)
		require.Equal(t, "1337", infoResponse.Self.Identifier)
		require.Empty(t, infoResponse.Self.Addresses)
	})
}

func startAdminServer(t *testing.T, service *mocksrv.AdminServiceMock) *AdminClient {
	srv := mocksrv.StartAdminApiServer(t, service)

	c, err := ethrpc.DialContext(context.Background(), "http://"+srv)
	require.NoError(t, err)

	return NewAdminClient(c)
}
