package rpc

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

func TestRpcClient(t *testing.T) {
	service := &mockService{roundNumber: 1337}
	client := startServerAndClient(t, service)
	t.Run("GetRoundNumber_OK", func(t *testing.T) {
		roundNumber, err := client.GetRoundNumber(context.Background())
		require.NoError(t, err)
		require.EqualValues(t, 1337, roundNumber)
	})
	t.Run("GetRoundNumber_NOK", func(t *testing.T) {
		service.roundNumberErr = errors.New("some error")
		_, err := client.GetRoundNumber(context.Background())
		require.ErrorContains(t, err, "some error")
	})
}

func startServerAndClient(t *testing.T, service *mockService) *Client {
	server := rpc.NewServer()
	t.Cleanup(server.Stop)

	err := server.RegisterName("state", service)
	require.NoError(t, err)

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

	client, err := DialContext(context.Background(), "http://"+listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(client.Close)

	return client
}

type mockService struct {
	roundNumber    uint64
	roundNumberErr error
}

func (s *mockService) GetRoundNumber() (uint64, error) {
	return s.roundNumber, s.roundNumberErr
}
