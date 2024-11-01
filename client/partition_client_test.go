package client

import (
	"context"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/client/rpc/mocksrv"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
)

func TestBatchCallWithLimit(t *testing.T) {
	service := mocksrv.NewStateServiceMock()
	srv := mocksrv.StartStateApiServer(t, service)

	var batch []rpc.BatchElem
	service.Units = make(map[string]*sdktypes.Unit[any])
	for i := byte(0); i < 11; i++ {
		id := money.NewBillID(nil, []byte{i})
		service.Units[string(id)] = createUnit(id)

		var u sdktypes.Unit[string]
		batch = append(batch, rpc.BatchElem{
			Method: "state_getUnit",
			Args:   []any{id, false},
			Result: &u,
		})
	}

	batchCallWithLimit := func(limit int) {
		client, err := newPartitionClient(context.Background(), "http://"+srv, WithBatchItemLimit(limit))
		require.NoError(t, err)
		t.Cleanup(client.Close)
		require.NoError(t, client.batchCallWithLimit(context.Background(), batch))
		require.Equal(t, 11, service.GetUnitCalls)
		service.Reset()
	}

	batchCallWithLimit(-5)
	batchCallWithLimit(0)
	batchCallWithLimit(2)
	batchCallWithLimit(12)
}

func createUnit(id types.UnitID) *sdktypes.Unit[any] {
	return &sdktypes.Unit[any]{
		PartitionID: money.DefaultPartitionID,
		UnitID:      id,
		Data:        id.String(),
	}
}

func startServerAndPartitionClient(t *testing.T, service *mocksrv.StateServiceMock) *partitionClient {
	srv := mocksrv.StartStateApiServer(t, service)

	partitionClient, err := newPartitionClient(context.Background(), "http://"+srv, WithBatchItemLimit(2))
	t.Cleanup(partitionClient.Close)
	require.NoError(t, err)

	return partitionClient
}