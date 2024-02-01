package rpc

import (
	"context"
	"crypto"
	"errors"
	"net"
	"net/http"
	"testing"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/types"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

func TestRpcClient(t *testing.T) {
	service := &mockService{}
	client := startServerAndClient(t, service)

	t.Run("GetRoundNumber_OK", func(t *testing.T) {
		service.reset()
		service.roundNumber = 1337

		roundNumber, err := client.GetRoundNumber(context.Background())
		require.NoError(t, err)
		require.EqualValues(t, 1337, roundNumber)
	})
	t.Run("GetRoundNumber_NOK", func(t *testing.T) {
		service.reset()
		service.err = errors.New("some error")

		_, err := client.GetRoundNumber(context.Background())
		require.ErrorContains(t, err, "some error")
	})

	t.Run("GetUnit_OK", func(t *testing.T) {
		service.reset()
		unitID := []byte{1}
		unitAndProof := &types.UnitDataAndProof{
			UnitData: &types.StateUnitData{
				Data:   cbor.RawMessage{0x81, 0x00},
				Bearer: predicates.PredicateBytes{0x83, 0x00, 0x01, 0xF6},
			},
			Proof: &types.UnitStateProof{
				UnitID: unitID,
			},
		}
		unitAndProofCbor, err := encodeCbor(unitAndProof)
		require.NoError(t, err)
		service.unitData = &rpc.UnitData{DataAndProofCBOR: unitAndProofCbor}

		unitData, err := client.GetUnit(context.Background(), unitID, true, true)
		require.NoError(t, err)
		require.Equal(t, unitAndProof, unitData)
	})
	t.Run("GetUnit_NOK", func(t *testing.T) {
		service.reset()
		service.err = errors.New("some error")
		unitID := []byte{1}

		unitData, err := client.GetUnit(context.Background(), unitID, true, true)
		require.ErrorContains(t, err, "some error")
		require.Nil(t, unitData)
	})

	t.Run("GetUnitsByOwnerID_OK", func(t *testing.T) {
		service.reset()
		ownerID := []byte{1}
		unitID1 := []byte{2}
		unitID2 := []byte{3}
		service.ownerUnitIds = []types.UnitID{unitID1, unitID2}

		unitIDs, err := client.GetUnitsByOwnerID(context.Background(), ownerID)
		require.NoError(t, err)
		require.Equal(t, service.ownerUnitIds, unitIDs)
	})
	t.Run("GetUnitsByOwnerID_NOK", func(t *testing.T) {
		service.reset()
		service.err = errors.New("some error")
		ownerID := []byte{1}

		unitData, err := client.GetUnitsByOwnerID(context.Background(), ownerID)
		require.ErrorContains(t, err, "some error")
		require.Nil(t, unitData)
	})

	t.Run("SendTransaction_OK", func(t *testing.T) {
		service.reset()
		unitID := []byte{1}
		tx := &types.TransactionOrder{Payload: &types.Payload{UnitID: unitID}}

		txHash, err := client.SendTransaction(context.Background(), tx)
		require.NoError(t, err)
		require.Equal(t, tx.Hash(crypto.SHA256), txHash)
	})
	t.Run("SendTransaction_NOK", func(t *testing.T) {
		service.reset()
		service.err = errors.New("some error")
		unitID := []byte{1}
		tx := &types.TransactionOrder{Payload: &types.Payload{UnitID: unitID}}

		txHash, err := client.SendTransaction(context.Background(), tx)
		require.ErrorContains(t, err, "some error")
		require.Nil(t, txHash)
	})

	t.Run("GetTransactionProof_OK", func(t *testing.T) {
		service.reset()
		txHash := []byte{1}
		unitID := []byte{1}
		txRecord := &types.TransactionRecord{TransactionOrder: &types.TransactionOrder{Payload: &types.Payload{UnitID: unitID}}}
		txProof := &types.TxProof{}
		txRecordCbor, err := encodeCbor(txRecord)
		require.NoError(t, err)
		txProofCbor, err := encodeCbor(txProof)
		require.NoError(t, err)
		service.txProof = &rpc.TransactionRecordAndProof{TxRecordCbor: txRecordCbor, TxProofCbor: txProofCbor}

		txRecordRes, txProofRes, err := client.GetTransactionProof(context.Background(), txHash)
		require.NoError(t, err)
		require.Equal(t, txRecord, txRecordRes)
		require.Equal(t, txProof, txProofRes)
	})
	t.Run("GetTransactionProof_NOK", func(t *testing.T) {
		service.reset()
		service.err = errors.New("some error")
		txHash := []byte{1}

		txr, txp, err := client.GetTransactionProof(context.Background(), txHash)
		require.ErrorContains(t, err, "some error")
		require.Nil(t, txr)
		require.Nil(t, txp)
	})

	t.Run("GetBlock_OK", func(t *testing.T) {
		service.reset()
		unitID := []byte{1}
		txRecord := &types.TransactionRecord{TransactionOrder: &types.TransactionOrder{Payload: &types.Payload{UnitID: unitID}}}
		block := &types.Block{Transactions: []*types.TransactionRecord{txRecord}}
		blockCbor, err := encodeCbor(block)
		require.NoError(t, err)
		service.block = &rpc.Block{BlockCbor: blockCbor}

		blockRes, err := client.GetBlock(context.Background(), 1)
		require.NoError(t, err)
		require.Equal(t, block, blockRes)
	})
	t.Run("GetBlock_NOK", func(t *testing.T) {
		service.reset()
		service.err = errors.New("some error")

		block, err := client.GetBlock(context.Background(), 1)
		require.ErrorContains(t, err, "some error")
		require.Nil(t, block)
	})
}

func startServerAndClient(t *testing.T, service *mockService) *Client {
	server := ethrpc.NewServer()
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
	roundNumber  uint64
	unitData     *rpc.UnitData
	ownerUnitIds []types.UnitID
	txProof      *rpc.TransactionRecordAndProof
	block        *rpc.Block
	err          error
}

func (s *mockService) GetRoundNumber() (uint64, error) {
	return s.roundNumber, s.err
}

func (s *mockService) GetUnit(unitID []byte, returnProof, returnData bool) (*rpc.UnitData, error) {
	return s.unitData, s.err
}

func (s *mockService) GetUnitsByOwnerID(ownerID []byte) ([]types.UnitID, error) {
	return s.ownerUnitIds, s.err
}

func (s *mockService) SendTransaction(tx *rpc.Transaction) ([]byte, error) {
	var txo *types.TransactionOrder
	if err := cbor.Unmarshal(tx.TxOrderCbor, &txo); err != nil {
		return nil, err
	}
	return txo.Hash(crypto.SHA256), s.err
}

func (s *mockService) GetTransactionProof(txHash []byte) (*rpc.TransactionRecordAndProof, error) {
	return s.txProof, s.err
}

func (s *mockService) GetBlock(roundNumber uint64) (*rpc.Block, error) {
	return s.block, s.err
}

func (s *mockService) reset() {
	s.roundNumber = 0
	s.unitData = nil
	s.ownerUnitIds = nil
	s.txProof = nil
	s.err = nil
}
