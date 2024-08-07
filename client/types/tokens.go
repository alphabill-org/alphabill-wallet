package types

import (
	"context"
	"crypto"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
)

var NoParent = TokenTypeID(make([]byte, crypto.SHA256.Size()))

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
	}

	FungibleTokenType struct {
		SystemID                 types.SystemID
		ID                       TokenTypeID
		ParentTypeID             TokenTypeID
		Symbol                   string
		Name                     string
		Icon                     *tokens.Icon
		SubTypeCreationPredicate Predicate
		TokenCreationPredicate   Predicate
		InvariantPredicate       Predicate
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
		TokenCreationPredicate   Predicate
		InvariantPredicate       Predicate
		DataUpdatePredicate      Predicate
	}

	FungibleToken struct {
		SystemID       types.SystemID
		ID             TokenID
		Symbol         string
		TypeID         TokenTypeID
		TypeName       string
		OwnerPredicate []byte // TODO: could use sdktypes.Predicate?
		Nonce          []byte // TODO: could be uint64? it is elsewhere
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
		Nonce               []byte // TODO: could be uint64? it is elsewhere
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

func (tt *FungibleTokenType) Create(txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.CreateFungibleTokenTypeAttributes{
		Symbol:                             tt.Symbol,
		Name:                               tt.Name,
		Icon:                               tt.Icon,
		ParentTypeID:                       tt.ParentTypeID,
		DecimalPlaces:                      tt.DecimalPlaces,
		SubTypeCreationPredicate:           tt.SubTypeCreationPredicate,
		TokenCreationPredicate:             tt.TokenCreationPredicate,
		InvariantPredicate:                 tt.InvariantPredicate,
		SubTypeCreationPredicateSignatures: nil,
	}

	txPayload, err := NewPayload(tt.SystemID, tt.ID, tokens.PayloadTypeCreateFungibleTokenType, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	tx := NewTransactionOrder(txPayload)
	err = GenerateAndSetProofs(tx, attr, &attr.SubTypeCreationPredicateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (tt *NonFungibleTokenType) Create(txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.CreateNonFungibleTokenTypeAttributes{
		Symbol:                             tt.Symbol,
		Name:                               tt.Name,
		Icon:                               tt.Icon,
		ParentTypeID:                       tt.ParentTypeID,
		DataUpdatePredicate:                tt.DataUpdatePredicate,
		SubTypeCreationPredicate:           tt.SubTypeCreationPredicate,
		TokenCreationPredicate:             tt.TokenCreationPredicate,
		InvariantPredicate:                 tt.InvariantPredicate,
		SubTypeCreationPredicateSignatures: nil,
	}
	txPayload, err := NewPayload(tt.SystemID, tt.ID, tokens.PayloadTypeCreateNFTType, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	tx := NewTransactionOrder(txPayload)
	err = GenerateAndSetProofs(tx, attr, &attr.SubTypeCreationPredicateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (t *FungibleToken) Create(txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.MintFungibleTokenAttributes{
		Bearer:                           t.OwnerPredicate,
		TypeID:                           t.TypeID,
		Value:                            t.Amount,
		Nonce:                            0,
		TokenCreationPredicateSignatures: nil,
	}
	txPayload, err := NewPayload(t.SystemID, nil, tokens.PayloadTypeMintFungibleToken, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	// generate tokenID
	unitPart, err := tokens.HashForNewTokenID(attr, txPayload.ClientMetadata, crypto.SHA256)
	if err != nil {
		return nil, err
	}
	txPayload.UnitID = tokens.NewFungibleTokenID(t.ID, unitPart)
	t.ID = txPayload.UnitID

	tx := NewTransactionOrder(txPayload)
	err = GenerateAndSetProofs(tx, attr, &attr.TokenCreationPredicateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}

	return tx, nil
}

func (t *FungibleToken) Transfer(ownerPredicate []byte, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.TransferFungibleTokenAttributes{
		NewBearer:                    ownerPredicate,
		Value:                        t.Amount,
		Nonce:                        t.Nonce,
		Counter:                      t.Counter,
		TypeID:                       t.TypeID,
		InvariantPredicateSignatures: nil,
	}
	txPayload, err := NewPayload(t.SystemID, t.ID, tokens.PayloadTypeTransferFungibleToken, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	tx := NewTransactionOrder(txPayload)
	err = GenerateAndSetProofs(tx, attr, &attr.InvariantPredicateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (t *FungibleToken) Split(amount uint64, ownerPredicate []byte, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.SplitFungibleTokenAttributes{
		NewBearer:                    ownerPredicate,
		TargetValue:                  amount,
		Nonce:                        nil,
		Counter:                      t.Counter,
		TypeID:                       t.TypeID,
		RemainingValue:               t.Amount - amount,
		InvariantPredicateSignatures: nil,
	}
	txPayload, err := NewPayload(t.SystemID, t.ID, tokens.PayloadTypeSplitFungibleToken, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	tx := NewTransactionOrder(txPayload)
	err = GenerateAndSetProofs(tx, attr, &attr.InvariantPredicateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (t *FungibleToken) Burn(targetTokenID types.UnitID, targetTokenCounter uint64, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.BurnFungibleTokenAttributes{
		TypeID:                       t.TypeID,
		Value:                        t.Amount,
		TargetTokenID:                targetTokenID,
		TargetTokenCounter:           targetTokenCounter,
		Counter:                      t.Counter,
		InvariantPredicateSignatures: nil,
	}
	txPayload, err := NewPayload(t.SystemID, t.ID, tokens.PayloadTypeBurnFungibleToken, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	tx := NewTransactionOrder(txPayload)
	err = GenerateAndSetProofs(tx, attr, &attr.InvariantPredicateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (t *FungibleToken) Join(burnTxs []*types.TransactionRecord, burnProofs []*types.TxProof, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.JoinFungibleTokenAttributes{
		BurnTransactions:             burnTxs,
		Proofs:                       burnProofs,
		Counter:                      t.Counter,
		InvariantPredicateSignatures: nil,
	}
	txPayload, err := NewPayload(t.SystemID, t.ID, tokens.PayloadTypeJoinFungibleToken, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	tx := NewTransactionOrder(txPayload)
	err = GenerateAndSetProofs(tx, attr, &attr.InvariantPredicateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}
	return tx, nil
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

func (t *NonFungibleToken) Create(txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.MintNonFungibleTokenAttributes{
		Bearer:                           t.OwnerPredicate,
		TypeID:                           t.TypeID,
		Name:                             t.Name,
		URI:                              t.URI,
		Data:                             t.Data,
		DataUpdatePredicate:              t.DataUpdatePredicate,
		Nonce:                            0,
		TokenCreationPredicateSignatures: nil,
	}
	txPayload, err := NewPayload(t.SystemID, nil, tokens.PayloadTypeMintNFT, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	// generate tokenID
	unitPart, err := tokens.HashForNewTokenID(attr, txPayload.ClientMetadata, crypto.SHA256)
	if err != nil {
		return nil, err
	}
	txPayload.UnitID = tokens.NewNonFungibleTokenID(t.ID, unitPart)
	t.ID = txPayload.UnitID

	tx := NewTransactionOrder(txPayload)
	err = GenerateAndSetProofs(tx, attr, &attr.TokenCreationPredicateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (t *NonFungibleToken) Transfer(ownerPredicate []byte, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.TransferNonFungibleTokenAttributes{
		NewBearer:                    ownerPredicate,
		Nonce:                        t.Nonce,
		Counter:                      t.Counter,
		TypeID:                       t.TypeID,
		InvariantPredicateSignatures: nil,
	}
	txPayload, err := NewPayload(t.SystemID, t.ID, tokens.PayloadTypeTransferNFT, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	tx := NewTransactionOrder(txPayload)
	err = GenerateAndSetProofs(tx, attr, &attr.InvariantPredicateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func (t *NonFungibleToken) Update(data []byte, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.UpdateNonFungibleTokenAttributes{
		Data:                 data,
		Counter:              t.Counter,
		DataUpdateSignatures: nil,
	}
	txPayload, err := NewPayload(t.SystemID, t.ID, tokens.PayloadTypeUpdateNFT, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	tx := NewTransactionOrder(txPayload)
	err = GenerateAndSetProofs(tx, attr, &attr.DataUpdateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}
	return tx, nil
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
		LockStatus:                   lockStatus,
		Counter:                      counter,
		InvariantPredicateSignatures: nil,
	}
	txPayload, err := NewPayload(systemID, id, tokens.PayloadTypeLockToken, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	tx := NewTransactionOrder(txPayload)
	err = GenerateAndSetProofs(tx, attr, &attr.InvariantPredicateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func unlockToken(systemID types.SystemID, id types.UnitID, counter uint64, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.UnlockTokenAttributes{
		Counter:                      counter,
		InvariantPredicateSignatures: nil,
	}
	txPayload, err := NewPayload(systemID, id, tokens.PayloadTypeUnlockToken, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	tx := NewTransactionOrder(txPayload)
	err = GenerateAndSetProofs(tx, attr, &attr.InvariantPredicateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}
	return tx, nil
}
