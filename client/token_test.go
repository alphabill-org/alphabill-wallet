package client

import (
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils"
	"github.com/stretchr/testify/require"
)

func TestNewFungibleToken(t *testing.T) {
	t.Parallel()

	ftParams := &FungibleTokenParams{
		SystemID:       8,
		TypeID:         []byte{1},
		OwnerPredicate: sdktypes.Predicate(templates.AlwaysFalseBytes()),
		Amount:         100,
	}
	ft, err := NewFungibleToken(ftParams)
	require.NoError(t, err)
	require.Nil(t, ft.ID())

	tx, err := ft.Create()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.NotNil(t, tx.UnitID())
	require.True(t, tx.UnitID().HasType(tokens.FungibleTokenUnitType))
	require.EqualValues(t, ft.ID(), tx.UnitID())

	attrs := &tokens.MintFungibleTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attrs))
	require.Equal(t, ft.TypeID(), attrs.TypeID)
	require.Equal(t, ft.Amount(), attrs.Value)
	require.Equal(t, ft.OwnerPredicate(), attrs.Bearer)
}

func TestNewNonFungibleToken(t *testing.T) {
	t.Parallel()

	nftParams := NonFungibleTokenParams{
		SystemID:            1,
		TypeID:              []byte{1},
		OwnerPredicate:      sdktypes.Predicate(templates.AlwaysFalseBytes()),
		Name:                "foo",
		URI:                 "http://example.com",
		Data:                []byte{2},
		DataUpdatePredicate: sdktypes.Predicate(templates.AlwaysFalseBytes()),
	}
	// OK
	nft, err := NewNonFungibleToken(&nftParams)
	require.NoError(t, err)
	require.Nil(t, nft.ID())

	// invalid name
	nftParams2 := nftParams
	nftParams2.Name = fmt.Sprintf("%x", testutils.RandomBytes(129))[:257]
	_, err = NewNonFungibleToken(&nftParams2)
	require.ErrorContains(t, err, "name exceeds the maximum allowed size of 256 bytes")

	// invalid URI
	nftParams3 := nftParams
	nftParams3.URI = "invalid_uri"
	_, err = NewNonFungibleToken(&nftParams3)
	require.ErrorContains(t, err, "URI 'invalid_uri' is invalid")

	// invalid URI 2
	nftParams4 := nftParams
	nftParams4.URI = string(testutils.RandomBytes(4097))
	_, err = NewNonFungibleToken(&nftParams4)
	require.ErrorContains(t, err, "URI exceeds the maximum allowed size of 4096 bytes")

	// invalid URI 2
	nftParams5 := nftParams
	nftParams5.Data = testutils.RandomBytes(65537)
	_, err = NewNonFungibleToken(&nftParams5)
	require.ErrorContains(t, err, "data exceeds the maximum allowed size of 65536 bytes")

	tx, err := nft.Create()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.NotNil(t, tx.UnitID())
	require.True(t, tx.UnitID().HasType(tokens.NonFungibleTokenUnitType))
	require.EqualValues(t, nft.ID(), tx.UnitID())

	attrs := &tokens.MintNonFungibleTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attrs))
	require.Equal(t, nft.TypeID(), attrs.TypeID)
	require.Equal(t, nft.URI(), attrs.URI)
	require.Equal(t, nft.Data(), attrs.Data)
	require.Equal(t, nft.Name(), attrs.Name)
	require.EqualValues(t, nft.DataUpdatePredicate(), attrs.DataUpdatePredicate)
	require.Equal(t, nft.OwnerPredicate(), attrs.Bearer)
}
