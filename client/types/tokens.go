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

		GetFungibleToken(ctx context.Context, id TokenID) (FungibleToken, error)
		GetFungibleTokens(ctx context.Context, ownerID []byte) ([]FungibleToken, error)
		GetFungibleTokenTypes(ctx context.Context, creator PubKey) ([]FungibleTokenType, error)
		GetFungibleTokenTypeHierarchy(ctx context.Context, typeID TokenTypeID) ([]FungibleTokenType, error)

		GetNonFungibleToken(ctx context.Context, id TokenID) (NonFungibleToken, error)
		GetNonFungibleTokens(ctx context.Context, ownerID []byte) ([]NonFungibleToken, error)
		GetNonFungibleTokenTypes(ctx context.Context, creator PubKey) ([]NonFungibleTokenType, error)
	}

	TokenType interface {
		SystemID() types.SystemID
		ID() TokenTypeID
		ParentTypeID() TokenTypeID
		Symbol() string
		Name() string
		Icon() *tokens.Icon
		SubTypeCreationPredicate() Predicate
		TokenCreationPredicate() Predicate
		InvariantPredicate() Predicate

		Create(txOptions ...TxOption) (*types.TransactionOrder, error)
	}

	FungibleTokenType interface {
		TokenType
		DecimalPlaces() uint32
	}

	NonFungibleTokenType interface {
		TokenType
		DataUpdatePredicate() Predicate
	}

	FungibleTokenTypeParams struct {
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

	NonFungibleTokenTypeParams struct {
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

	Token interface {
		SystemID() types.SystemID
		ID() TokenID
		TypeID() TokenTypeID
		TypeName() string
		Symbol() string
		OwnerPredicate() []byte
		Nonce() []byte
		LockStatus() uint64
		Counter() uint64
		IncreaseCounter()

		Create(txOptions ...TxOption) (*types.TransactionOrder, error)
		Transfer(ownerPredicate []byte, txOptions ...TxOption) (*types.TransactionOrder, error)
		Lock(lockStatus uint64, txOptions ...TxOption) (*types.TransactionOrder, error)
		Unlock(txOptions ...TxOption) (*types.TransactionOrder, error)
	}

	FungibleToken interface {
		Token
		Amount() uint64
		DecimalPlaces() uint32
		Burned() bool

		Split(amount uint64, ownerPredicate []byte, txOptions ...TxOption) (*types.TransactionOrder, error)
		Burn(targetTokenID types.UnitID, targetTokenCounter uint64, txOptions ...TxOption) (*types.TransactionOrder, error)
		Join(burnTxs []*types.TransactionRecord, burnProofs []*types.TxProof, txOptions ...TxOption) (*types.TransactionOrder, error)
	}

	NonFungibleToken interface {
		Token
		Name() string
		URI() string
		Data() []byte
		DataUpdatePredicate() Predicate

		Update(data []byte, txOptions ...TxOption) (*types.TransactionOrder, error)
	}

	FungibleTokenParams struct {
		SystemID       types.SystemID
		TypeID         TokenTypeID
		OwnerPredicate Predicate
		Amount         uint64
	}

	NonFungibleTokenParams struct {
		SystemID            types.SystemID
		TypeID              TokenTypeID
		OwnerPredicate      Predicate
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

func (pk PubKey) Hash() PubKeyHash {
	return hash.Sum256(pk)
}
