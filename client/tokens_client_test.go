package client

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	tokentxs "github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/client/rpc/mocksrv"
	"github.com/alphabill-org/alphabill-wallet/client/types"
)

func TestTokensRpcClient(t *testing.T) {
	service := mocksrv.NewStateServiceMock()
	client := startServerAndTokensClient(t, service)

	t.Run("GetFungibleToken_OK", func(t *testing.T) {
		tokenID := tokentxs.NewFungibleTokenID(nil, []byte{1})
		tokenTypeID := tokentxs.NewFungibleTokenTypeID(nil, []byte{2})
		tokenType := &types.FungibleTokenType{
			PartitionID:   tokens.DefaultPartitionID,
			ID:            tokenTypeID,
			Symbol:        "ABC",
			Name:          "Name of ABC Token Type",
			DecimalPlaces: 2,
		}
		ft := &types.FungibleToken{
			PartitionID:   tokenType.PartitionID,
			ID:            tokenID,
			Symbol:        tokenType.Symbol,
			TypeID:        tokenType.ID,
			TypeName:      tokenType.Name,
			Counter:       123,
			Amount:        100,
			DecimalPlaces: tokenType.DecimalPlaces,
		}
		*service = *mocksrv.NewStateServiceMock(
			mocksrv.WithUnit(&types.Unit[any]{
				PartitionID: tokenType.PartitionID,
				UnitID:      tokenType.ID,
				Data: tokentxs.FungibleTokenTypeData{
					Symbol:        tokenType.Symbol,
					Name:          tokenType.Name,
					DecimalPlaces: tokenType.DecimalPlaces,
				},
			}),
			mocksrv.WithUnit(&types.Unit[any]{
				PartitionID: ft.PartitionID,
				UnitID:      ft.ID,
				Data: tokentxs.FungibleTokenData{
					TokenType: tokenType.ID,
					Value:     ft.Amount,
					Counter:   ft.Counter,
				},
			}),
		)

		actualToken, err := client.GetFungibleToken(context.Background(), tokenID)
		require.NoError(t, err)
		require.Equal(t, ft, actualToken)
	})
	t.Run("GetFungibleToken_NOK", func(t *testing.T) {
		*service = *mocksrv.NewStateServiceMock(mocksrv.WithError(errors.New("some error")))
		tokenID := tokentxs.NewFungibleTokenID(nil, []byte{1})

		ft, err := client.GetFungibleToken(context.Background(), tokenID)
		require.ErrorContains(t, err, "some error")
		require.Nil(t, ft)
	})
	t.Run("GetFungibleToken_NotFound", func(t *testing.T) {
		*service = *mocksrv.NewStateServiceMock()
		tokenID := tokentxs.NewFungibleTokenID(nil, []byte{1})

		ft, err := client.GetFungibleToken(context.Background(), tokenID)
		require.Nil(t, err)
		require.Nil(t, ft)
	})

	t.Run("GetTokens_OK", func(t *testing.T) {
		ownerID := []byte{1}

		ftTokenID := tokentxs.NewFungibleTokenID(nil, []byte{1})
		ftTokenTypeID := tokentxs.NewFungibleTokenTypeID(nil, []byte{2})
		ftTokenType := &types.FungibleTokenType{
			PartitionID:   tokens.DefaultPartitionID,
			ID:            ftTokenTypeID,
			Symbol:        "ABC",
			Name:          "Fungible ABC Token",
			DecimalPlaces: 2,
		}

		ft := &types.FungibleToken{
			PartitionID:    tokens.DefaultPartitionID,
			ID:             ftTokenID,
			Symbol:         ftTokenType.Symbol,
			TypeID:         ftTokenTypeID,
			TypeName:       ftTokenType.Name,
			Counter:        123,
			OwnerPredicate: ownerID,
			Amount:         100,
			DecimalPlaces:  ftTokenType.DecimalPlaces,
		}

		nftTokenID := tokentxs.NewNonFungibleTokenID(nil, []byte{3})
		nftTokenTypeID := tokentxs.NewNonFungibleTokenTypeID(nil, []byte{4})
		nftTokenType := &types.NonFungibleTokenType{
			PartitionID: tokens.DefaultPartitionID,
			ID:          nftTokenTypeID,
			Symbol:      "ABC-NFT",
			Name:        "Non-Fungible ABC Token",
		}
		nft := &types.NonFungibleToken{
			PartitionID:    tokens.DefaultPartitionID,
			ID:             nftTokenID,
			Symbol:         nftTokenType.Symbol,
			TypeID:         nftTokenTypeID,
			TypeName:       nftTokenType.Name,
			Counter:        321,
			OwnerPredicate: ownerID,
			Name:           "NFT name",
		}

		// mock two tokens - one nft one ft
		*service = *mocksrv.NewStateServiceMock(
			// fungible token type
			mocksrv.WithUnit(&types.Unit[any]{
				PartitionID: tokens.DefaultPartitionID,
				UnitID:      ftTokenTypeID,
				Data: tokentxs.FungibleTokenTypeData{
					Symbol:        ftTokenType.Symbol,
					Name:          ftTokenType.Name,
					DecimalPlaces: ftTokenType.DecimalPlaces,
				},
			}),
			// fungible token unit
			mocksrv.WithOwnerUnit(ownerID, &types.Unit[any]{
				PartitionID: tokens.DefaultPartitionID,
				UnitID:      ftTokenID,
				Data: tokentxs.FungibleTokenData{
					TokenType:      ftTokenTypeID,
					Value:          ft.Amount,
					Counter:        ft.Counter,
					OwnerPredicate: ownerID,
				},
			}),

			// non-fungible token type
			mocksrv.WithUnit(&types.Unit[any]{
				PartitionID: tokens.DefaultPartitionID,
				UnitID:      nftTokenTypeID,
				Data: tokentxs.NonFungibleTokenTypeData{
					Symbol: nftTokenType.Symbol,
					Name:   nftTokenType.Name,
				},
			}),
			// non-fungible token unit
			mocksrv.WithOwnerUnit(ownerID, &types.Unit[any]{
				PartitionID: tokens.DefaultPartitionID,
				UnitID:      nftTokenID,
				Data: tokentxs.NonFungibleTokenData{
					TypeID:         nftTokenTypeID,
					Name:           nft.Name,
					Counter:        nft.Counter,
					OwnerPredicate: ownerID,
				},
			}),
		)

		nfts, err := client.GetNonFungibleTokens(context.Background(), ownerID)
		require.NoError(t, err)
		require.Len(t, nfts, 1)
		require.Equal(t, nft, nfts[0])

		fts, err := client.GetFungibleTokens(context.Background(), ownerID)
		require.NoError(t, err)
		require.Len(t, fts, 1)
		require.Equal(t, ft, fts[0])
	})
	t.Run("GetFungibleToken_NOK", func(t *testing.T) {
		*service = *mocksrv.NewStateServiceMock(mocksrv.WithError(errors.New("some error")))
		tokenID := tokentxs.NewFungibleTokenID(nil, []byte{1})

		ft, err := client.GetFungibleToken(context.Background(), tokenID)
		require.ErrorContains(t, err, "some error")
		require.Nil(t, ft)
	})

	t.Run("GetFungibleTokenTypeHierarchy_OK", func(t *testing.T) {
		// create 3 levels deep type hierarchy
		var tokenTypes []*types.FungibleTokenType
		var units []*types.Unit[any]
		prevTypeID := types.NoParent
		for i := uint8(1); i <= 3; i++ {
			typeID := tokentxs.NewFungibleTokenTypeID(nil, []byte{i})
			tokenType := &types.FungibleTokenType{
				PartitionID:   tokens.DefaultPartitionID,
				ID:            typeID,
				ParentTypeID:  prevTypeID,
				Symbol:        "ABC",
				Name:          fmt.Sprintf("ABC %d", i),
				DecimalPlaces: 2,
			}
			prevTypeID = typeID
			tokenTypes = append(tokenTypes, tokenType)
			units = append(units, &types.Unit[any]{
				PartitionID: tokens.DefaultPartitionID,
				UnitID:      typeID,
				Data: tokentxs.FungibleTokenTypeData{
					Symbol:        tokenType.Symbol,
					Name:          tokenType.Name,
					DecimalPlaces: tokenType.DecimalPlaces,
					ParentTypeID:  tokenType.ParentTypeID,
				},
			})
		}

		*service = *mocksrv.NewStateServiceMock(
			mocksrv.WithUnits(units...),
		)

		// type hierarchy: 3 -> 2 -> 1 (root)
		typeHierarchy, err := client.GetFungibleTokenTypeHierarchy(context.Background(), tokenTypes[2].ID)
		require.NoError(t, err)
		require.Len(t, typeHierarchy, 3)
		require.Equal(t, typeHierarchy[0], tokenTypes[2])
		require.Equal(t, typeHierarchy[1], tokenTypes[1])
		require.Equal(t, typeHierarchy[2], tokenTypes[0])

		require.Equal(t, typeHierarchy[0].ParentTypeID, typeHierarchy[1].ID)
		require.Equal(t, typeHierarchy[1].ParentTypeID, typeHierarchy[2].ID)
		require.Equal(t, typeHierarchy[2].ParentTypeID, types.NoParent)
	})
	t.Run("GetFungibleTokenTypeHierarchy_NOK", func(t *testing.T) {
		*service = *mocksrv.NewStateServiceMock()
		typeID := tokentxs.NewFungibleTokenTypeID(nil, []byte{1})

		typeHierarchy, err := client.GetFungibleTokenTypeHierarchy(context.Background(), typeID)
		require.ErrorContains(t, err, fmt.Sprintf("fungible token type %s not found", typeID.String()))
		require.Nil(t, typeHierarchy)
	})

	t.Run("GetNonFungibleTokenTypeHierarchy_OK", func(t *testing.T) {
		// create 3 levels deep type hierarchy
		var tokenTypes []*types.NonFungibleTokenType
		var units []*types.Unit[any]
		prevTypeID := types.NoParent
		for i := uint8(1); i <= 3; i++ {
			typeID := tokentxs.NewNonFungibleTokenTypeID(nil, []byte{i})
			tokenType := &types.NonFungibleTokenType{
				PartitionID:  tokens.DefaultPartitionID,
				ID:           typeID,
				ParentTypeID: prevTypeID,
				Symbol:       "ABC",
				Name:         fmt.Sprintf("ABC %d", i),
			}
			prevTypeID = typeID
			tokenTypes = append(tokenTypes, tokenType)
			units = append(units, &types.Unit[any]{
				PartitionID: tokens.DefaultPartitionID,
				UnitID:      typeID,
				Data: tokentxs.NonFungibleTokenTypeData{
					Symbol:       tokenType.Symbol,
					Name:         tokenType.Name,
					ParentTypeID: tokenType.ParentTypeID,
				},
			})
		}

		*service = *mocksrv.NewStateServiceMock(
			mocksrv.WithUnits(units...),
		)

		// type hierarchy: 3 -> 2 -> 1 (root)
		typeHierarchy, err := client.GetNonFungibleTokenTypeHierarchy(context.Background(), tokenTypes[2].ID)
		require.NoError(t, err)
		require.Len(t, typeHierarchy, 3)
		require.Equal(t, typeHierarchy[0], tokenTypes[2])
		require.Equal(t, typeHierarchy[1], tokenTypes[1])
		require.Equal(t, typeHierarchy[2], tokenTypes[0])

		require.Equal(t, typeHierarchy[0].ParentTypeID, typeHierarchy[1].ID)
		require.Equal(t, typeHierarchy[1].ParentTypeID, typeHierarchy[2].ID)
		require.Equal(t, typeHierarchy[2].ParentTypeID, types.NoParent)
	})
	t.Run("GetNonFungibleTokenTypeHierarchy_NOK", func(t *testing.T) {
		*service = *mocksrv.NewStateServiceMock()
		typeID := tokentxs.NewNonFungibleTokenTypeID(nil, []byte{1})

		typeHierarchy, err := client.GetNonFungibleTokenTypeHierarchy(context.Background(), typeID)
		require.ErrorContains(t, err, fmt.Sprintf("non-fungible token type %s not found", typeID.String()))
		require.Nil(t, typeHierarchy)
	})
}

func startServerAndTokensClient(t *testing.T, service *mocksrv.StateServiceMock) types.TokensPartitionClient {
	srv := mocksrv.StartStateApiServer(t, service)

	tokensClient, err := NewTokensPartitionClient(context.Background(), "http://"+srv)
	t.Cleanup(tokensClient.Close)
	require.NoError(t, err)

	return tokensClient
}
