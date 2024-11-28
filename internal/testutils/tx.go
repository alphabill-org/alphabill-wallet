package testutils

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/stretchr/testify/require"
)

// testutils.TxHash(t, tx)
func TxHash(t *testing.T, tx *types.TransactionOrder) []byte {
	hash, err := tx.Hash(crypto.SHA256)
	require.NoError(t, err)
	return hash
}
