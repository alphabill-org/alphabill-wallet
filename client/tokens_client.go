package client

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
)

type tokensPartitionClient struct {
	*partitionClient
}

// NewTokensPartitionClient creates a tokens partition client for the given RPC URL.
func NewTokensPartitionClient(ctx context.Context, rpcUrl string) (sdktypes.TokensPartitionClient, error) {
	partitionClient, err := newPartitionClient(ctx, rpcUrl)
	if err != nil {
		return nil, err
	}

	return &tokensPartitionClient{
		partitionClient: partitionClient,
	}, nil
}

// GetFungibleToken returns fungible token for the given token id.
// Returns nil,nil if the token does not exist.
func (c *tokensPartitionClient) GetFungibleToken(ctx context.Context, tokenID sdktypes.TokenID) (*sdktypes.FungibleToken, error) {
	if !tokenID.HasType(tokens.FungibleTokenUnitType) {
		return nil, fmt.Errorf("invalid fungible token id: %s", tokenID)
	}

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

	return &sdktypes.FungibleToken{
		SystemID:       ft.SystemID,
		ID:             ft.UnitID,
		Symbol:         ftType.Data.Symbol,
		TypeID:         ft.Data.TokenType,
		TypeName:       ftType.Data.Name,
		OwnerPredicate: ft.OwnerPredicate,
		Counter:        ft.Data.Counter,
		LockStatus:     ft.Data.Locked,
		Amount:         ft.Data.Value,
		DecimalPlaces:  ftType.Data.DecimalPlaces,
	}, nil
}

// GetNonFungibleToken returns non-fungible token for the given token id.
// Returns nil,nil if the token does not exist.
func (c *tokensPartitionClient) GetNonFungibleToken(ctx context.Context, tokenID sdktypes.TokenID) (*sdktypes.NonFungibleToken, error) {
	if !tokenID.HasType(tokens.NonFungibleTokenUnitType) {
		return nil, fmt.Errorf("invalid non-fungible token id: %s", tokenID)
	}

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

	return &sdktypes.NonFungibleToken{
		SystemID:            nft.SystemID,
		ID:                  nft.UnitID,
		Symbol:              nftType.Data.Symbol,
		TypeID:              nft.Data.TypeID,
		TypeName:            nftType.Data.Name,
		OwnerPredicate:      nft.OwnerPredicate,
		Counter:             nft.Data.Counter,
		LockStatus:          nft.Data.Locked,
		Name:                nft.Data.Name,
		URI:                 nft.Data.URI,
		Data:                nft.Data.Data,
		DataUpdatePredicate: nft.Data.DataUpdatePredicate,
	}, nil
}

// GetFungibleTokens returns fungible tokens for the given owner id.
func (c *tokensPartitionClient) GetFungibleTokens(ctx context.Context, ownerID []byte) ([]*sdktypes.FungibleToken, error) {
	unitIds, err := c.GetUnitsByOwnerID(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch owner unit ids: %w", err)
	}

	var fungibleTokens []*sdktypes.FungibleToken
	for _, unitID := range unitIds {
		if !unitID.HasType(tokens.FungibleTokenUnitType) {
			continue
		}
		fungibleToken, err := c.GetFungibleToken(ctx, unitID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch token: %w", err)
		}
		fungibleTokens = append(fungibleTokens, fungibleToken)
	}

	return fungibleTokens, nil
}

// GetNonFungibleTokens returns non-fungible tokens for the given owner id.
func (c *tokensPartitionClient) GetNonFungibleTokens(ctx context.Context, ownerID []byte) ([]*sdktypes.NonFungibleToken, error) {
	unitIds, err := c.GetUnitsByOwnerID(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch owner unit ids: %w", err)
	}

	var nonFungibleTokens []*sdktypes.NonFungibleToken
	for _, unitID := range unitIds {
		if !unitID.HasType(tokens.NonFungibleTokenUnitType) {
			continue
		}
		nonFungibleToken, err := c.GetNonFungibleToken(ctx, unitID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch token: %w", err)
		}
		nonFungibleTokens = append(nonFungibleTokens, nonFungibleToken)
	}

	return nonFungibleTokens, nil
}

