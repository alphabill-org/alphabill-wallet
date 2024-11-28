package tokens

import (
	"bytes"
	"context"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/internal/testutils"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/logger"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
)

func TestConfirmUnitsTx_skip(t *testing.T) {
	rpcClient := &mockTokensPartitionClient{
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			return nil, nil
		},
	}
	batch := txsubmitter.NewBatch(rpcClient, logger.New(t))
	sub, err := txsubmitter.New(&types.TransactionOrder{Payload: types.Payload{ClientMetadata: &types.ClientMetadata{Timeout: 1}}})
	require.NoError(t, err)
	batch.Add(sub)
	require.NoError(t, batch.SendTx(context.Background(), false))

}

func TestConfirmUnitsTx_ok(t *testing.T) {
	getRoundNumberCalled := false
	getTxProofCalled := false
	rpcClient := &mockTokensPartitionClient{
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			return nil, nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			getRoundNumberCalled = true
			return 100, nil
		},
		getTransactionProof: func(ctx context.Context, txHash hex.Bytes) (*types.TxRecordProof, error) {
			getTxProofCalled = true
			return &types.TxRecordProof{TxRecord: &types.TransactionRecord{ServerMetadata: &types.ServerMetadata{SuccessIndicator: types.TxStatusSuccessful}}}, nil
		},
	}
	batch := txsubmitter.NewBatch(rpcClient, logger.New(t))
	sub, err := txsubmitter.New(&types.TransactionOrder{Payload: types.Payload{ClientMetadata: &types.ClientMetadata{Timeout: 101}}})
	require.NoError(t, err)
	batch.Add(sub)
	require.NoError(t, batch.SendTx(context.Background(), true))
	require.True(t, getRoundNumberCalled)
	require.True(t, getTxProofCalled)
}

func TestConfirmUnitsTx_timeout(t *testing.T) {
	getRoundNumberCalled := 0
	getTxProofCalled := 0
	randomTxHash1 := testutils.RandomBytes(32)
	rpcClient := &mockTokensPartitionClient{
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			return nil, nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			getRoundNumberCalled++
			if getRoundNumberCalled == 1 {
				return 100, nil
			}
			return 103, nil
		},
		getTransactionProof: func(ctx context.Context, txHash hex.Bytes) (*types.TxRecordProof, error) {
			getTxProofCalled++
			if bytes.Equal(txHash, randomTxHash1) {
				return &types.TxRecordProof{}, nil
			}
			return nil, nil
		},
	}
	batch := txsubmitter.NewBatch(rpcClient, logger.New(t))
	sub1, err := txsubmitter.New(&types.TransactionOrder{Payload: types.Payload{ClientMetadata: &types.ClientMetadata{Timeout: 101}}})
	sub1.TxHash = randomTxHash1
	require.NoError(t, err)
	batch.Add(sub1)
	sub2, err := txsubmitter.New(&types.TransactionOrder{Payload: types.Payload{ClientMetadata: &types.ClientMetadata{Timeout: 102}}})
	require.NoError(t, err)
	batch.Add(sub2)
	require.ErrorContains(t, batch.SendTx(context.Background(), true), "confirmation timeout")
	require.EqualValues(t, 2, getRoundNumberCalled)
	require.EqualValues(t, 2, getTxProofCalled)
	require.True(t, sub1.Confirmed())
	require.False(t, sub2.Confirmed())
}
