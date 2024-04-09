package rpc

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/alphabill-org/alphabill/rpc"
	tokentxs "github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/client/rpc/mocksrv"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/api"
	"github.com/alphabill-org/alphabill-wallet/wallet/tokens"
)

func TestTokensRpcClient(t *testing.T) {
	service := mocksrv.NewStateServiceMock()
	client := startServerAndTokensClient(t, service)

	t.Run("GetToken_OK", func(t *testing.T) {
		tokenID := tokentxs.NewFungibleTokenID(nil, []byte{1})
		tokenTypeID := tokentxs.NewFungibleTokenTypeID(nil, []byte{2})
		tokenType := &tokens.TokenUnitType{
			ID:            tokenTypeID,
			Symbol:        "ABC",
			Name:          "Name of ABC Token Type",
			DecimalPlaces: 2,
			Kind:          tokens.Fungible,
		}
		tokenUnit := &tokens.TokenUnit{
			ID:       tokenID,
			Symbol:   tokenType.Symbol,
			TypeID:   tokenTypeID,
			TypeName: tokenType.Name,
			Amount:   100,
			Decimals: tokenType.DecimalPlaces,
			Kind:     tokens.Fungible,
			Counter:  123,
		}
		*service = *mocksrv.NewStateServiceMock(
			mocksrv.WithUnit(&rpc.Unit[any]{
				UnitID: tokenTypeID,
				Data: tokentxs.FungibleTokenTypeData{
					Symbol:        tokenType.Symbol,
					Name:          tokenType.Name,
					DecimalPlaces: tokenType.DecimalPlaces,
				},
			}),
			mocksrv.WithUnit(&rpc.Unit[any]{
				UnitID: tokenID,
				Data: tokentxs.FungibleTokenData{
					TokenType: tokenTypeID,
					Value:     tokenUnit.Amount,
					T:         168,
					Counter:   tokenUnit.Counter,
				},
			}),
		)

		actualTokenUnit, err := client.GetToken(context.Background(), tokenID)
		require.NoError(t, err)
		require.Equal(t, tokenUnit, actualTokenUnit)
	})
	t.Run("GetToken_NOK", func(t *testing.T) {
		*service = *mocksrv.NewStateServiceMock(mocksrv.WithError(errors.New("some error")))
		tokenID := tokentxs.NewFungibleTokenID(nil, []byte{1})

		tokenUnit, err := client.GetToken(context.Background(), tokenID)
		require.ErrorContains(t, err, "some error")
		require.Nil(t, tokenUnit)
	})
	t.Run("GetToken_NotFound", func(t *testing.T) {
		*service = *mocksrv.NewStateServiceMock()
		tokenID := tokentxs.NewFungibleTokenID(nil, []byte{1})

		tokenUnit, err := client.GetToken(context.Background(), tokenID)
		require.ErrorIs(t, err, api.ErrNotFound)
		require.Nil(t, tokenUnit)
	})

	t.Run("GetTokens_OK", func(t *testing.T) {
		ownerID := []byte{1}

		ftTokenID := tokentxs.NewFungibleTokenID(nil, []byte{1})
		ftTokenTypeID := tokentxs.NewFungibleTokenTypeID(nil, []byte{2})
		ftTokenType := &tokens.TokenUnitType{
			ID:            ftTokenTypeID,
			Symbol:        "ABC",
			Name:          "Fungible ABC Token",
			DecimalPlaces: 2,
			Kind:          tokens.Fungible,
		}
		ftTokenUnit := &tokens.TokenUnit{
			ID:       ftTokenID,
			Symbol:   ftTokenType.Symbol,
			TypeID:   ftTokenTypeID,
			TypeName: ftTokenType.Name,
			Owner:    ownerID,
			Amount:   100,
			Decimals: ftTokenType.DecimalPlaces,
			Kind:     tokens.Fungible,
			Counter:  123,
		}

		nftTokenID := tokentxs.NewNonFungibleTokenID(nil, []byte{3})
		nftTokenTypeID := tokentxs.NewNonFungibleTokenTypeID(nil, []byte{4})
		nftTokenType := &tokens.TokenUnitType{
			ID:     nftTokenTypeID,
			Symbol: "ABC-NFT",
			Name:   "Non-fungible ABC Token",
			Kind:   tokens.NonFungible,
		}
		nftTokenUnit := &tokens.TokenUnit{
			ID:       nftTokenID,
			Symbol:   nftTokenType.Symbol,
			TypeID:   nftTokenTypeID,
			TypeName: nftTokenType.Name,
			Owner:    ownerID,
			NftName:  "NFT name",
			Kind:     tokens.NonFungible,
			Counter:  321,
		}

		// mock two tokens - one nft one ft
		*service = *mocksrv.NewStateServiceMock(
			// fungible token type
			mocksrv.WithUnit(&rpc.Unit[any]{
				UnitID: ftTokenTypeID,
				Data: tokentxs.FungibleTokenTypeData{
					Symbol:        ftTokenType.Symbol,
					Name:          ftTokenType.Name,
					DecimalPlaces: ftTokenType.DecimalPlaces,
				},
				OwnerPredicate: ownerID,
			}),
			// fungible token unit
			mocksrv.WithOwnerUnit(&rpc.Unit[any]{
				UnitID: ftTokenID,
				Data: tokentxs.FungibleTokenData{
					TokenType: ftTokenTypeID,
					Value:     ftTokenUnit.Amount,
					T:         100,
					Counter:   ftTokenUnit.Counter,
				},
				OwnerPredicate: ownerID,
			}),

			// non-fungible token type
			mocksrv.WithUnit(&rpc.Unit[any]{
				UnitID: nftTokenTypeID,
				Data: tokentxs.NonFungibleTokenTypeData{
					Symbol: nftTokenType.Symbol,
					Name:   nftTokenType.Name,
				},
				OwnerPredicate: ownerID,
			}),
			// non-fungible token unit
			mocksrv.WithOwnerUnit(&rpc.Unit[any]{
				UnitID: nftTokenID,
				Data: tokentxs.NonFungibleTokenData{
					NftType: nftTokenTypeID,
					Name:    nftTokenUnit.NftName,
					T:       100,
					Counter: nftTokenUnit.Counter,
				},
				OwnerPredicate: ownerID,
			}),
		)

		// test kind=Any returns both tokens
		tokenz, err := client.GetTokens(context.Background(), tokens.Any, ownerID)
		require.NoError(t, err)
		require.Len(t, tokenz, 2)
		// sort by type - so that fungible token comes first
		slices.SortFunc(tokenz, func(a, b *tokens.TokenUnit) int {
			return a.TypeID.Compare(b.TypeID)
		})
		require.Equal(t, ftTokenUnit, tokenz[0])
		require.Equal(t, nftTokenUnit, tokenz[1])

		// test kind=NonFungible returns only non-fungible token
		tokenz, err = client.GetTokens(context.Background(), tokens.NonFungible, ownerID)
		require.NoError(t, err)
		require.Len(t, tokenz, 1)
		require.Equal(t, nftTokenUnit, tokenz[0])

		// test kind=Fungible returns only fungible token
		tokenz, err = client.GetTokens(context.Background(), tokens.Fungible, ownerID)
		require.NoError(t, err)
		require.Len(t, tokenz, 1)
		require.Equal(t, ftTokenUnit, tokenz[0])

	})
	t.Run("GetTokens_NOK", func(t *testing.T) {
		*service = *mocksrv.NewStateServiceMock(mocksrv.WithError(errors.New("some error")))
		tokenID := tokentxs.NewFungibleTokenID(nil, []byte{1})

		tokenUnit, err := client.GetToken(context.Background(), tokenID)
		require.ErrorContains(t, err, "some error")
		require.Nil(t, tokenUnit)
	})

	t.Run("GetTypeHierarchy_OK", func(t *testing.T) {
		// create 3 levels deep type hierarchy
		var tokenTypes []*tokens.TokenUnitType
		var units []*rpc.Unit[any]
		prevTypeID := tokens.NoParent
		for i := uint8(1); i <= 3; i++ {
			typeID := tokentxs.NewFungibleTokenTypeID(nil, []byte{i})
			tokenType := &tokens.TokenUnitType{
				ID:            typeID,
				ParentTypeID:  prevTypeID,
				Symbol:        "ABC",
				Name:          fmt.Sprintf("ABC %d", i),
				DecimalPlaces: 2,
				Kind:          tokens.Fungible,
			}
			prevTypeID = typeID
			tokenTypes = append(tokenTypes, tokenType)
			units = append(units, &rpc.Unit[any]{
				UnitID: typeID,
				Data: tokentxs.FungibleTokenTypeData{
					Symbol:        tokenType.Symbol,
					Name:          tokenType.Name,
					DecimalPlaces: tokenType.DecimalPlaces,
					ParentTypeId:  tokenType.ParentTypeID,
				},
			})
		}

		*service = *mocksrv.NewStateServiceMock(
			mocksrv.WithUnits(units...),
		)

		// type hierarchy: 3 -> 2 -> 1 (root)
		typeHierarchy, err := client.GetTypeHierarchy(context.Background(), tokenTypes[2].ID)
		require.NoError(t, err)
		require.Len(t, typeHierarchy, 3)
		require.Equal(t, typeHierarchy[0], tokenTypes[2])
		require.Equal(t, typeHierarchy[1], tokenTypes[1])
		require.Equal(t, typeHierarchy[2], tokenTypes[0])

		require.Equal(t, typeHierarchy[0].ParentTypeID, typeHierarchy[1].ID)
		require.Equal(t, typeHierarchy[1].ParentTypeID, typeHierarchy[2].ID)
		require.Equal(t, typeHierarchy[2].ParentTypeID, tokens.NoParent)
	})
	t.Run("GetTypeHierarchy_NOK", func(t *testing.T) {
		*service = *mocksrv.NewStateServiceMock(mocksrv.WithError(errors.New("some error")))
		typeID := tokentxs.NewFungibleTokenTypeID(nil, []byte{1})

		typeHierarchy, err := client.GetTypeHierarchy(context.Background(), typeID)
		require.ErrorContains(t, err, "some error")
		require.Nil(t, typeHierarchy)
	})
}

func startServerAndTokensClient(t *testing.T, service *mocksrv.StateServiceMock) *TokensClient {
	srv := mocksrv.StartStateApiServer(t, service)

	client, err := DialContext(context.Background(), "http://"+srv)
	require.NoError(t, err)
	t.Cleanup(client.Close)

	return NewTokensClient(client)
}
