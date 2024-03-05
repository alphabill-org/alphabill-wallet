package mocksrv

import (
	"net"
	"net/http"
	"testing"

	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
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

func StartStateApiServer(t *testing.T, service *StateServiceMock) string {
	return StartServer(t, map[string]interface{}{"state": service})
}

func StartAdminApiServer(t *testing.T, service *AdminServiceMock) string {
	return StartServer(t, map[string]interface{}{"admin": service})
}
