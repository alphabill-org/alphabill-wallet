package types

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/txsystem/nop"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/stretchr/testify/require"
)

func TestNopTxSigner_NopTxCanBeSigned(t *testing.T) {
	signer, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	txSigner, err := NewNopTxSigner(signer)
	require.NoError(t, err)

	t.Run("sign locking tx", func(t *testing.T) {
		tx := &types.TransactionOrder{Payload: types.Payload{Type: nop.TransactionTypeNOP}}
		require.NoError(t, txSigner.SignTx(tx))
		require.NotEmpty(t, tx.AuthProof)
		require.NotEmpty(t, tx.FeeProof)
	})

	t.Run("sign commit tx", func(t *testing.T) {
		tx := &types.TransactionOrder{Payload: types.Payload{Type: nop.TransactionTypeNOP}}
		require.NoError(t, txSigner.SignCommitTx(tx))
		require.NotEmpty(t, tx.StateUnlock)
		require.EqualValues(t, types.StateUnlockExecute, tx.StateUnlock[0])
		require.NotEmpty(t, tx.AuthProof)
		require.NotEmpty(t, tx.FeeProof)
	})

	t.Run("sign rollback tx", func(t *testing.T) {
		tx := &types.TransactionOrder{Payload: types.Payload{Type: nop.TransactionTypeNOP}}
		require.NoError(t, txSigner.SignRollbackTx(tx))
		require.NotEmpty(t, tx.StateUnlock)
		require.EqualValues(t, types.StateUnlockRollback, tx.StateUnlock[0])
		require.NotEmpty(t, tx.AuthProof)
		require.NotEmpty(t, tx.FeeProof)
	})

}
