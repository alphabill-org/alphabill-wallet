package mocksrv

import (
	"net"
	"net/http"
	"testing"

	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
	clienttypes "github.com/alphabill-org/alphabill-wallet/client/types"
)

func StartServer(t *testing.T, services map[string]interface{}) string {
	server := ethrpc.NewServer()
	t.Cleanup(server.Stop)

	for serviceName, service := range services {
		err := server.RegisterName(serviceName, service)
		require.NoError(t, err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = listener.Close()
	})

	httpServer := &http.Server{
		Addr:    listener.Addr().String(),
		Handler: server,
	}

	go httpServer.Serve(listener)
	t.Cleanup(func() {
		_ = httpServer.Close()
	})
	return httpServer.Addr
}

func StartStateApiServer(t *testing.T, pdr *types.PartitionDescriptionRecord, service *StateServiceMock) string {
	// as a part of client init it queries admin service for getNodeInfo so we need to
	// set up the response. Once AB-1800 gets resolved might not be necessary anymore.
	infoResponse := clienttypes.NodeInfoResponse{
		NetworkID:       pdr.NetworkID,
		PartitionID:     pdr.PartitionID,
		PartitionTypeID: pdr.PartitionTypeID,
		Self:            clienttypes.PeerInfo{Identifier: "1337", Addresses: make([]string, 0)},
	}

	return StartServer(t, map[string]interface{}{"state": service, "admin": NewAdminServiceMock(WithInfoResponse(&infoResponse))})
}

func StartAdminApiServer(t *testing.T, service *AdminServiceMock) string {
	return StartServer(t, map[string]interface{}{"admin": service})
}
