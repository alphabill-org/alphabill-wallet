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
		bill := &sdktypes.Bill{
			NetworkID:  50,
			SystemID:   money.DefaultSystemID,
			ID:         []byte{1},
			Value:      192,
			LastUpdate: 168,
			Counter:    123,
			LockStatus: 0,
		}
		service.Units = map[string]*sdktypes.Unit[any]{
			string(bill.ID): {
				NetworkID: bill.NetworkID,
				SystemID:  bill.SystemID,
				UnitID:    bill.ID,
				Data: &money.BillData{
					V:       bill.Value,
					T:       bill.LastUpdate,
					Counter: bill.Counter,
					Locked:  bill.LockStatus,
				},
			},
		}

		returnedBill, err := client.GetBill(context.Background(), bill.ID)
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

	moneyClient, err := NewMoneyPartitionClient(context.Background(), "http://"+srv)
	t.Cleanup(moneyClient.Close)
	require.NoError(t, err)

	return moneyClient
}
