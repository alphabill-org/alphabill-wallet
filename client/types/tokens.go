package types

import (
	"context"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
)

var NoParent = TokenTypeID(nil)

type (
	TokensPartitionClient interface {
		PartitionClient

		GetFungibleToken(ctx context.Context, id TokenID) (*FungibleToken, error)
		GetFungibleTokens(ctx context.Context, ownerID []byte) ([]*FungibleToken, error)
		GetFungibleTokenTypes(ctx context.Context, creator PubKey) ([]*FungibleTokenType, error)
		GetFungibleTokenTypeHierarchy(ctx context.Context, typeID TokenTypeID) ([]*FungibleTokenType, error)

		GetNonFungibleToken(ctx context.Context, id TokenID) (*NonFungibleToken, error)
		GetNonFungibleTokens(ctx context.Context, ownerID []byte) ([]*NonFungibleToken, error)
		GetNonFungibleTokenTypes(ctx context.Context, creator PubKey) ([]*NonFungibleTokenType, error)
		GetNonFungibleTokenTypeHierarchy(ctx context.Context, typeID TokenTypeID) ([]*NonFungibleTokenType, error)
	}

	FungibleTokenType struct {
		SystemID                 types.SystemID
		ID                       TokenTypeID
		ParentTypeID             TokenTypeID
		Symbol                   string
		Name                     string
		Icon                     *tokens.Icon
		SubTypeCreationPredicate Predicate
		TokenMintingPredicate    Predicate
		TokenTypeOwnerPredicate  Predicate
		DecimalPlaces            uint32
	}

	NonFungibleTokenType struct {
		SystemID                 types.SystemID
		ID                       TokenTypeID
		ParentTypeID             TokenTypeID
		Symbol                   string
		Name                     string
		Icon                     *tokens.Icon
		SubTypeCreationPredicate Predicate
		TokenMintingPredicate    Predicate
		TokenTypeOwnerPredicate  Predicate
		DataUpdatePredicate      Predicate
	}

	FungibleToken struct {
		SystemID       types.SystemID
		ID             TokenID
		Symbol         string
		TypeID         TokenTypeID
		TypeName       string
		OwnerPredicate []byte // TODO: could use sdktypes.Predicate?
		Counter        uint64
		LockStatus     uint64
		Amount         uint64
		DecimalPlaces  uint32
		Burned         bool
	}

	NonFungibleToken struct {
		SystemID            types.SystemID
		ID                  TokenID
		Symbol              string
		TypeID              TokenTypeID
		TypeName            string
		OwnerPredicate      []byte // TODO: could use sdktypes.Predicate?
		Counter             uint64
		LockStatus          uint64
		Name                string
		URI                 string
		Data                []byte
		DataUpdatePredicate Predicate
	}

	TokenID     = types.UnitID
	TokenTypeID = types.UnitID

	TxHash     []byte
	Predicate  []byte
	PubKey     []byte
	PubKeyHash []byte
)

