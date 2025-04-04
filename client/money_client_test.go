package client

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	moneyid "github.com/alphabill-org/alphabill-go-base/testutils/money"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-wallet/client/rpc/mocksrv"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
)

func TestMoneyClient(t *testing.T) {
	pdr := moneyid.PDR()
	service := mocksrv.NewStateServiceMock()
	client := startServerAndMoneyClient(t, &pdr, service)

	t.Run("GetBill_OK", func(t *testing.T) {
		service.Reset()
		bill := &sdktypes.Bill{
			NetworkID:   50,
			PartitionID: money.DefaultPartitionID,
			ID:          []byte{1},
			Value:       192,
			Counter:     123,
		}
		service.Units = map[string]*sdktypes.Unit[any]{
			string(bill.ID): {
				NetworkID:   bill.NetworkID,
				PartitionID: bill.PartitionID,
				UnitID:      bill.ID,
				Data: &money.BillData{
					Value:   bill.Value,
					Counter: bill.Counter,
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

func startServerAndMoneyClient(t *testing.T, pdr *types.PartitionDescriptionRecord, service *mocksrv.StateServiceMock) sdktypes.MoneyPartitionClient {
	srv := mocksrv.StartStateApiServer(t, pdr, service)

	moneyClient, err := NewMoneyPartitionClient(context.Background(), "http://"+srv)
	t.Cleanup(moneyClient.Close)
	require.NoError(t, err)

	return moneyClient
}
