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

type TokensPartitionClient struct {
	*partitionClient
}

// NewTokensPartitionClient creates a tokens partition client for the given RPC URL.
func NewTokensPartitionClient(ctx context.Context, rpcUrl string, opts ...Option) (*TokensPartitionClient, error) {
	partitionClient, err := newPartitionClient(ctx, rpcUrl, tokens.PartitionTypeID, opts...)
	if err != nil {
		return nil, err
	}

	return &TokensPartitionClient{
		partitionClient: partitionClient,
	}, nil
}

// GetFungibleToken returns fungible token for the given token id.
// Returns nil,nil if the token does not exist.
func (c *TokensPartitionClient) GetFungibleToken(ctx context.Context, tokenID sdktypes.TokenID) (*sdktypes.FungibleToken, error) {
	if err := tokenID.TypeMustBe(tokens.FungibleTokenUnitType, c.pdr); err != nil {
		return nil, fmt.Errorf("invalid fungible token id: %w", err)
	}

	var ft *sdktypes.Unit[tokens.FungibleTokenData]
	if err := c.RpcClient.CallContext(ctx, &ft, "state_getUnit", tokenID, false); err != nil {
		return nil, err
	}
	if ft == nil {
		return nil, nil
	}

	var ftType *sdktypes.Unit[tokens.FungibleTokenTypeData]
	if err := c.RpcClient.CallContext(ctx, &ftType, "state_getUnit", ft.Data.TypeID, false); err != nil {
		return nil, err
	}
	if ftType == nil {
		return nil, nil
	}

	return &sdktypes.FungibleToken{
		NetworkID:      ft.NetworkID,
		PartitionID:    ft.PartitionID,
		ID:             ft.UnitID,
		Symbol:         ftType.Data.Symbol,
		TypeID:         ft.Data.TypeID,
		TypeName:       ftType.Data.Name,
		OwnerPredicate: ft.Data.OwnerPredicate,
		Counter:        ft.Data.Counter,
		LockStatus:     ft.Data.Locked,
		Amount:         ft.Data.Value,
		DecimalPlaces:  ftType.Data.DecimalPlaces,
	}, nil
}

// GetNonFungibleToken returns non-fungible token for the given token id.
// Returns nil,nil if the token does not exist.
func (c *TokensPartitionClient) GetNonFungibleToken(ctx context.Context, tokenID sdktypes.TokenID) (*sdktypes.NonFungibleToken, error) {
	if err := tokenID.TypeMustBe(tokens.NonFungibleTokenUnitType, c.pdr); err != nil {
		return nil, fmt.Errorf("invalid non-fungible token id: %w", err)
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
		NetworkID:           nft.NetworkID,
		PartitionID:         nft.PartitionID,
		ID:                  nft.UnitID,
		Symbol:              nftType.Data.Symbol,
		TypeID:              nft.Data.TypeID,
		TypeName:            nftType.Data.Name,
		OwnerPredicate:      nft.Data.OwnerPredicate,
		Counter:             nft.Data.Counter,
		LockStatus:          nft.Data.Locked,
		Name:                nft.Data.Name,
		URI:                 nft.Data.URI,
		Data:                nft.Data.Data,
		DataUpdatePredicate: sdktypes.Predicate(nft.Data.DataUpdatePredicate),
	}, nil
}

