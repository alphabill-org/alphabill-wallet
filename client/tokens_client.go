package client

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill-wallet/client/rpc"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
)

type (
	tokensPartitionClient struct {
		*rpc.AdminAPIClient
		*rpc.StateAPIClient
	}
)

// NewTokensPartitionClient creates a tokens partition client for the given RPC URL.
func NewTokensPartitionClient(ctx context.Context, rpcUrl string) (*tokensPartitionClient, error) {
	adminApiClient, err := rpc.NewAdminAPIClient(ctx, rpcUrl)
	if err != nil {
		return nil, err
	}
	stateApiClient, err := rpc.NewStateAPIClient(ctx, rpcUrl)
	if err != nil {
		return nil, err
	}
	return &tokensPartitionClient{
		AdminAPIClient: adminApiClient,
		StateAPIClient: stateApiClient,
	}, nil
}

// GetToken returns token for the given token id.
// Returns ErrNotFound if the token does not exist.
func (c *tokensPartitionClient) GetToken(ctx context.Context, id sdktypes.TokenID) (*sdktypes.TokenUnit, error) {
	return c.getTokenUnit(ctx, id)
}

// GetTokens returns tokens for the given owner id.
func (c *tokensPartitionClient) GetTokens(ctx context.Context, kind sdktypes.Kind, ownerID []byte) ([]*sdktypes.TokenUnit, error) {
	unitIds, err := c.GetUnitsByOwnerID(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch owner unit ids: %w", err)
	}
	var tokenz []*sdktypes.TokenUnit
	for _, unitID := range unitIds {
		// only fetch NFTs and FTs, ignoring fee credit units and token type units (type units are not indexed anyway)
		if unitID.HasType(tokens.FungibleTokenUnitType) || unitID.HasType(tokens.NonFungibleTokenUnitType) {
			tokenUnit, err := c.GetToken(ctx, unitID)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch token: %w", err)
			}
			if kind == sdktypes.Any || kind == tokenUnit.Kind {
				tokenz = append(tokenz, tokenUnit)
			}
		}
	}
	return tokenz, nil
}

func (c *tokensPartitionClient) GetTokenTypes(ctx context.Context, kind sdktypes.Kind, creator sdktypes.PubKey) ([]*sdktypes.TokenTypeUnit, error) {
	// TODO AB-1448
	return nil, nil
}

// GetTypeHierarchy returns type hierarchy for given token type id where the root type is the last element (no parent).
func (c *tokensPartitionClient) GetTypeHierarchy(ctx context.Context, typeID sdktypes.TokenTypeID) ([]*sdktypes.TokenTypeUnit, error) {
	var tokenTypes []*sdktypes.TokenTypeUnit
	for len(typeID) > 0 && !typeID.Eq(sdktypes.NoParent) {
		tokenType, err := c.getTokenType(ctx, typeID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch token type: %w", err)
		}
		tokenTypes = append(tokenTypes, tokenType)
		typeID = tokenType.ParentTypeID
	}
	return tokenTypes, nil
}

// GetFeeCreditRecord finds the first fee credit record in tokens partition for the given owner ID,
// returns nil if fee credit record does not exist.
func (c *tokensPartitionClient) GetFeeCreditRecordByOwnerID(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
	return c.StateAPIClient.GetFeeCreditRecordByOwnerID(ctx, ownerID, tokens.FeeCreditRecordUnitType)
}

func (c *tokensPartitionClient) ConfirmTransaction(ctx context.Context, tx *types.TransactionOrder, log *slog.Logger) (*sdktypes.Proof, error) {
	txBatch := txsubmitter.New(tx).ToBatch(c, log)
	err := txBatch.SendTx(ctx, true)
	if err != nil {
		return nil, err
	}
	return txBatch.Submissions()[0].Proof, nil
}

func (c *tokensPartitionClient) Close() {
	c.AdminAPIClient.Close()
	c.StateAPIClient.Close()
}

