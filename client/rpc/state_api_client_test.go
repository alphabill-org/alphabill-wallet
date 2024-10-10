package rpc

import (
	"context"
	"crypto"
	"errors"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/client/rpc/mocksrv"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
)

func TestRpcClient(t *testing.T) {
	service := mocksrv.NewStateServiceMock()
	client := startStateServer(t, service)

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
		tx := &types.TransactionOrder{Payload: types.Payload{UnitID: unitID}}

		txHash, err := client.SendTransaction(context.Background(), tx)
		require.NoError(t, err)
		require.Equal(t, tx.Hash(crypto.SHA256), txHash)
	})
	t.Run("SendTransaction_NOK", func(t *testing.T) {
		service.Reset()
		service.Err = errors.New("some error")
		unitID := []byte{1}
		tx := &types.TransactionOrder{Payload: types.Payload{UnitID: unitID}}

		txHash, err := client.SendTransaction(context.Background(), tx)
		require.ErrorContains(t, err, "some error")
		require.Nil(t, txHash)
	})

	t.Run("GetTransactionProof_OK", func(t *testing.T) {
		service.Reset()
		txHash := []byte{1}
		unitID := []byte{1}
		txRecordProof := &types.TxRecordProof{
			TxRecord: &types.TransactionRecord{TransactionOrder: &types.TransactionOrder{Payload: types.Payload{UnitID: unitID}}},
			TxProof:  &types.TxProof{},
		}
		txRecordProofCBOR, err := encodeCbor(txRecordProof)
		require.NoError(t, err)
		service.TxProofs = map[string]*sdktypes.TransactionRecordAndProof{
			string(txHash): {TxRecordProof: txRecordProofCBOR},
		}

		proof, err := client.GetTransactionProof(context.Background(), txHash)
		require.NoError(t, err)
		require.NotNil(t, proof)
		require.Equal(t, txRecordProof.TxRecord, proof.TxRecord)
		require.Equal(t, txRecordProof.TxProof, proof.TxProof)
	})
	t.Run("GetTransactionProof_NOK", func(t *testing.T) {
		service.Reset()
		service.Err = errors.New("some error")
		txHash := []byte{1}

		proof, err := client.GetTransactionProof(context.Background(), txHash)
		require.ErrorContains(t, err, "some error")
		require.Nil(t, proof)
	})
	t.Run("GetTransactionProof_NotFound", func(t *testing.T) {
		service.Reset()

		proof, err := client.GetTransactionProof(context.Background(), []byte{1})
		require.Nil(t, err)
		require.Nil(t, proof)
	})

	t.Run("GetBlock_OK", func(t *testing.T) {
		service.Reset()
		unitID := []byte{1}
		txRecord := &types.TransactionRecord{TransactionOrder: &types.TransactionOrder{Payload: types.Payload{UnitID: unitID}}}
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
	t.Run("GetBlock_NotFound", func(t *testing.T) {
		service.Reset()

		block, err := client.GetBlock(context.Background(), 1)
		require.Nil(t, err)
		require.Nil(t, block)
	})
}

func startStateServer(t *testing.T, service *mocksrv.StateServiceMock) *StateAPIClient {
	srv := mocksrv.StartServer(t, map[string]interface{}{"state": service})

	client, err := NewStateAPIClient(context.Background(), "http://"+srv)
	require.NoError(t, err)
	t.Cleanup(client.Close)

	return client
}