// GetFungibleTokens returns fungible tokens for the given owner id.
func (c *TokensPartitionClient) GetFungibleTokens(ctx context.Context, ownerID []byte) ([]*sdktypes.FungibleToken, error) {
	unitIDs, err := c.GetUnitsByOwnerID(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch owner unit ids: %w", err)
	}

	var fts []*sdktypes.FungibleToken
	var batch []rpc.BatchElem
	for _, unitID := range unitIDs {
		if unitID.TypeMustBe(tokens.FungibleTokenUnitType, c.pdr) != nil {
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
		typeID, _ := u.Data.TypeID.MarshalText()
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
		typeID, _ := u.Data.TypeID.MarshalText()
		ftType := types[string(typeID)]

		fts = append(fts, &sdktypes.FungibleToken{
			NetworkID:      u.NetworkID,
			PartitionID:    u.PartitionID,
			ID:             u.UnitID,
			Symbol:         ftType.Data.Symbol,
			TypeID:         u.Data.TypeID,
			TypeName:       ftType.Data.Name,
			OwnerPredicate: u.Data.OwnerPredicate,
			Counter:        u.Data.Counter,
			LockStatus:     u.Data.Locked,
			Amount:         u.Data.Value,
			DecimalPlaces:  ftType.Data.DecimalPlaces,
		})
	}

	return fts, nil
}

// GetNonFungibleTokens returns non-fungible tokens for the given owner id.
func (c *TokensPartitionClient) GetNonFungibleTokens(ctx context.Context, ownerID []byte) ([]*sdktypes.NonFungibleToken, error) {
	unitIDs, err := c.GetUnitsByOwnerID(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch owner unit ids: %w", err)
	}

	var nfts []*sdktypes.NonFungibleToken
	var batch []rpc.BatchElem
	for _, unitID := range unitIDs {
		if unitID.TypeMustBe(tokens.NonFungibleTokenUnitType, c.pdr) != nil {
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
			NetworkID:           u.NetworkID,
			PartitionID:         u.PartitionID,
			ID:                  u.UnitID,
			Symbol:              nftType.Data.Symbol,
			TypeID:              u.Data.TypeID,
			TypeName:            nftType.Data.Name,
			OwnerPredicate:      u.Data.OwnerPredicate,
			Counter:             u.Data.Counter,
			LockStatus:          u.Data.Locked,
			Name:                u.Data.Name,
			URI:                 u.Data.URI,
			Data:                u.Data.Data,
			DataUpdatePredicate: sdktypes.Predicate(u.Data.DataUpdatePredicate),
		})
	}

	return nfts, nil
}

func (c *TokensPartitionClient) GetFungibleTokenTypes(ctx context.Context, creator sdktypes.PubKey) ([]*sdktypes.FungibleTokenType, error) {
	// TODO AB-1448
	return nil, nil
}

func (c *TokensPartitionClient) GetNonFungibleTokenTypes(ctx context.Context, creator sdktypes.PubKey) ([]*sdktypes.NonFungibleTokenType, error) {
	// TODO AB-1448
	return nil, nil
}

