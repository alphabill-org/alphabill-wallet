package client

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/ethereum/go-ethereum/rpc"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
)

type tokensPartitionClient struct {
	*partitionClient
}

// NewTokensPartitionClient creates a tokens partition client for the given RPC URL.
func NewTokensPartitionClient(ctx context.Context, rpcUrl string, opts ...Option) (sdktypes.TokensPartitionClient, error) {
	partitionClient, err := newPartitionClient(ctx, rpcUrl, opts...)
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
		return nil, fmt.Errorf("non-fungible token with id %s has invalid token type %s", tokenID, nft.Data.TypeID)
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
	unitIDs, err := c.GetUnitsByOwnerID(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch owner unit ids: %w", err)
	}

	var fts []*sdktypes.FungibleToken
	var batch []rpc.BatchElem
	for _, unitID := range unitIDs {
		if !unitID.HasType(tokens.FungibleTokenUnitType) {
			continue
		}

		var u sdktypes.Unit[tokens.FungibleTokenData]
		batch = append(batch, rpc.BatchElem{
			Method: "state_getUnit",
			Args:   []any{unitID, false},
			Result: &u,
		})
	}

	if len(batch) == 0 {
		return fts, nil
	}
	if err := c.batchCallWithLimit(ctx, batch); err != nil {
		return nil, fmt.Errorf("failed to fetch fungible tokens: %w", err)
	}

	types := make(map[string]*sdktypes.Unit[tokens.FungibleTokenTypeData])
	for _, batchElem := range batch {
		if batchElem.Error != nil {
			return nil, fmt.Errorf("failed to fetch fungible token: %w", batchElem.Error)
		}
		u := batchElem.Result.(*sdktypes.Unit[tokens.FungibleTokenData])
		typeID, _ := u.Data.TokenType.MarshalText()
		types[string(typeID)] = nil
	}

	var typesBatch []rpc.BatchElem
	for typeID := range types {
		var u sdktypes.Unit[tokens.FungibleTokenTypeData]
		typesBatch = append(typesBatch, rpc.BatchElem{
			Method: "state_getUnit",
			Args:   []any{typeID, false},
			Result: &u,
		})
	}
	if len(typesBatch) > 0 {
		if err := c.batchCallWithLimit(ctx, typesBatch); err != nil {
			return nil, fmt.Errorf("failed to fetch fungible token types: %w", err)
		}
		for _, batchElem := range typesBatch {
			if batchElem.Error != nil {
				return nil, fmt.Errorf("failed to fetch fungible token type: %w", batchElem.Error)
			}
			u := batchElem.Result.(*sdktypes.Unit[tokens.FungibleTokenTypeData])
			types[batchElem.Args[0].(string)] = u
		}
	}

	for _, batchElem := range batch {
		u := batchElem.Result.(*sdktypes.Unit[tokens.FungibleTokenData])
		typeID, _ := u.Data.TokenType.MarshalText()
		ftType := types[string(typeID)]

		fts = append(fts, &sdktypes.FungibleToken{
			SystemID:       u.SystemID,
			ID:             u.UnitID,
			Symbol:         ftType.Data.Symbol,
			TypeID:         u.Data.TokenType,
			TypeName:       ftType.Data.Name,
			OwnerPredicate: u.OwnerPredicate,
			Counter:        u.Data.Counter,
			LockStatus:     u.Data.Locked,
			Amount:         u.Data.Value,
			DecimalPlaces:  ftType.Data.DecimalPlaces,
		})
	}

	return fts, nil
}

// GetNonFungibleTokens returns non-fungible tokens for the given owner id.
func (c *tokensPartitionClient) GetNonFungibleTokens(ctx context.Context, ownerID []byte) ([]*sdktypes.NonFungibleToken, error) {
	unitIDs, err := c.GetUnitsByOwnerID(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch owner unit ids: %w", err)
	}

	var nfts []*sdktypes.NonFungibleToken
	var batch []rpc.BatchElem
	for _, unitID := range unitIDs {
		if !unitID.HasType(tokens.NonFungibleTokenUnitType) {
			continue
		}

		var u sdktypes.Unit[tokens.NonFungibleTokenData]
		batch = append(batch, rpc.BatchElem{
			Method: "state_getUnit",
			Args:   []any{unitID, false},
			Result: &u,
		})
	}
	if len(batch) == 0 {
		return nfts, nil
	}
	if err := c.batchCallWithLimit(ctx, batch); err != nil {
		return nil, fmt.Errorf("failed to fetch non-fungible tokens: %w", err)
	}

	types := make(map[string]*sdktypes.Unit[tokens.NonFungibleTokenTypeData])
	for _, batchElem := range batch {
		if batchElem.Error != nil {
			return nil, fmt.Errorf("failed to fetch non-fungible token: %w", batchElem.Error)
		}
		u := batchElem.Result.(*sdktypes.Unit[tokens.NonFungibleTokenData])
		typeID, _ := u.Data.TypeID.MarshalText()
		types[string(typeID)] = nil
	}

	var typesBatch []rpc.BatchElem
	for typeID := range types {
		var u sdktypes.Unit[tokens.NonFungibleTokenTypeData]
		typesBatch = append(typesBatch, rpc.BatchElem{
			Method: "state_getUnit",
			Args:   []any{typeID, false},
			Result: &u,
		})
	}
	if len(typesBatch) > 0 {
		if err := c.batchCallWithLimit(ctx, typesBatch); err != nil {
			return nil, fmt.Errorf("failed to fetch non-fungible token types: %w", err)
		}
		for _, batchElem := range typesBatch {
			if batchElem.Error != nil {
				return nil, fmt.Errorf("failed to fetch non-fungible token type: %w", batchElem.Error)
			}
			u := batchElem.Result.(*sdktypes.Unit[tokens.NonFungibleTokenTypeData])
			types[batchElem.Args[0].(string)] = u
		}
	}

	for _, batchElem := range batch {
		u := batchElem.Result.(*sdktypes.Unit[tokens.NonFungibleTokenData])
		typeID, _ := u.Data.TypeID.MarshalText()
		nftType := types[string(typeID)]

		nfts = append(nfts, &sdktypes.NonFungibleToken{
			SystemID:            u.SystemID,
			ID:                  u.UnitID,
			Symbol:              nftType.Data.Symbol,
			TypeID:              u.Data.TypeID,
			TypeName:            nftType.Data.Name,
			OwnerPredicate:      u.OwnerPredicate,
			Counter:             u.Data.Counter,
			LockStatus:          u.Data.Locked,
			Name:                u.Data.Name,
			URI:                 u.Data.URI,
			Data:                u.Data.Data,
			DataUpdatePredicate: u.Data.DataUpdatePredicate,
		})
	}

	return nfts, nil
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
		if tokenType == nil {
			return nil, fmt.Errorf("fungible token type %s not found", typeID)
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
		TokenMintingPredicate:    ftType.Data.TokenMintingPredicate,
		TokenTypeOwnerPredicate:  ftType.Data.TokenTypeOwnerPredicate,
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
		TokenMintingPredicate:    nftType.Data.TokenMintingPredicate,
		TokenTypeOwnerPredicate:  nftType.Data.TokenTypeOwnerPredicate,
		DataUpdatePredicate:      nftType.Data.DataUpdatePredicate,
	}, nil
}
