package rpc

import (
	"context"
	"crypto"
	"errors"
	"testing"

	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/client/rpc/mocksrv"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/api"
)

func TestRpcClient(t *testing.T) {
	service := mocksrv.NewRpcServerMock()
	client := startServerAndClient(t, service)

	t.Run("GetRoundNumber_OK", func(t *testing.T) {
		service.Reset()
		service.RoundNumber = 1337

		roundNumber, err := client.GetRoundNumber(context.Background())
		require.NoError(t, err)
		require.EqualValues(t, 1337, roundNumber)
	})
	t.Run("GetRoundNumber_NOK", func(t *testing.T) {
		service.Reset()
		service.Err = errors.New("some error")

		_, err := client.GetRoundNumber(context.Background())
		require.ErrorContains(t, err, "some error")
	})

	t.Run("GetBill_OK", func(t *testing.T) {
		service.Reset()
		bill := &api.Bill{
			ID: []byte{1},
			BillData: &money.BillData{
				V:        192,
				T:        168,
				Backlink: []byte{1, 2, 3, 4, 5},
			},
		}
		service.Units = map[string]*rpc.Unit[any]{
			string(bill.ID): {
				UnitID: bill.ID,
				Data:   bill.BillData,
			},
		}

		returnedBill, err := client.GetBill(context.Background(), bill.ID, false)
		require.NoError(t, err)
		require.Equal(t, bill, returnedBill)
	})
	t.Run("GetBill_NOK", func(t *testing.T) {
		service.Reset()
		service.Err = errors.New("some error")
		unitID := []byte{1}

		_, err := client.GetBill(context.Background(), unitID, false)
		require.ErrorContains(t, err, "some error")
	})

	t.Run("GetFeeCreditRecord_OK", func(t *testing.T) {
		service.Reset()
		fcb := &api.FeeCreditBill{
			ID: []byte{1},
			FeeCreditRecord: &unit.FeeCreditRecord{
				Balance:  192,
				Timeout:  168,
				Backlink: []byte{1, 2, 3, 4, 5},
			},
		}
		service.Units = map[string]*rpc.Unit[any]{
			string(fcb.ID): {
				UnitID: fcb.ID,
				Data:   fcb.FeeCreditRecord,
			},
		}

		returnedBill, err := client.GetFeeCreditRecord(context.Background(), fcb.ID, false)
		require.NoError(t, err)
		require.Equal(t, fcb, returnedBill)
	})
	t.Run("GetFeeCreditRecord_NOK", func(t *testing.T) {
		service.Reset()
		service.Err = errors.New("some error")
		unitID := []byte{1}

		_, err := client.GetFeeCreditRecord(context.Background(), unitID, false)
		require.ErrorContains(t, err, "some error")
	})

	t.Run("GetUnitsByOwnerID_OK", func(t *testing.T) {
		service.Reset()
		ownerID := []byte{1}
		unitID1 := []byte{2}
		unitID2 := []byte{3}
		service.OwnerUnitIDs = map[string][]types.UnitID{
			string(ownerID): {unitID1, unitID2},
		}

		unitIDs, err := client.GetUnitsByOwnerID(context.Background(), ownerID)
		require.NoError(t, err)
		require.Equal(t, service.OwnerUnitIDs[string(ownerID)], unitIDs)
	})
	t.Run("GetUnitsByOwnerID_NOK", func(t *testing.T) {
		service.Reset()
		service.Err = errors.New("some error")
		ownerID := []byte{1}

		unitData, err := client.GetUnitsByOwnerID(context.Background(), ownerID)
		require.ErrorContains(t, err, "some error")
		require.Nil(t, unitData)
	})

	t.Run("SendTransaction_OK", func(t *testing.T) {
		service.Reset()
		unitID := []byte{1}
		tx := &types.TransactionOrder{Payload: &types.Payload{UnitID: unitID}}

		txHash, err := client.SendTransaction(context.Background(), tx)
		require.NoError(t, err)
		require.Equal(t, tx.Hash(crypto.SHA256), txHash)
	})
	t.Run("SendTransaction_NOK", func(t *testing.T) {
		service.Reset()
		service.Err = errors.New("some error")
		unitID := []byte{1}
		tx := &types.TransactionOrder{Payload: &types.Payload{UnitID: unitID}}

		txHash, err := client.SendTransaction(context.Background(), tx)
		require.ErrorContains(t, err, "some error")
		require.Nil(t, txHash)
	})

	t.Run("GetTransactionProof_OK", func(t *testing.T) {
		service.Reset()
		txHash := []byte{1}
		unitID := []byte{1}
		txRecord := &types.TransactionRecord{TransactionOrder: &types.TransactionOrder{Payload: &types.Payload{UnitID: unitID}}}
		txProof := &types.TxProof{}
		txRecordCbor, err := encodeCbor(txRecord)
		require.NoError(t, err)
		txProofCbor, err := encodeCbor(txProof)
		require.NoError(t, err)
		service.TxProofs = map[string]*rpc.TransactionRecordAndProof{
			string(txHash): {TxRecord: txRecordCbor, TxProof: txProofCbor},
		}

		txRecordRes, txProofRes, err := client.GetTransactionProof(context.Background(), txHash)
		require.NoError(t, err)
		require.Equal(t, txRecord, txRecordRes)
		require.Equal(t, txProof, txProofRes)
	})
	t.Run("GetTransactionProof_NOK", func(t *testing.T) {
		service.Reset()
		service.Err = errors.New("some error")
		txHash := []byte{1}

		txr, txp, err := client.GetTransactionProof(context.Background(), txHash)
		require.ErrorContains(t, err, "some error")
		require.Nil(t, txr)
		require.Nil(t, txp)
	})

	t.Run("GetBlock_OK", func(t *testing.T) {
		service.Reset()
		unitID := []byte{1}
		txRecord := &types.TransactionRecord{TransactionOrder: &types.TransactionOrder{Payload: &types.Payload{UnitID: unitID}}}
		block := &types.Block{Transactions: []*types.TransactionRecord{txRecord}}
		blockCbor, err := encodeCbor(block)
		require.NoError(t, err)
		service.Block = blockCbor

		blockRes, err := client.GetBlock(context.Background(), 1)
		require.NoError(t, err)
		require.Equal(t, block, blockRes)
	})
	t.Run("GetBlock_NOK", func(t *testing.T) {
		service.Reset()
		service.Err = errors.New("some error")

		block, err := client.GetBlock(context.Background(), 1)
		require.ErrorContains(t, err, "some error")
		require.Nil(t, block)
	})
}

func startServerAndClient(t *testing.T, service *mocksrv.MockService) *Client {
	srv := mocksrv.StartServer(t, service)

	client, err := DialContext(context.Background(), "http://"+srv)
	require.NoError(t, err)
	t.Cleanup(client.Close)

	return client
}
