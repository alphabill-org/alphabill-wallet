package tokens

import (
	"crypto"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/alphabill-org/alphabill/types"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
)

const (
	predicateEmpty = "empty"
	predicateTrue  = "true"
	predicateFalse = "false"
	predicatePtpkh = "ptpkh"
	hexPrefix      = "0x"
)

type (
	PredicateInput struct {
		// first priority
		Argument types.PredicateBytes
		// if Argument empty, check AccountNumber
		AccountNumber uint64
	}

	CreateFungibleTokenTypeAttributes struct {
		Symbol                   string
		Name                     string
		Icon                     *Icon
		DecimalPlaces            uint32
		ParentTypeId             TokenTypeID
		SubTypeCreationPredicate wallet.Predicate
		TokenCreationPredicate   wallet.Predicate
		InvariantPredicate       wallet.Predicate
	}

	Icon struct {
		Type string
		Data []byte
	}

	CreateNonFungibleTokenTypeAttributes struct {
		Symbol                   string
		Name                     string
		Icon                     *Icon
		ParentTypeId             TokenTypeID
		SubTypeCreationPredicate wallet.Predicate
		TokenCreationPredicate   wallet.Predicate
		InvariantPredicate       wallet.Predicate
		DataUpdatePredicate      wallet.Predicate
	}

	MintNonFungibleTokenAttributes struct {
		Name                string
		NftType             TokenTypeID
		Uri                 string
		Data                []byte
		Bearer              wallet.Predicate
		DataUpdatePredicate wallet.Predicate
	}

	MintAttr interface {
		types.SigBytesProvider
		SetBearer([]byte)
		SetTokenCreationPredicateSignatures([][]byte)
	}

	AttrWithSubTypeCreationInputs interface {
		types.SigBytesProvider
		SetSubTypeCreationPredicateSignatures([][]byte)
	}

	AttrWithInvariantPredicateInputs interface {
		types.SigBytesProvider
		SetInvariantPredicateSignatures([][]byte)
	}
)

func ParsePredicates(arguments []string, keyNr uint64, am account.Manager) ([]*PredicateInput, error) {
	creationInputs := make([]*PredicateInput, 0, len(arguments))
	for _, argument := range arguments {
		input, err := parsePredicate(argument, keyNr, am)
		if err != nil {
			return nil, err
		}
		creationInputs = append(creationInputs, input)
	}
	return creationInputs, nil
}

// parsePredicate uses the following format:
// empty|true|false|empty produce an empty predicate argument
// ptpkh (provided key #) or ptpkh:n (n > 0) produce an argument with the signed transaction by the given key
func parsePredicate(argument string, keyNr uint64, am account.Manager) (*PredicateInput, error) {
	if len(argument) == 0 || argument == predicateEmpty || argument == predicateTrue || argument == predicateFalse {
		return &PredicateInput{Argument: nil}, nil
	}
	var err error
	if strings.HasPrefix(argument, predicatePtpkh) {
		if split := strings.Split(argument, ":"); len(split) == 2 {
			keyStr := split[1]
			if strings.HasPrefix(strings.ToLower(keyStr), hexPrefix) {
				return nil, fmt.Errorf("invalid creation input: '%s'", argument)
			} else {
				keyNr, err = strconv.ParseUint(keyStr, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid creation input: '%s': %w", argument, err)
				}
			}
		}
		if keyNr < 1 {
			return nil, fmt.Errorf("invalid key number: %v in '%s'", keyNr, argument)
		}
		_, err := am.GetAccountKey(keyNr - 1)
		if err != nil {
			return nil, err
		}
		return &PredicateInput{AccountNumber: keyNr}, nil

	}
	if strings.HasPrefix(argument, hexPrefix) {
		decoded, err := DecodeHexOrEmpty(argument)
		if err != nil {
			return nil, err
		}
		return &PredicateInput{Argument: decoded}, nil
	}
	return nil, fmt.Errorf("invalid creation input: '%s'", argument)
}

func ParsePredicateClause(clause string, keyNr uint64, am account.Manager) ([]byte, error) {
	if len(clause) == 0 || clause == predicateTrue {
		return templates.AlwaysTrueBytes(), nil
	}
	if clause == predicateFalse {
		return templates.AlwaysFalseBytes(), nil
	}

	var err error
	if strings.HasPrefix(clause, predicatePtpkh) {
		if split := strings.Split(clause, ":"); len(split) == 2 {
			keyStr := split[1]
			if strings.HasPrefix(strings.ToLower(keyStr), hexPrefix) {
				if len(keyStr) < 3 {
					return nil, fmt.Errorf("invalid predicate clause: '%s'", clause)
				}
				keyHash, err := hexutil.Decode(keyStr)
				if err != nil {
					return nil, err
				}
				return templates.NewP2pkh256BytesFromKeyHash(keyHash), nil
			} else {
				keyNr, err = strconv.ParseUint(keyStr, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid predicate clause: '%s': %w", clause, err)
				}
			}
		}
		if keyNr < 1 {
			return nil, fmt.Errorf("invalid key number: %v in '%s'", keyNr, clause)
		}
		accountKey, err := am.GetAccountKey(keyNr - 1)
		if err != nil {
			return nil, err
		}
		return templates.NewP2pkh256BytesFromKeyHash(accountKey.PubKeyHash.Sha256), nil

	}
	if strings.HasPrefix(clause, hexPrefix) {
		return DecodeHexOrEmpty(clause)
	}
	return nil, fmt.Errorf("invalid predicate clause: '%s'", clause)
}