func (tt *FungibleTokenType) Define(txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.DefineFungibleTokenAttributes{
		Symbol:                   tt.Symbol,
		Name:                     tt.Name,
		Icon:                     tt.Icon,
		ParentTypeID:             tt.ParentTypeID,
		DecimalPlaces:            tt.DecimalPlaces,
		SubTypeCreationPredicate: tt.SubTypeCreationPredicate,
		TokenMintingPredicate:    tt.TokenMintingPredicate,
		TokenTypeOwnerPredicate:  tt.TokenTypeOwnerPredicate,
	}

	txPayload, err := NewPayload(tt.SystemID, tt.ID, tokens.PayloadTypeDefineFT, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	return NewTransactionOrder(txPayload), nil
}

func (tt *NonFungibleTokenType) Define(txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.DefineNonFungibleTokenAttributes{
		Symbol:                   tt.Symbol,
		Name:                     tt.Name,
		Icon:                     tt.Icon,
		ParentTypeID:             tt.ParentTypeID,
		DataUpdatePredicate:      tt.DataUpdatePredicate,
		SubTypeCreationPredicate: tt.SubTypeCreationPredicate,
		TokenMintingPredicate:    tt.TokenMintingPredicate,
		TokenTypeOwnerPredicate:  tt.TokenTypeOwnerPredicate,
	}
	txPayload, err := NewPayload(tt.SystemID, tt.ID, tokens.PayloadTypeDefineNFT, attr, txOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to create tx payload: %w", err)
	}

	return NewTransactionOrder(txPayload), nil
}

func (t *FungibleToken) Mint(txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.MintFungibleTokenAttributes{
		OwnerPredicate: t.OwnerPredicate,
		TypeID:         t.TypeID,
		Value:          t.Amount,
		Nonce:          0,
	}
	txPayload, err := NewPayload(t.SystemID, nil, tokens.PayloadTypeMintFT, attr, txOptions...)
	if err != nil {
		return nil, err
	}
	tx := NewTransactionOrder(txPayload)

	// generate tokenID
	unitPart, err := tokens.HashForNewTokenID(tx, crypto.SHA256)
	if err != nil {
		return nil, err
	}
	txPayload.UnitID = tokens.NewFungibleTokenID(t.ID, unitPart)
	t.ID = txPayload.UnitID

	return tx, nil
}

func (t *FungibleToken) Transfer(ownerPredicate []byte, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.TransferFungibleTokenAttributes{
		NewOwnerPredicate: ownerPredicate,
		Value:             t.Amount,
		Counter:           t.Counter,
		TypeID:            t.TypeID,
	}
	txPayload, err := NewPayload(t.SystemID, t.ID, tokens.PayloadTypeTransferFT, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	return NewTransactionOrder(txPayload), nil
}

func (t *FungibleToken) Split(amount uint64, ownerPredicate []byte, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.SplitFungibleTokenAttributes{
		NewOwnerPredicate: ownerPredicate,
		TargetValue:       amount,
		Counter:           t.Counter,
		TypeID:            t.TypeID,
	}
	txPayload, err := NewPayload(t.SystemID, t.ID, tokens.PayloadTypeSplitFT, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	return NewTransactionOrder(txPayload), nil
}

func (t *FungibleToken) Burn(targetTokenID types.UnitID, targetTokenCounter uint64, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.BurnFungibleTokenAttributes{
		TypeID:             t.TypeID,
		Value:              t.Amount,
		TargetTokenID:      targetTokenID,
		TargetTokenCounter: targetTokenCounter,
		Counter:            t.Counter,
	}
	txPayload, err := NewPayload(t.SystemID, t.ID, tokens.PayloadTypeBurnFT, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	return NewTransactionOrder(txPayload), nil
}

func (t *FungibleToken) Join(burnTxs []*types.TransactionRecord, burnProofs []*types.TxProof, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.JoinFungibleTokenAttributes{
		BurnTransactions: burnTxs,
		Proofs:           burnProofs,
		Counter:          t.Counter,
	}
	txPayload, err := NewPayload(t.SystemID, t.ID, tokens.PayloadTypeJoinFT, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	return NewTransactionOrder(txPayload), nil
}

func (t *FungibleToken) Lock(lockStatus uint64, txOptions ...Option) (*types.TransactionOrder, error) {
	return lockToken(t.SystemID, t.ID, t.Counter, lockStatus, txOptions...)
}

func (t *FungibleToken) Unlock(txOptions ...Option) (*types.TransactionOrder, error) {
	return unlockToken(t.SystemID, t.ID, t.Counter, txOptions...)
}

func (t *FungibleToken) GetID() TokenID {
	return t.ID
}

func (t *FungibleToken) GetOwnerPredicate() Predicate {
	return t.OwnerPredicate
}

func (t *FungibleToken) GetLockStatus() uint64 {
	return t.LockStatus
}

func (t *NonFungibleToken) Mint(txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.MintNonFungibleTokenAttributes{
		OwnerPredicate:      t.OwnerPredicate,
		TypeID:              t.TypeID,
		Name:                t.Name,
		URI:                 t.URI,
		Data:                t.Data,
		DataUpdatePredicate: t.DataUpdatePredicate,
		Nonce:               0,
	}
	txPayload, err := NewPayload(t.SystemID, nil, tokens.PayloadTypeMintNFT, attr, txOptions...)
	if err != nil {
		return nil, err
	}
	tx := NewTransactionOrder(txPayload)

	// generate tokenID
	unitPart, err := tokens.HashForNewTokenID(tx, crypto.SHA256)
	if err != nil {
		return nil, err
	}
	txPayload.UnitID = tokens.NewNonFungibleTokenID(t.ID, unitPart)
	t.ID = txPayload.UnitID

	return tx, nil
}

func (t *NonFungibleToken) Transfer(ownerPredicate []byte, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.TransferNonFungibleTokenAttributes{
		NewOwnerPredicate: ownerPredicate,
		Counter:           t.Counter,
		TypeID:            t.TypeID,
	}
	txPayload, err := NewPayload(t.SystemID, t.ID, tokens.PayloadTypeTransferNFT, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	return NewTransactionOrder(txPayload), nil
}

func (t *NonFungibleToken) Update(data []byte, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.UpdateNonFungibleTokenAttributes{
		Data:    data,
		Counter: t.Counter,
	}
	txPayload, err := NewPayload(t.SystemID, t.ID, tokens.PayloadTypeUpdateNFT, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	return NewTransactionOrder(txPayload), nil
}

func (t *NonFungibleToken) Lock(lockStatus uint64, txOptions ...Option) (*types.TransactionOrder, error) {
	return lockToken(t.SystemID, t.ID, t.Counter, lockStatus, txOptions...)
}

func (t *NonFungibleToken) Unlock(txOptions ...Option) (*types.TransactionOrder, error) {
	return unlockToken(t.SystemID, t.ID, t.Counter, txOptions...)
}

func (t *NonFungibleToken) GetID() TokenID {
	return t.ID
}

func (t *NonFungibleToken) GetOwnerPredicate() Predicate {
	return t.OwnerPredicate
}

func (t *NonFungibleToken) GetLockStatus() uint64 {
	return t.LockStatus
}

func (pk PubKey) Hash() PubKeyHash {
	return hash.Sum256(pk)
}

func lockToken(systemID types.SystemID, id types.UnitID, counter uint64, lockStatus uint64, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.LockTokenAttributes{
		LockStatus: lockStatus,
		Counter:    counter,
	}
	txPayload, err := NewPayload(systemID, id, tokens.PayloadTypeLockToken, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	return NewTransactionOrder(txPayload), nil
}

func unlockToken(systemID types.SystemID, id types.UnitID, counter uint64, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.UnlockTokenAttributes{
		Counter: counter,
	}
	txPayload, err := NewPayload(systemID, id, tokens.PayloadTypeUnlockToken, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	return NewTransactionOrder(txPayload), nil
}