func (c *tokensPartitionClient) GetFungibleTokenTypes(ctx context.Context, creator sdktypes.PubKey) ([]*sdktypes.FungibleTokenType, error) {
	// TODO AB-1448
	return nil, nil
}

func (c *tokensPartitionClient) GetNonFungibleTokenTypes(ctx context.Context, creator sdktypes.PubKey) ([]*sdktypes.NonFungibleTokenType, error) {
	// TODO AB-1448
	return nil, nil
}

// GetFungibleTokenTypeHierarchy returns type hierarchy for given token type id where the root type is the last element (no parent).
func (c *tokensPartitionClient) GetFungibleTokenTypeHierarchy(ctx context.Context, typeID sdktypes.TokenTypeID) ([]*sdktypes.FungibleTokenType, error) {
	var tokenTypes []*sdktypes.FungibleTokenType
	for len(typeID) > 0 && !typeID.Eq(sdktypes.NoParent) {
		tokenType, err := c.getFungibleTokenType(ctx, typeID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch token type: %w", err)
		}
		tokenTypes = append(tokenTypes, tokenType)
		typeID = tokenType.ParentTypeID
	}
	return tokenTypes, nil
}

// GetFeeCreditRecordByOwnerID finds the first fee credit record in tokens partition for the given owner ID,
// returns nil if fee credit record does not exist.
func (c *tokensPartitionClient) GetFeeCreditRecordByOwnerID(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
	return c.getFeeCreditRecordByOwnerID(ctx, ownerID, tokens.FeeCreditRecordUnitType)
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

func (c *tokensPartitionClient) getFungibleTokenType(ctx context.Context, typeID sdktypes.TokenTypeID) (*sdktypes.FungibleTokenType, error) {
	if !typeID.HasType(tokens.FungibleTokenTypeUnitType) {
		return nil, fmt.Errorf("invalid fungible token type id: %s", typeID)
	}
	var ftType *sdktypes.Unit[tokens.FungibleTokenTypeData]
	if err := c.RpcClient.CallContext(ctx, &ftType, "state_getUnit", typeID, false); err != nil {
		return nil, err
	}
	if ftType == nil {
		return nil, nil
	}
	return &sdktypes.FungibleTokenType{
		SystemID:                 ftType.SystemID,
		ID:                       ftType.UnitID,
		ParentTypeID:             ftType.Data.ParentTypeID,
		Symbol:                   ftType.Data.Symbol,
		Name:                     ftType.Data.Name,
		Icon:                     ftType.Data.Icon,
		SubTypeCreationPredicate: ftType.Data.SubTypeCreationPredicate,
		TokenCreationPredicate:   ftType.Data.TokenCreationPredicate,
		InvariantPredicate:       ftType.Data.InvariantPredicate,
		DecimalPlaces:            ftType.Data.DecimalPlaces,
	}, nil
}

func (c *tokensPartitionClient) getNonFungibleTokenType(ctx context.Context, typeID sdktypes.TokenTypeID) (*sdktypes.NonFungibleTokenType, error) {
	if !typeID.HasType(tokens.NonFungibleTokenTypeUnitType) {
		return nil, fmt.Errorf("invalid non-fungible token type id: %s", typeID)
	}
	var nftType *sdktypes.Unit[tokens.NonFungibleTokenTypeData]
	if err := c.RpcClient.CallContext(ctx, &nftType, "state_getUnit", typeID, false); err != nil {
		return nil, err
	}
	if nftType == nil {
		return nil, nil
	}
	return &sdktypes.NonFungibleTokenType{
		SystemID:                 nftType.SystemID,
		ID:                       nftType.UnitID,
		ParentTypeID:             nftType.Data.ParentTypeID,
		Symbol:                   nftType.Data.Symbol,
		Name:                     nftType.Data.Name,
		Icon:                     nftType.Data.Icon,
		SubTypeCreationPredicate: nftType.Data.SubTypeCreationPredicate,
		TokenCreationPredicate:   nftType.Data.TokenCreationPredicate,
		InvariantPredicate:       nftType.Data.InvariantPredicate,
		DataUpdatePredicate:      nftType.Data.DataUpdatePredicate,
	}, nil
}