// GetFungibleTokenTypeHierarchy returns type hierarchy for given token type id where the root type is the last element (no parent).
func (c *TokensPartitionClient) GetFungibleTokenTypeHierarchy(ctx context.Context, typeID sdktypes.TokenTypeID) ([]*sdktypes.FungibleTokenType, error) {
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

// GetNonFungibleTokenTypeHierarchy returns type hierarchy for given token type id where the root type is the last element (no parent).
func (c *TokensPartitionClient) GetNonFungibleTokenTypeHierarchy(ctx context.Context, typeID sdktypes.TokenTypeID) ([]*sdktypes.NonFungibleTokenType, error) {
	var tokenTypes []*sdktypes.NonFungibleTokenType
	for len(typeID) > 0 && !typeID.Eq(sdktypes.NoParent) {
		tokenType, err := c.getNonFungibleTokenType(ctx, typeID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch token type: %w", err)
		}
		if tokenType == nil {
			return nil, fmt.Errorf("non-fungible token type %s not found", typeID)
		}
		tokenTypes = append(tokenTypes, tokenType)
		typeID = tokenType.ParentTypeID
	}
	return tokenTypes, nil
}

// GetFeeCreditRecordByOwnerID finds the first fee credit record in tokens partition for the given owner ID,
// returns nil if fee credit record does not exist.
func (c *TokensPartitionClient) GetFeeCreditRecordByOwnerID(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
	return c.getFeeCreditRecordByOwnerID(ctx, ownerID, tokens.FeeCreditRecordUnitType)
}

func (c *TokensPartitionClient) ConfirmTransaction(ctx context.Context, tx *types.TransactionOrder, log *slog.Logger) (*types.TxRecordProof, error) {
	sub, err := txsubmitter.New(tx)
	if err != nil {
		return nil, fmt.Errorf("failed to create tx submission: %w", err)
	}
	txBatch := sub.ToBatch(c, log)

	if err := txBatch.SendTx(ctx, true); err != nil {
		return nil, err
	}
	if !sub.Success() {
		return nil, fmt.Errorf("transaction failed with status %d", sub.Status())
	}
	return sub.Proof, nil
}

func (c *TokensPartitionClient) Close() {
	c.AdminAPIClient.Close()
	c.StateAPIClient.Close()
}

func (c *TokensPartitionClient) getFungibleTokenType(ctx context.Context, typeID sdktypes.TokenTypeID) (*sdktypes.FungibleTokenType, error) {
	if err := typeID.TypeMustBe(tokens.FungibleTokenTypeUnitType, c.pdr); err != nil {
		return nil, fmt.Errorf("invalid fungible token type id: %w", err)
	}
	var ftType *sdktypes.Unit[tokens.FungibleTokenTypeData]
	if err := c.RpcClient.CallContext(ctx, &ftType, "state_getUnit", typeID, false); err != nil {
		return nil, err
	}
	if ftType == nil {
		return nil, nil
	}
	return &sdktypes.FungibleTokenType{
		NetworkID:                ftType.NetworkID,
		PartitionID:              ftType.PartitionID,
		ID:                       ftType.UnitID,
		ParentTypeID:             ftType.Data.ParentTypeID,
		Symbol:                   ftType.Data.Symbol,
		Name:                     ftType.Data.Name,
		Icon:                     ftType.Data.Icon,
		SubTypeCreationPredicate: sdktypes.Predicate(ftType.Data.SubTypeCreationPredicate),
		TokenMintingPredicate:    sdktypes.Predicate(ftType.Data.TokenMintingPredicate),
		TokenTypeOwnerPredicate:  sdktypes.Predicate(ftType.Data.TokenTypeOwnerPredicate),
		DecimalPlaces:            ftType.Data.DecimalPlaces,
	}, nil
}

func (c *TokensPartitionClient) getNonFungibleTokenType(ctx context.Context, typeID sdktypes.TokenTypeID) (*sdktypes.NonFungibleTokenType, error) {
	if err := typeID.TypeMustBe(tokens.NonFungibleTokenTypeUnitType, c.pdr); err != nil {
		return nil, fmt.Errorf("invalid non-fungible token type id: %w", err)
	}
	var nftType *sdktypes.Unit[tokens.NonFungibleTokenTypeData]
	if err := c.RpcClient.CallContext(ctx, &nftType, "state_getUnit", typeID, false); err != nil {
		return nil, err
	}
	if nftType == nil {
		return nil, nil
	}
	return &sdktypes.NonFungibleTokenType{
		PartitionID:              nftType.PartitionID,
		ID:                       nftType.UnitID,
		ParentTypeID:             nftType.Data.ParentTypeID,
		Symbol:                   nftType.Data.Symbol,
		Name:                     nftType.Data.Name,
		Icon:                     nftType.Data.Icon,
		SubTypeCreationPredicate: sdktypes.Predicate(nftType.Data.SubTypeCreationPredicate),
		TokenMintingPredicate:    sdktypes.Predicate(nftType.Data.TokenMintingPredicate),
		TokenTypeOwnerPredicate:  sdktypes.Predicate(nftType.Data.TokenTypeOwnerPredicate),
		DataUpdatePredicate:      sdktypes.Predicate(nftType.Data.DataUpdatePredicate),
	}, nil
}
