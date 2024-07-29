package client

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill-wallet/client/tx"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
)

var NoParent = sdktypes.TokenTypeID(make([]byte, crypto.SHA256.Size()))

type (
	tokenType struct {
		systemID                 types.SystemID
		id                       sdktypes.TokenTypeID
		parentTypeID             sdktypes.TokenTypeID
		symbol                   string
		name                     string
		icon                     *tokens.Icon
		subTypeCreationPredicate sdktypes.Predicate
		tokenCreationPredicate   sdktypes.Predicate
		invariantPredicate       sdktypes.Predicate
	}

	fungibleTokenType struct {
		tokenType
		decimalPlaces uint32
	}

	nonFungibleTokenType struct {
		tokenType
		dataUpdatePredicate sdktypes.Predicate
	}
)

func NewFungibleTokenType(params *sdktypes.FungibleTokenTypeParams) (sdktypes.FungibleTokenType, error) {
	if params.ID == nil {
		var err error
		params.ID, err = tokens.NewRandomFungibleTokenTypeID(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to generate fungible token type ID: %w", err)
		}
	}

	if len(params.ID) != tokens.UnitIDLength {
		return nil, fmt.Errorf("invalid token type ID: expected hex length is %d characters (%d bytes)",
			tokens.UnitIDLength*2, tokens.UnitIDLength)
	}
	if !params.ID.HasType(tokens.FungibleTokenTypeUnitType) {
		return nil, fmt.Errorf("invalid token type ID: expected unit type is 0x%X", tokens.FungibleTokenTypeUnitType)
	}

	return &fungibleTokenType{
		tokenType: tokenType{
			systemID:                 params.SystemID,
			id:                       params.ID,
			parentTypeID:             params.ParentTypeID,
			symbol:                   params.Symbol,
			name:                     params.Name,
			icon:                     params.Icon,
			subTypeCreationPredicate: params.SubTypeCreationPredicate,
			tokenCreationPredicate:   params.TokenCreationPredicate,
			invariantPredicate:       params.InvariantPredicate,
		},
		decimalPlaces: params.DecimalPlaces,
	}, nil
}

func NewNonFungibleTokenType(params *sdktypes.NonFungibleTokenTypeParams) (sdktypes.NonFungibleTokenType, error) {
	if params.ID == nil {
		var err error
		params.ID, err = tokens.NewRandomNonFungibleTokenTypeID(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to generate non-fungible token type ID: %w", err)
		}
	}

	if len(params.ID) != tokens.UnitIDLength {
		return nil, fmt.Errorf("invalid token type ID: expected hex length is %d characters (%d bytes)",
			tokens.UnitIDLength*2, tokens.UnitIDLength)
	}
	if !params.ID.HasType(tokens.NonFungibleTokenTypeUnitType) {
		return nil, fmt.Errorf("invalid token type ID: expected unit type is 0x%X", tokens.NonFungibleTokenTypeUnitType)
	}

	return &nonFungibleTokenType{
		tokenType: tokenType{
			systemID:                 params.SystemID,
			id:                       params.ID,
			parentTypeID:             params.ParentTypeID,
			symbol:                   params.Symbol,
			name:                     params.Name,
			icon:                     params.Icon,
			subTypeCreationPredicate: params.SubTypeCreationPredicate,
			tokenCreationPredicate:   params.TokenCreationPredicate,
			invariantPredicate:       params.InvariantPredicate,
		},
		dataUpdatePredicate: params.DataUpdatePredicate,
	}, nil
}

func (tt *fungibleTokenType) Create(txOptions ...tx.TxOption) (*types.TransactionOrder, error) {
	opts := tx.TxOptionsWithDefaults(txOptions)
	attr := &tokens.CreateFungibleTokenTypeAttributes{
		Symbol:                             tt.symbol,
		Name:                               tt.name,
		Icon:                               tt.icon,
		ParentTypeID:                       tt.parentTypeID,
		DecimalPlaces:                      tt.decimalPlaces,
		SubTypeCreationPredicate:           tt.subTypeCreationPredicate,
		TokenCreationPredicate:             tt.tokenCreationPredicate,
		InvariantPredicate:                 tt.invariantPredicate,
		SubTypeCreationPredicateSignatures: nil,
	}

	txPayload, err := tx.NewPayload(tt.systemID, tt.id, tokens.PayloadTypeCreateFungibleTokenType, attr, opts)
	if err != nil {
		return nil, err
	}

	txo := tx.NewTransactionOrder(txPayload)
	err = tx.GenerateAndSetProofs(txo, attr, &attr.SubTypeCreationPredicateSignatures, opts)
	if err != nil {
		return nil, err
	}
	return txo, nil
}

func (tt *fungibleTokenType) DecimalPlaces() uint32 {
	return tt.decimalPlaces
}

func (tt *nonFungibleTokenType) Create(txOptions ...tx.TxOption) (*types.TransactionOrder, error) {
	opts := tx.TxOptionsWithDefaults(txOptions)
	attr := &tokens.CreateNonFungibleTokenTypeAttributes{
		Symbol:                             tt.symbol,
		Name:                               tt.name,
		Icon:                               tt.icon,
		ParentTypeID:                       tt.parentTypeID,
		DataUpdatePredicate:                tt.dataUpdatePredicate,
		SubTypeCreationPredicate:           tt.subTypeCreationPredicate,
		TokenCreationPredicate:             tt.tokenCreationPredicate,
		InvariantPredicate:                 tt.invariantPredicate,
		SubTypeCreationPredicateSignatures: nil,
	}
	txPayload, err := tx.NewPayload(tt.systemID, tt.id, tokens.PayloadTypeCreateNFTType, attr, opts)
	if err != nil {
		return nil, err
	}

	txo := tx.NewTransactionOrder(txPayload)
	err = tx.GenerateAndSetProofs(txo, attr, &attr.SubTypeCreationPredicateSignatures, opts)
	if err != nil {
		return nil, err
	}
	return txo, nil
}

func (tt *nonFungibleTokenType) DataUpdatePredicate() sdktypes.Predicate {
	return tt.dataUpdatePredicate
}

func (tt *tokenType) SystemID() types.SystemID {
	return tt.systemID
}

func (tt *tokenType) ID() sdktypes.TokenTypeID {
	return tt.id
}

func (tt *tokenType) ParentTypeID() sdktypes.TokenTypeID {
	return tt.parentTypeID
}

func (tt *tokenType) Symbol() string {
	return tt.symbol
}

func (tt *tokenType) Name() string {
	return tt.name
}

func (tt *tokenType) Icon() *tokens.Icon {
	return tt.icon
}

func (tt *tokenType) SubTypeCreationPredicate() sdktypes.Predicate {
	return tt.subTypeCreationPredicate
}

func (tt *tokenType) TokenCreationPredicate() sdktypes.Predicate {
	return tt.tokenCreationPredicate
}

func (tt *tokenType) InvariantPredicate() sdktypes.Predicate {
	return tt.invariantPredicate
}
