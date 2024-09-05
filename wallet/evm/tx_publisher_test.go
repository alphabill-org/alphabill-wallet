package evm

import (
	"context"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/stretchr/testify/require"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/evm/client"
)

type MockClient struct {
	RoundNr      uint64
	RoundNrError error
	PostError    error
	Proof        *sdktypes.Proof
	ProofError   error
}

func createTxOrder() *types.TransactionOrder {
	return &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:   3,
			UnitID:     []byte{0, 0, 0, 1},
			Type:       "test",
			Attributes: []byte{0, 0, 0, 0, 0, 0, 0},
			ClientMetadata: &types.ClientMetadata{
				Timeout: 3,
			},
		},
	}
}

func NewClientMock(round uint64, proof *sdktypes.Proof) Client {
	return &MockClient{
		RoundNr: round,
		Proof:   proof,
	}
}

func (m *MockClient) GetRoundNumber(ctx context.Context) (*client.RoundNumber, error) {
	defer func() { m.RoundNr++ }()
	return &client.RoundNumber{
		RoundNumber:            m.RoundNr,
		LastIndexedRoundNumber: m.RoundNr,
	}, m.RoundNrError
}

func (m *MockClient) PostTransaction(ctx context.Context, tx *types.TransactionOrder) error {
	return m.PostError
}

func (m *MockClient) GetTxProof(ctx context.Context, unitID types.UnitID, txHash sdktypes.TxHash) (*sdktypes.Proof, error) {
	return m.Proof, m.ProofError
}

func TestTxPublisher_SendTx_Cancel(t *testing.T) {
	client := NewClientMock(1, &sdktypes.Proof{})
	txPublisher := NewTxPublisher(client)
	require.NotNil(t, txPublisher)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	txOrder := createTxOrder()
	proof, err := txPublisher.SendTx(ctx, txOrder, nil)
	require.Nil(t, proof)
	require.ErrorContains(t, err, "confirming transaction interrupted: context canceled")
}

func TestTxPublisher_SendTx_RoundTimeout(t *testing.T) {
	client := NewClientMock(1, nil)
	txPublisher := NewTxPublisher(client)
	require.NotNil(t, txPublisher)
	ctx := context.Background()
	txOrder := createTxOrder()
	proof, err := txPublisher.SendTx(ctx, txOrder, nil)
	require.Nil(t, proof)
	require.ErrorContains(t, err, "confirmation timeout evm round 3, tx timeout round 3")
}
