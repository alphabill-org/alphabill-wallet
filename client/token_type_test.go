package client

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/stretchr/testify/require"
)

func TestNewFungibleTokenType(t *testing.T) {
	t.Parallel()

	ttParams := &sdktypes.FungibleTokenTypeParams{
		Symbol:                   "AB",
		Name:                     "Long name for AB",
		Icon:                     &tokens.Icon{Type: "image/png", Data: []byte{1}},
		DecimalPlaces:            0,
		ParentTypeID:             nil,
		SubTypeCreationPredicate: sdktypes.Predicate(templates.AlwaysFalseBytes()),
		TokenCreationPredicate:   sdktypes.Predicate(templates.AlwaysTrueBytes()),
		InvariantPredicate:       sdktypes.Predicate(templates.AlwaysTrueBytes()),
	}
	tt1, err := NewFungibleTokenType(ttParams)
	require.NoError(t, err)
	require.NotNil(t, tt1.ID())
	require.True(t, tt1.ID().HasType(tokens.FungibleTokenTypeUnitType))

	ttParams.ID = []byte{1}
	_, err = NewFungibleTokenType(ttParams)
	require.ErrorContains(t, err, "invalid token type ID: expected hex length is 66 characters (33 bytes)")

	ttParams.ID = make([]byte, tokens.UnitIDLength)
	_, err = NewFungibleTokenType(ttParams)
	require.ErrorContains(t, err, "invalid token type ID: expected unit type is 0x20")

	tx, err := tt1.Create()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, tt1.ID(), tx.UnitID())

	attrs := &tokens.CreateFungibleTokenTypeAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attrs))
	require.Equal(t, tt1.Symbol(), attrs.Symbol)
	require.Equal(t, tt1.Name(), attrs.Name)
	require.Equal(t, tt1.Icon().Type, attrs.Icon.Type)
	require.Equal(t, tt1.Icon().Data, attrs.Icon.Data)
	require.Equal(t, tt1.DecimalPlaces(), attrs.DecimalPlaces)
}

func TestNewNonFungibleTokenType(t *testing.T) {
	t.Parallel()

	ttParams := &sdktypes.NonFungibleTokenTypeParams{
		Symbol:                   "AB",
		Name:                     "Long name for AB",
		Icon:                     &tokens.Icon{Type: "image/png", Data: []byte{1}},
		ParentTypeID:             nil,
		SubTypeCreationPredicate: sdktypes.Predicate(templates.AlwaysFalseBytes()),
		TokenCreationPredicate:   sdktypes.Predicate(templates.AlwaysTrueBytes()),
		InvariantPredicate:       sdktypes.Predicate(templates.AlwaysTrueBytes()),
		DataUpdatePredicate:      sdktypes.Predicate(templates.AlwaysTrueBytes()),
	}
	tt1, err := NewNonFungibleTokenType(ttParams)
	require.NoError(t, err)
	require.NotNil(t, tt1.ID())
	require.True(t, tt1.ID().HasType(tokens.NonFungibleTokenTypeUnitType))

	ttParams.ID = []byte{1}
	_, err = NewNonFungibleTokenType(ttParams)
	require.ErrorContains(t, err, "invalid token type ID: expected hex length is 66 characters (33 bytes)")

	ttParams.ID = make([]byte, tokens.UnitIDLength)
	_, err = NewNonFungibleTokenType(ttParams)
	require.ErrorContains(t, err, "invalid token type ID: expected unit type is 0x22")

	tx, err := tt1.Create()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, tt1.ID(), tx.UnitID())

	attrs := &tokens.CreateNonFungibleTokenTypeAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attrs))
	require.Equal(t, tt1.Symbol(), attrs.Symbol)
	require.Equal(t, tt1.Name(), attrs.Name)
	require.Equal(t, tt1.Icon().Type, attrs.Icon.Type)
	require.Equal(t, tt1.Icon().Data, attrs.Icon.Data)
}
