package tokens

import (
	"bytes"
	"strings"

	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
)

type (
	TokenUnitType struct {
		ID            TokenTypeID `json:"id"`
		ParentTypeID  TokenTypeID `json:"typeId"`
		Kind          TokenKind   `json:"kind"`
		Symbol        string      `json:"symbol"`
		DecimalPlaces uint32      `json:"decimalPlaces"`
	}

	TokenUnit struct {
		ID       TokenID     `json:"id"`
		Kind     TokenKind   `json:"kind"`
		Symbol   string      `json:"symbol"`
		TypeID   TokenTypeID `json:"typeId"`
		Amount   uint64      `json:"amount"`        // fungible only
		URI      string      `json:"uri,omitempty"` // nft only
		Backlink []byte      `json:"backlink"`
	}

	TokenKind uint

	TokenID     []byte
	TokenTypeID []byte

	TokenWithOwner struct {
		Token *TokenUnit
		Owner PublicKey
	}

	TokenTypeInfo interface {
		GetSymbol() string
		GetTypeId() TokenTypeID
	}

	PublicKey []byte

	PredicateInput struct {
		// first priority
		Argument tokens.Predicate
		// if Argument empty, check AccountNumber
		AccountNumber uint64
	}
)

// TokenKind enum
// Only insert new values to the end of the enum
// NB! remember to update stringer method
const (
	Any TokenKind = 1 << iota
	TokenType
	Token
	Fungible
	NonFungible
)

const (
	txTimeoutBlockCount               = 100
	AllAccounts                   int = -1
	alwaysTrueTokensAccountNumber     = 0

	FungibleTokenType    = TokenType | Fungible
	NonFungibleTokenType = TokenType | NonFungible
	FungibleToken        = Token | Fungible
	NonFungibleToken     = Token | NonFungible
)

func (t *TokenUnit) IsFungible() bool {
	return t.Kind&FungibleToken == FungibleToken
}

func (k TokenKind) String() string {
	if k&Any != 0 {
		return "any"
	}
	res := make([]string, 0)
	if k&TokenType != 0 {
		res = append(res, "type")
	}
	if k&Token != 0 {
		res = append(res, "token")
	}
	if k&Fungible != 0 {
		res = append(res, "fungible")
	}
	if k&NonFungible != 0 {
		res = append(res, "non-fungible")
	}
	return strings.Join(res, ",")
}

func (t TokenTypeID) equal(to TokenTypeID) bool {
	return bytes.Equal(t, to)
}

func (tp *TokenUnitType) GetSymbol() string {
	return tp.Symbol
}

func (tp *TokenUnitType) GetTypeId() TokenTypeID {
	return tp.ID
}

func (t *TokenUnit) GetSymbol() string {
	return t.Symbol
}

func (t *TokenUnit) GetTypeId() TokenTypeID {
	return t.TypeID
}

func (id TokenID) String() string {
	return string(id)
}
