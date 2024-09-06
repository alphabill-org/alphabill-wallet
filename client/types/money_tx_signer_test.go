package types

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/stretchr/testify/require"
)

func TestMoneyTxSigner_EachTxTypeCanBeSigned(t *testing.T) {
	signer, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	txSigner, err := NewMoneyTxSigner(signer)
	require.NoError(t, err)

	tests := []struct {
		name string
		txo  *types.TransactionOrder
	}{
		{
			name: "transfer",
			txo:  &types.TransactionOrder{Payload: &types.Payload{Type: money.PayloadTypeTransfer}},
		},
		{
			name: "split",
			txo:  &types.TransactionOrder{Payload: &types.Payload{Type: money.PayloadTypeSplit}},
		},
		{
			name: "dust transfer",
			txo:  &types.TransactionOrder{Payload: &types.Payload{Type: money.PayloadTypeTransDC}},
		},
		{
			name: "swap",
			txo:  &types.TransactionOrder{Payload: &types.Payload{Type: money.PayloadTypeSwapDC}},
		},
		{
			name: "lock",
			txo:  &types.TransactionOrder{Payload: &types.Payload{Type: money.PayloadTypeLock}},
		},
		{
			name: "unlock",
			txo:  &types.TransactionOrder{Payload: &types.Payload{Type: money.PayloadTypeUnlock}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, txSigner.SignTx(tt.txo))
			require.NotEmpty(t, tt.txo.AuthProof)
			require.NotEmpty(t, tt.txo.FeeProof)
		})
	}
}
