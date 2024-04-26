package tokens

import (
	"bytes"
	"context"
	"testing"

	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/internal/testutils"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/logger"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
)

func TestConfirmUnitsTx_skip(t *testing.T) {
	rpcClient := &mockTokensRpcClient{
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			return nil, nil
		},
	}
	batch := txsubmitter.NewBatch(rpcClient, logger.New(t))
	batch.Add(&txsubmitter.TxSubmission{Transaction: &types.TransactionOrder{Payload: &types.Payload{ClientMetadata: &types.ClientMetadata{Timeout: 1}}}})
	err := batch.SendTx(context.Background(), false)
	require.NoError(t, err)

}

func TestConfirmUnitsTx_ok(t *testing.T) {
	getRoundNumberCalled := false
	getTxProofCalled := false
	rpcClient := &mockTokensRpcClient{
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			return nil, nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			getRoundNumberCalled = true
			return 100, nil
		},
		getTransactionProof: func(ctx context.Context, txHash types.Bytes) (*types.TransactionRecord, *types.TxProof, error) {
			getTxProofCalled = true
			return &types.TransactionRecord{}, &types.TxProof{}, nil
		},
	}
	batch := txsubmitter.NewBatch(rpcClient, logger.New(t))
	batch.Add(&txsubmitter.TxSubmission{Transaction: &types.TransactionOrder{Payload: &types.Payload{ClientMetadata: &types.ClientMetadata{Timeout: 101}}}})
	err := batch.SendTx(context.Background(), true)
	require.NoError(t, err)
	require.True(t, getRoundNumberCalled)
	require.True(t, getTxProofCalled)
}

func TestConfirmUnitsTx_timeout(t *testing.T) {
	getRoundNumberCalled := 0
	getTxProofCalled := 0
	randomTxHash1 := test.RandomBytes(32)
	rpcClient := &mockTokensRpcClient{
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
		getTransactionProof: func(ctx context.Context, txHash types.Bytes) (*types.TransactionRecord, *types.TxProof, error) {
			getTxProofCalled++
			if bytes.Equal(txHash, randomTxHash1) {
				return &types.TransactionRecord{}, &types.TxProof{}, nil
			}
			return nil, nil, nil
		},
	}
	batch := txsubmitter.NewBatch(rpcClient, logger.New(t))
	sub1 := &txsubmitter.TxSubmission{Transaction: &types.TransactionOrder{Payload: &types.Payload{ClientMetadata: &types.ClientMetadata{Timeout: 101}}}, TxHash: randomTxHash1}
	batch.Add(sub1)
	sub2 := &txsubmitter.TxSubmission{Transaction: &types.TransactionOrder{Payload: &types.Payload{ClientMetadata: &types.ClientMetadata{Timeout: 102}}}}
	batch.Add(sub2)
	err := batch.SendTx(context.Background(), true)
	require.ErrorContains(t, err, "confirmation timeout")
	require.EqualValues(t, 2, getRoundNumberCalled)
	require.EqualValues(t, 2, getTxProofCalled)
	require.True(t, sub1.Confirmed())
	require.False(t, sub2.Confirmed())
}

func TestCachingRoundNumberFetcher(t *testing.T) {
	getRoundNumberCalled := 0
	rpcClient := &mockTokensRpcClient{
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			getRoundNumberCalled++
			return 100, nil
		},
	}
	fetcher := &cachingRoundNumberFetcher{delegate: rpcClient.GetRoundNumber}
	roundNumber, err := fetcher.getRoundNumber(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 100, roundNumber)
	require.EqualValues(t, 1, getRoundNumberCalled)
	roundNumber, err = fetcher.getRoundNumber(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 100, roundNumber)
	require.EqualValues(t, 1, getRoundNumberCalled)
}
