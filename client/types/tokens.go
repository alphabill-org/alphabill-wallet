package types

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/nop"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
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
		NetworkID                types.NetworkID
		PartitionID              types.PartitionID
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
		NetworkID                types.NetworkID
		PartitionID              types.PartitionID
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
		NetworkID      types.NetworkID
		PartitionID    types.PartitionID
		ID             TokenID
		Symbol         string
		TypeID         TokenTypeID
		TypeName       string
		OwnerPredicate []byte // TODO: could use sdktypes.Predicate?
		Counter        uint64
		StateLockTx    hex.Bytes
		Amount         uint64
		DecimalPlaces  uint32
		Burned         bool
	}

	NonFungibleToken struct {
		NetworkID           types.NetworkID
		PartitionID         types.PartitionID
		ID                  TokenID
		Symbol              string
		TypeID              TokenTypeID
		TypeName            string
		OwnerPredicate      []byte // TODO: could use sdktypes.Predicate?
		Counter             uint64
		StateLockTx         hex.Bytes
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

	return NewTransactionOrder(tt.NetworkID, tt.PartitionID, tt.ID, tokens.TransactionTypeDefineFT, attr, txOptions...)
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
	return NewTransactionOrder(tt.NetworkID, tt.PartitionID, tt.ID, tokens.TransactionTypeDefineNFT, attr, txOptions...)
}

