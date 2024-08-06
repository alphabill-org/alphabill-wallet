package types

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-wallet/client/tx"
	"github.com/stretchr/testify/require"
)

func TestSplitTransactionAmount(t *testing.T) {
	receiverPubKeyHash := hash.Sum256([]byte{1})
	billID := money.NewBillID(nil, nil)
	b := &Bill{
		SystemID: money.DefaultSystemID,
		ID:       billID,
		Value:    500,
		Counter:  1234,
	}
	amount := uint64(150)
	timeout := uint64(100)
	refNo := []byte("120543")

	targetUnits := []*money.TargetUnit{
		{
			OwnerCondition: templates.NewP2pkh256BytesFromKeyHash(receiverPubKeyHash),
			Amount: amount,
		},
	}
	tx, err := b.Split(targetUnits,
		tx.WithTimeout(timeout),
		tx.WithReferenceNumber(refNo))

	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, b.SystemID, tx.SystemID())
	require.EqualValues(t, billID, tx.UnitID())
	require.EqualValues(t, timeout, tx.Timeout())
	require.Equal(t, refNo, tx.Payload.ClientMetadata.ReferenceNumber)
	require.Nil(t, tx.OwnerProof)

	so := &money.SplitAttributes{}
	err = tx.UnmarshalAttributes(so)
	require.NoError(t, err)
	require.Equal(t, amount, so.TargetUnits[0].Amount)
	require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(receiverPubKeyHash), so.TargetUnits[0].OwnerCondition)
	require.EqualValues(t, 350, so.RemainingValue)
	require.EqualValues(t, b.Counter, so.Counter)
}