func (c *tokensPartitionClient) getTokenUnit(ctx context.Context, tokenID sdktypes.TokenID) (*sdktypes.TokenUnit, error) {
	if tokenID.HasType(tokens.FungibleTokenUnitType) {
		var ft *sdktypes.Unit[tokens.FungibleTokenData]
		if err := c.RpcClient.CallContext(ctx, &ft, "state_getUnit", tokenID, false); err != nil {
			return nil, err
		}
		if ft == nil {
			return nil, nil
		}

		var ftType *sdktypes.Unit[tokens.FungibleTokenTypeData]
		if err := c.RpcClient.CallContext(ctx, &ftType, "state_getUnit", ft.Data.TokenType, false); err != nil {
			return nil, err
		}
		if ftType == nil {
			return nil, nil
		}

		return &sdktypes.TokenUnit{
			// common
			ID:       ft.UnitID,
			Symbol:   ftType.Data.Symbol,
			TypeID:   ft.Data.TokenType,
			TypeName: ftType.Data.Name,
			Owner:    ft.OwnerPredicate,
			Counter:  ft.Data.Counter,
			Locked:   ft.Data.Locked,

			// fungible only
			Amount:   ft.Data.Value,
			Decimals: ftType.Data.DecimalPlaces,

			// meta
			Kind: sdktypes.Fungible,
		}, nil
	} else if tokenID.HasType(tokens.NonFungibleTokenUnitType) {
		var nft *sdktypes.Unit[tokens.NonFungibleTokenData]
		if err := c.RpcClient.CallContext(ctx, &nft, "state_getUnit", tokenID, false); err != nil {
			return nil, err
		}
		if nft == nil {
			return nil, nil
		}

		var nftType *sdktypes.Unit[tokens.NonFungibleTokenTypeData]
		if err := c.RpcClient.CallContext(ctx, &nftType, "state_getUnit", nft.Data.TypeID, false); err != nil {
			return nil, err
		}
		if nftType == nil {
			return nil, nil
		}

		return &sdktypes.TokenUnit{
			// common
			ID:       nft.UnitID,
			Symbol:   nftType.Data.Symbol,
			TypeID:   nft.Data.TypeID,
			TypeName: nftType.Data.Name,
			Owner:    nft.OwnerPredicate,
			Counter:  nft.Data.Counter,
			Locked:   nft.Data.Locked,

			// nft only
			NftName:                nft.Data.Name,
			NftURI:                 nft.Data.URI,
			NftData:                nft.Data.Data,
			NftDataUpdatePredicate: nft.Data.DataUpdatePredicate,

			// meta
			Kind: sdktypes.NonFungible,
		}, nil
	} else {
		return nil, fmt.Errorf("invalid token id: %s", tokenID)
	}
}

func (c *tokensPartitionClient) getTokenType(ctx context.Context, typeID sdktypes.TokenTypeID) (*sdktypes.TokenTypeUnit, error) {
	if typeID.HasType(tokens.FungibleTokenTypeUnitType) {
		var ftType *sdktypes.Unit[tokens.FungibleTokenTypeData]
		if err := c.RpcClient.CallContext(ctx, &ftType, "state_getUnit", typeID, false); err != nil {
			return nil, err
		}
		if ftType == nil {
			return nil, nil
		}
		return &sdktypes.TokenTypeUnit{
			ID:                       ftType.UnitID,
			ParentTypeID:             ftType.Data.ParentTypeID,
			Symbol:                   ftType.Data.Symbol,
			Name:                     ftType.Data.Name,
			Icon:                     ftType.Data.Icon,
			SubTypeCreationPredicate: ftType.Data.SubTypeCreationPredicate,
			TokenCreationPredicate:   ftType.Data.TokenCreationPredicate,
			InvariantPredicate:       ftType.Data.InvariantPredicate,
			DecimalPlaces:            ftType.Data.DecimalPlaces,
			Kind:                     sdktypes.Fungible,
		}, nil
	} else if typeID.HasType(tokens.NonFungibleTokenTypeUnitType) {
		var nftType *sdktypes.Unit[tokens.NonFungibleTokenTypeData]
		if err := c.RpcClient.CallContext(ctx, &nftType, "state_getUnit", typeID, false); err != nil {
			return nil, err
		}
		if nftType == nil {
			return nil, nil
		}
		return &sdktypes.TokenTypeUnit{
			ID:                       nftType.UnitID,
			ParentTypeID:             nftType.Data.ParentTypeID,
			Symbol:                   nftType.Data.Symbol,
			Name:                     nftType.Data.Name,
			Icon:                     nftType.Data.Icon,
			SubTypeCreationPredicate: nftType.Data.SubTypeCreationPredicate,
			TokenCreationPredicate:   nftType.Data.TokenCreationPredicate,
			InvariantPredicate:       nftType.Data.InvariantPredicate,
			NftDataUpdatePredicate:   nftType.Data.DataUpdatePredicate,
			Kind:                     sdktypes.NonFungible,
		}, nil
	} else {
		return nil, fmt.Errorf("invalid token type id: %s", typeID)
	}
}