func (c *CreateFungibleTokenTypeAttributes) ToCBOR() *tokens.CreateFungibleTokenTypeAttributes {
	var icon *tokens.Icon
	if c.Icon != nil {
		icon = &tokens.Icon{Type: c.Icon.Type, Data: c.Icon.Data}
	}
	return &tokens.CreateFungibleTokenTypeAttributes{
		Name:                     c.Name,
		Icon:                     icon,
		Symbol:                   c.Symbol,
		DecimalPlaces:            c.DecimalPlaces,
		ParentTypeID:             c.ParentTypeId,
		SubTypeCreationPredicate: c.SubTypeCreationPredicate,
		TokenCreationPredicate:   c.TokenCreationPredicate,
		InvariantPredicate:       c.InvariantPredicate,
	}
}

func (c *CreateNonFungibleTokenTypeAttributes) ToCBOR() *tokens.CreateNonFungibleTokenTypeAttributes {
	var icon *tokens.Icon
	if c.Icon != nil {
		icon = &tokens.Icon{Type: c.Icon.Type, Data: c.Icon.Data}
	}
	return &tokens.CreateNonFungibleTokenTypeAttributes{
		Symbol:                   c.Symbol,
		Name:                     c.Name,
		Icon:                     icon,
		ParentTypeID:             c.ParentTypeId,
		SubTypeCreationPredicate: c.SubTypeCreationPredicate,
		TokenCreationPredicate:   c.TokenCreationPredicate,
		InvariantPredicate:       c.InvariantPredicate,
		DataUpdatePredicate:      c.DataUpdatePredicate,
	}
}

func (a *MintNonFungibleTokenAttributes) ToCBOR() *tokens.MintNonFungibleTokenAttributes {
	return &tokens.MintNonFungibleTokenAttributes{
		Name:                a.Name,
		NFTTypeID:           a.NftType,
		URI:                 a.Uri,
		Data:                a.Data,
		Bearer:              a.Bearer,
		DataUpdatePredicate: a.DataUpdatePredicate,
	}
}

func DecodeHexOrEmpty(input string) ([]byte, error) {
	if len(input) == 0 || input == predicateEmpty {
		return nil, nil
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(strings.ToLower(input), hexPrefix))
	if err != nil {
		return nil, err
	}
	if len(decoded) == 0 {
		return nil, nil
	}
	return decoded, nil
}

// =========================
// == backend types below ==
// =========================

type (
	TokenUnitType struct {
		// common
		ID                       TokenTypeID      `json:"id"`
		ParentTypeID             TokenTypeID      `json:"parentTypeId"`
		Symbol                   string           `json:"symbol"`
		Name                     string           `json:"name,omitempty"`
		Icon                     *tokens.Icon     `json:"icon,omitempty"`
		SubTypeCreationPredicate wallet.Predicate `json:"subTypeCreationPredicate,omitempty"`
		TokenCreationPredicate   wallet.Predicate `json:"tokenCreationPredicate,omitempty"`
		InvariantPredicate       wallet.Predicate `json:"invariantPredicate,omitempty"`

		// fungible only
		DecimalPlaces uint32 `json:"decimalPlaces,omitempty"`

		// nft only
		NftDataUpdatePredicate wallet.Predicate `json:"nftDataUpdatePredicate,omitempty"`

		// meta
		Kind   Kind          `json:"kind"`
		TxHash wallet.TxHash `json:"txHash"`
	}

	TokenUnit struct {
		// common
		ID       TokenID     `json:"id"`
		Symbol   string      `json:"symbol"`
		TypeID   TokenTypeID `json:"typeId"`
		TypeName string      `json:"typeName"`
		Owner    types.Bytes `json:"owner"`
		Locked   uint64      `json:"locked"`

		// fungible only
		Amount   uint64 `json:"amount,omitempty,string"`
		Decimals uint32 `json:"decimals,omitempty"`
		Burned   bool   `json:"burned,omitempty"`

		// nft only
		NftName                string           `json:"nftName,omitempty"`
		NftURI                 string           `json:"nftUri,omitempty"`
		NftData                []byte           `json:"nftData,omitempty"`
		NftDataUpdatePredicate wallet.Predicate `json:"nftDataUpdatePredicate,omitempty"`

		// meta
		Kind   Kind          `json:"kind"`
		TxHash wallet.TxHash `json:"txHash"`
	}

	TokenID     = types.UnitID
	TokenTypeID = types.UnitID
	Kind        byte
)

const (
	Any Kind = 1 << iota
	Fungible
	NonFungible
)

var (
	NoParent = TokenTypeID(make([]byte, crypto.SHA256.Size()))
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
