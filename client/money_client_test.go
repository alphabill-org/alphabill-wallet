package client

import (
	"context"
	"errors"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/client/rpc/mocksrv"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
)

func TestMoneyClient(t *testing.T) {
	service := mocksrv.NewStateServiceMock()
	client := startServerAndMoneyClient(t, service)

	t.Run("GetBill_OK", func(t *testing.T) {
		service.Reset()
		bill := &bill{
			systemID:   money.DefaultSystemID,
			id:         []byte{1},
			value:      192,
			lastUpdate: 168,
			counter:    123,
			lockStatus: 0,
		}
		service.Units = map[string]*sdktypes.Unit[any]{
			string(bill.id): {
				SystemID: money.DefaultSystemID,
				UnitID:   bill.id,
				Data: &money.BillData{
					V: bill.value,
					T: bill.lastUpdate,
					Counter: bill.counter,
					Locked: bill.lockStatus,
				},
			},
		}

		returnedBill, err := client.GetBill(context.Background(), bill.id)
		require.NoError(t, err)
		require.Equal(t, bill, returnedBill)
	})
	t.Run("GetBill_NOK", func(t *testing.T) {
		service.Reset()
		service.Err = errors.New("some error")
		unitID := []byte{1}

		_, err := client.GetBill(context.Background(), unitID)
		require.ErrorContains(t, err, "some error")
	})
	t.Run("GetBill_NotFound", func(t *testing.T) {
		service.Reset()

		b, err := client.GetBill(context.Background(), []byte{})
		require.Nil(t, err)
		require.Nil(t, b)
	})
}

func startServerAndMoneyClient(t *testing.T, service *mocksrv.StateServiceMock) sdktypes.MoneyPartitionClient {
	srv := mocksrv.StartStateApiServer(t, service)

	moneyClient, err := NewMoneyPartitionClient(context.Background(), "http://" + srv)
	t.Cleanup(moneyClient.Close)
	require.NoError(t, err)

	return moneyClient
}
