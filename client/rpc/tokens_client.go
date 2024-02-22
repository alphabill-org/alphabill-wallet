package rpc

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/tokens"
	"github.com/alphabill-org/alphabill/rpc"
	tokentxs "github.com/alphabill-org/alphabill/txsystem/tokens"
)

// TokensClient defines typed wrappers for the Alphabill RPC API.
type TokensClient struct {
	*Client
}

// NewTokensClient creates a client that uses the given RPC client.
func NewTokensClient(c *Client) *TokensClient {
	return &TokensClient{Client: c}
}

func (c *TokensClient) GetToken(ctx context.Context, id tokens.TokenID) (*tokens.TokenUnit, error) {
	return c.getTokenUnit(ctx, id)
}

func (c *TokensClient) GetTokens(ctx context.Context, kind tokens.Kind, ownerID []byte) ([]*tokens.TokenUnit, error) {
	unitIds, err := c.GetUnitsByOwnerID(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch owner unit ids: %w", err)
	}
	var tokenz []*tokens.TokenUnit
	for _, unitID := range unitIds {
		// only fetch NFTs and FTs, ignoring fee credit units and token type units (type units are not indexed anyway)
		if unitID.HasType(tokentxs.FungibleTokenUnitType) || unitID.HasType(tokentxs.NonFungibleTokenUnitType) {
			tokenUnit, err := c.GetToken(ctx, unitID)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch token: %w", err)
			}
			if kind == tokens.Any || kind == tokenUnit.Kind {
				tokenz = append(tokenz, tokenUnit)
			}
		}
	}
	return tokenz, nil
}

func (c *TokensClient) GetTokenTypes(ctx context.Context, kind tokens.Kind, creator wallet.PubKey) ([]*tokens.TokenUnitType, error) {
	// TODO AB-1448
	return nil, nil
}

func (c *TokensClient) GetTypeHierarchy(ctx context.Context, typeID tokens.TokenTypeID) ([]*tokens.TokenUnitType, error) {
	var tokenTypes []*tokens.TokenUnitType
	for len(typeID) > 0 && !typeID.Eq(tokens.NoParent) {
		tokenType, err := c.getTokenType(ctx, typeID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch token type: %w", err)
		}
		tokenTypes = append(tokenTypes, tokenType)
		typeID = tokenType.ParentTypeID
	}
	return tokenTypes, nil
}

func (c *TokensClient) getTokenUnit(ctx context.Context, tokenID tokens.TokenID) (*tokens.TokenUnit, error) {
	if tokenID.HasType(tokentxs.FungibleTokenUnitType) {
		var ft rpc.Unit[tokentxs.FungibleTokenData]
		if err := c.c.CallContext(ctx, &ft, "state_getUnit", tokenID, false); err != nil {
			return nil, err
		}
		var ftType rpc.Unit[tokentxs.FungibleTokenTypeData]
		if err := c.c.CallContext(ctx, &ftType, "state_getUnit", ft.Data.TokenType, false); err != nil {
			return nil, err
		}
		return &tokens.TokenUnit{
			// common
			ID:       ft.UnitID,
			Symbol:   ftType.Data.Symbol,
			TypeID:   ft.Data.TokenType,
			TypeName: ftType.Data.Name,
			Owner:    ft.OwnerPredicate,
			Locked:   ft.Data.Locked,

			// fungible only
			Amount:   ft.Data.Value,
			Decimals: ftType.Data.DecimalPlaces,

			// meta
			Kind:   tokens.Fungible,
			TxHash: ft.Data.Backlink,
		}, nil
	} else if tokenID.HasType(tokentxs.NonFungibleTokenUnitType) {
		var nft rpc.Unit[tokentxs.NonFungibleTokenData]
		if err := c.c.CallContext(ctx, &nft, "state_getUnit", tokenID, false); err != nil {
			return nil, err
		}
		var nftType rpc.Unit[tokentxs.NonFungibleTokenTypeData]
		if err := c.c.CallContext(ctx, &nftType, "state_getUnit", nft.Data.NftType, false); err != nil {
			return nil, err
		}
		return &tokens.TokenUnit{
			// common
			ID:       nft.UnitID,
			Symbol:   nftType.Data.Symbol,
			TypeID:   nft.Data.NftType,
			TypeName: nftType.Data.Name,
			Owner:    nft.OwnerPredicate,
			Locked:   nft.Data.Locked,

			// nft only
			NftName:                nft.Data.Name,
			NftURI:                 nft.Data.URI,
			NftData:                nft.Data.Data,
			NftDataUpdatePredicate: nft.Data.DataUpdatePredicate,

			// meta
			Kind:   tokens.NonFungible,
			TxHash: nft.Data.Backlink,
		}, nil
	} else {
		return nil, fmt.Errorf("invalid token id: %s", tokenID)
	}
}

func (c *TokensClient) getTokenType(ctx context.Context, typeID tokens.TokenTypeID) (*tokens.TokenUnitType, error) {
	if typeID.HasType(tokentxs.FungibleTokenTypeUnitType) {
		var ftType rpc.Unit[tokentxs.FungibleTokenTypeData]
		if err := c.c.CallContext(ctx, &ftType, "state_getUnit", typeID, false); err != nil {
			return nil, err
		}
		return &tokens.TokenUnitType{
			ID:                       ftType.UnitID,
			ParentTypeID:             ftType.Data.ParentTypeId,
			Symbol:                   ftType.Data.Symbol,
			Name:                     ftType.Data.Name,
			Icon:                     ftType.Data.Icon,
			SubTypeCreationPredicate: ftType.Data.SubTypeCreationPredicate,
			TokenCreationPredicate:   ftType.Data.TokenCreationPredicate,
			InvariantPredicate:       ftType.Data.InvariantPredicate,
			DecimalPlaces:            ftType.Data.DecimalPlaces,
			Kind:                     tokens.Fungible,
		}, nil
	} else if typeID.HasType(tokentxs.NonFungibleTokenTypeUnitType) {
		var nftType rpc.Unit[tokentxs.NonFungibleTokenTypeData]
		if err := c.c.CallContext(ctx, &nftType, "state_getUnit", typeID, false); err != nil {
			return nil, err
		}
		return &tokens.TokenUnitType{
			ID:                       nftType.UnitID,
			ParentTypeID:             nftType.Data.ParentTypeId,
			Symbol:                   nftType.Data.Symbol,
			Name:                     nftType.Data.Name,
			Icon:                     nftType.Data.Icon,
			SubTypeCreationPredicate: nftType.Data.SubTypeCreationPredicate,
			TokenCreationPredicate:   nftType.Data.TokenCreationPredicate,
			InvariantPredicate:       nftType.Data.InvariantPredicate,
			NftDataUpdatePredicate:   nftType.Data.DataUpdatePredicate,
			Kind:                     tokens.NonFungible,
		}, nil
	} else {
		return nil, fmt.Errorf("invalid token type id: %s", typeID)
	}
}
