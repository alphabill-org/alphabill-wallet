package types

import (
	"context"
	"crypto"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
)

const (
	Any Kind = 1 << iota
	Fungible
	NonFungible
)

var NoParent = TokenTypeID(make([]byte, crypto.SHA256.Size()))

type (
	TokensPartitionClient interface {
		PartitionClient

		GetToken(ctx context.Context, id TokenID) (*TokenUnit, error)
		GetTokens(ctx context.Context, kind Kind, ownerID []byte) ([]*TokenUnit, error)
		GetTokenTypes(ctx context.Context, kind Kind, creator PubKey) ([]*TokenTypeUnit, error)
		GetTypeHierarchy(ctx context.Context, typeID TokenTypeID) ([]*TokenTypeUnit, error)
	}

	TokenUnit struct {
		// common
		ID       TokenID     `json:"id"`
		Symbol   string      `json:"symbol"`
		TypeID   TokenTypeID `json:"typeId"`
		TypeName string      `json:"typeName"`
		Owner    types.Bytes `json:"owner"`
		Nonce    types.Bytes `json:"nonce,omitempty"`
		Counter  uint64      `json:"counter"`
		Locked   uint64      `json:"locked"`

		// fungible only
		Amount   uint64 `json:"amount,omitempty,string"`
		Decimals uint32 `json:"decimals,omitempty"`
		Burned   bool   `json:"burned,omitempty"`

		// nft only
		NftName                string    `json:"nftName,omitempty"`
		NftURI                 string    `json:"nftUri,omitempty"`
		NftData                []byte    `json:"nftData,omitempty"`
		NftDataUpdatePredicate Predicate `json:"nftDataUpdatePredicate,omitempty"`

		// meta
		Kind Kind `json:"kind"`
	}

	TokenTypeUnit struct {
		// common
		ID                       TokenTypeID      `json:"id"`
		ParentTypeID             TokenTypeID      `json:"parentTypeId"`
		Symbol                   string           `json:"symbol"`
		Name                     string           `json:"name,omitempty"`
		Icon                     *tokens.Icon     `json:"icon,omitempty"`
		SubTypeCreationPredicate Predicate `json:"subTypeCreationPredicate,omitempty"`
		TokenCreationPredicate   Predicate `json:"tokenCreationPredicate,omitempty"`
		InvariantPredicate       Predicate `json:"invariantPredicate,omitempty"`

		// fungible only
		DecimalPlaces uint32 `json:"decimalPlaces,omitempty"`

		// nft only
		NftDataUpdatePredicate Predicate `json:"nftDataUpdatePredicate,omitempty"`

		// meta
		Kind   Kind   `json:"kind"`
		TxHash TxHash `json:"txHash"`
	}

	Kind          byte
	TokenID     = types.UnitID
	TokenTypeID = types.UnitID
	
	TxHash     []byte
	Predicate  []byte
	PubKey     []byte
	PubKeyHash []byte
)

func (tu *TokenUnit) IsLocked() bool {
	if tu != nil {
		return tu.Locked > 0
	}
	return false
}

func (kind Kind) String() string {
	switch kind {
	case Any:
		return "all"
	case Fungible:
		return "fungible"
	case NonFungible:
		return "nft"
	}
	return "unknown"
}

func (pk PubKey) Hash() PubKeyHash {
	return hash.Sum256(pk)
}