func (t *FungibleToken) Mint(pdr *types.PartitionDescriptionRecord, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.MintFungibleTokenAttributes{
		OwnerPredicate: t.OwnerPredicate,
		TypeID:         t.TypeID,
		Value:          t.Amount,
		Nonce:          0,
	}
	tx, err := NewTransactionOrder(pdr.NetworkID, pdr.PartitionID, nil, tokens.TransactionTypeMintFT, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	// generate tokenID
	if err = tokens.GenerateUnitID(tx, pdr); err != nil {
		return nil, fmt.Errorf("generating token ID: %w", err)
	}
	t.ID = tx.UnitID

	return tx, nil
}

func (t *FungibleToken) Transfer(ownerPredicate []byte, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.TransferFungibleTokenAttributes{
		NewOwnerPredicate: ownerPredicate,
		Value:             t.Amount,
		Counter:           t.Counter,
		TypeID:            t.TypeID,
	}
	return NewTransactionOrder(t.NetworkID, t.PartitionID, t.ID, tokens.TransactionTypeTransferFT, attr, txOptions...)
}

func (t *FungibleToken) Split(amount uint64, ownerPredicate []byte, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.SplitFungibleTokenAttributes{
		NewOwnerPredicate: ownerPredicate,
		TargetValue:       amount,
		Counter:           t.Counter,
		TypeID:            t.TypeID,
	}
	return NewTransactionOrder(t.NetworkID, t.PartitionID, t.ID, tokens.TransactionTypeSplitFT, attr, txOptions...)
}

func (t *FungibleToken) Burn(targetTokenID types.UnitID, targetTokenCounter uint64, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.BurnFungibleTokenAttributes{
		TypeID:             t.TypeID,
		Value:              t.Amount,
		TargetTokenID:      targetTokenID,
		TargetTokenCounter: targetTokenCounter,
		Counter:            t.Counter,
	}
	return NewTransactionOrder(t.NetworkID, t.PartitionID, t.ID, tokens.TransactionTypeBurnFT, attr, txOptions...)
}

func (t *FungibleToken) Join(burnTxProofs []*types.TxRecordProof, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.JoinFungibleTokenAttributes{BurnTokenProofs: burnTxProofs}
	return NewTransactionOrder(t.NetworkID, t.PartitionID, t.ID, tokens.TransactionTypeJoinFT, attr, txOptions...)
}

func (t *FungibleToken) Lock(stateLock *types.StateLock, txOptions ...Option) (*types.TransactionOrder, error) {
	return lockToken(stateLock, t.NetworkID, t.PartitionID, t.ID, t.Counter, txOptions...)
}

func (t *FungibleToken) Unlock(txOptions ...Option) (*types.TransactionOrder, error) {
	return unlockToken(t.NetworkID, t.PartitionID, t.ID, t.Counter+1, txOptions...) // +1 for not yet executed lock tx
}

func (t *FungibleToken) GetID() TokenID {
	return t.ID
}

func (t *FungibleToken) GetOwnerPredicate() Predicate {
	return t.OwnerPredicate
}

func (t *FungibleToken) GetStateLockTx() hex.Bytes {
	return t.StateLockTx
}

func (t *NonFungibleToken) Mint(pdr *types.PartitionDescriptionRecord, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.MintNonFungibleTokenAttributes{
		OwnerPredicate:      t.OwnerPredicate,
		TypeID:              t.TypeID,
		Name:                t.Name,
		URI:                 t.URI,
		Data:                t.Data,
		DataUpdatePredicate: t.DataUpdatePredicate,
		Nonce:               0,
	}
	tx, err := NewTransactionOrder(pdr.NetworkID, pdr.PartitionID, nil, tokens.TransactionTypeMintNFT, attr, txOptions...)
	if err != nil {
		return nil, fmt.Errorf("building transaction order: %w", err)
	}

	if err = tokens.GenerateUnitID(tx, pdr); err != nil {
		return nil, fmt.Errorf("generating token ID: %w", err)
	}
	t.ID = tx.UnitID

	return tx, nil
}

func (t *NonFungibleToken) Transfer(ownerPredicate []byte, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.TransferNonFungibleTokenAttributes{
		NewOwnerPredicate: ownerPredicate,
		Counter:           t.Counter,
		TypeID:            t.TypeID,
	}
	return NewTransactionOrder(t.NetworkID, t.PartitionID, t.ID, tokens.TransactionTypeTransferNFT, attr, txOptions...)
}

func (t *NonFungibleToken) Update(data []byte, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &tokens.UpdateNonFungibleTokenAttributes{
		Data:    data,
		Counter: t.Counter,
	}
	return NewTransactionOrder(t.NetworkID, t.PartitionID, t.ID, tokens.TransactionTypeUpdateNFT, attr, txOptions...)
}

func (t *NonFungibleToken) Lock(stateLock *types.StateLock, txOptions ...Option) (*types.TransactionOrder, error) {
	return lockToken(stateLock, t.NetworkID, t.PartitionID, t.ID, t.Counter, txOptions...)
}

func (t *NonFungibleToken) Unlock(txOptions ...Option) (*types.TransactionOrder, error) {
	return unlockToken(t.NetworkID, t.PartitionID, t.ID, t.Counter+1, txOptions...)
}

func (t *NonFungibleToken) GetID() TokenID {
	return t.ID
}

func (t *NonFungibleToken) GetOwnerPredicate() Predicate {
	return t.OwnerPredicate
}

func (t *NonFungibleToken) GetStateLockTx() hex.Bytes {
	return t.StateLockTx
}

func (pk PubKey) Hash() PubKeyHash {
	h := sha256.Sum256(pk)
	return h[:]
}

func lockToken(stateLock *types.StateLock, networkID types.NetworkID, partitionID types.PartitionID, id types.UnitID, counter uint64, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &nop.Attributes{
		Counter: &counter,
	}
	txOptions = append(txOptions, WithStateLock(stateLock))
	return NewTransactionOrder(networkID, partitionID, id, nop.TransactionTypeNOP, attr, txOptions...)
}

func unlockToken(networkID types.NetworkID, partitionID types.PartitionID, id types.UnitID, counter uint64, txOptions ...Option) (*types.TransactionOrder, error) {
	attr := &nop.Attributes{
		Counter: &counter,
	}
	return NewTransactionOrder(networkID, partitionID, id, nop.TransactionTypeNOP, attr, txOptions...)
}
