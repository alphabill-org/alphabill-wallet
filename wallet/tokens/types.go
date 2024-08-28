package tokens

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

const (
	predicateEmpty = "empty"
	predicateTrue  = "true"
	predicateFalse = "false"
	predicatePtpkh = "ptpkh"
	hexPrefix      = "0x"
	filePrefix     = "@"
)

type (
	PredicateInput struct {
		Argument   types.PredicateBytes
		AccountKey *account.AccountKey
	}

	DefineFungibleTokenAttributes struct {
		Symbol                   string
		Name                     string
		Icon                     *Icon
		DecimalPlaces            uint32
		ParentTypeID             sdktypes.TokenTypeID
		SubTypeCreationPredicate sdktypes.Predicate
		TokenMintingPredicate    sdktypes.Predicate
		TokenTypeOwnerPredicate  sdktypes.Predicate
	}

	Icon struct {
		Type string
		Data []byte
	}

	DefineNonFungibleTokenAttributes struct {
		Symbol                   string
		Name                     string
		Icon                     *Icon
		ParentTypeID             sdktypes.TokenTypeID
		SubTypeCreationPredicate sdktypes.Predicate
		TokenMintingPredicate    sdktypes.Predicate
		TokenTypeOwnerPredicate  sdktypes.Predicate
		DataUpdatePredicate      sdktypes.Predicate
	}

	MintNonFungibleTokenAttributes struct {
		TypeID              types.UnitID
		Name                string
		Uri                 string
		Data                []byte
		OwnerPredicate      sdktypes.Predicate
		DataUpdatePredicate sdktypes.Predicate
		Nonce               uint64
	}

	MintAttr interface {
		SetBearer([]byte)
		GetTypeID() types.UnitID
		SetTokenMintingProofs([][]byte)
	}

	AttrWithSubTypeCreationInputs interface {
		SetSubTypeCreationProofs([][]byte)
	}

	AttrWithInvariantPredicateInputs interface {
		SetInvariantProofs([][]byte)
	}
)

func ParsePredicateArguments(arguments []string, keyNr uint64, am account.Manager) ([]*PredicateInput, error) {
	creationInputs := make([]*PredicateInput, 0, len(arguments))
	for _, argument := range arguments {
		input, err := parsePredicateArgument(argument, keyNr, am)
		if err != nil {
			return nil, err
		}
		creationInputs = append(creationInputs, input)
	}
	return creationInputs, nil
}

/*
parsePredicateArgument parses the "argument" using following format:
  - empty | true | false -> will produce an empty predicate argument;
  - ptpkh (provided key #) or ptpkh:n -> will return either the default account number ("keyNr" param)
    or the user provided key index (the "n" part converted to int, must be greater than zero);
  - @filename -> will load content of the file to be used as predicate argument;
*/
func parsePredicateArgument(argument string, keyNr uint64, am account.Manager) (*PredicateInput, error) {
	switch {
	case len(argument) == 0 || argument == predicateEmpty || argument == predicateTrue || argument == predicateFalse:
		return &PredicateInput{Argument: nil}, nil
	case strings.HasPrefix(argument, predicatePtpkh):
		if split := strings.Split(argument, ":"); len(split) == 2 {
			var err error
			if keyNr, err = strconv.ParseUint(split[1], 10, 64); err != nil {
				return nil, fmt.Errorf("invalid key number: '%s': %w", argument, err)
			}
		}
		if keyNr < 1 {
			return nil, fmt.Errorf("invalid key number: %v in '%s'", keyNr, argument)
		}
		key, err := am.GetAccountKey(keyNr - 1)
		if err != nil {
			return nil, err
		}
		return &PredicateInput{AccountKey: key}, nil
	case strings.HasPrefix(argument, hexPrefix):
		decoded, err := DecodeHexOrEmpty(argument)
		if err != nil {
			return nil, err
		}
		return &PredicateInput{Argument: decoded}, nil
	case strings.HasPrefix(argument, filePrefix):
		filename, err := filepath.Abs(strings.TrimPrefix(argument, filePrefix))
		if err != nil {
			return nil, err
		}
		buf, err := os.ReadFile(filepath.Clean(filename))
		if err != nil {
			return nil, err
		}
		return &PredicateInput{Argument: buf}, nil
	default:
		return nil, fmt.Errorf("invalid predicate argument: %q", argument)
	}
}

func ParsePredicateClause(clause string, keyNr uint64, am account.Manager) ([]byte, error) {
	switch {
	case len(clause) == 0 || clause == predicateTrue:
		return templates.AlwaysTrueBytes(), nil
	case clause == predicateFalse:
		return templates.AlwaysFalseBytes(), nil
	case strings.HasPrefix(clause, hexPrefix):
		return DecodeHexOrEmpty(clause)
	case strings.HasPrefix(clause, filePrefix):
		filename, err := filepath.Abs(strings.TrimPrefix(clause, filePrefix))
		if err != nil {
			return nil, err
		}
		return os.ReadFile(filepath.Clean(filename))
	case strings.HasPrefix(clause, predicatePtpkh):
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
				var err error
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

	return nil, fmt.Errorf("invalid predicate clause: '%s'", clause)
}

func (c *DefineFungibleTokenAttributes) ToCBOR() *tokens.DefineFungibleTokenAttributes {
	var icon *tokens.Icon
	if c.Icon != nil {
		icon = &tokens.Icon{Type: c.Icon.Type, Data: c.Icon.Data}
	}
	return &tokens.DefineFungibleTokenAttributes{
		Symbol:                   c.Symbol,
		Name:                     c.Name,
		Icon:                     icon,
		DecimalPlaces:            c.DecimalPlaces,
		ParentTypeID:             c.ParentTypeID,
		SubTypeCreationPredicate: c.SubTypeCreationPredicate,
		TokenMintingPredicate:    c.TokenMintingPredicate,
		TokenTypeOwnerPredicate:  c.TokenTypeOwnerPredicate,
	}
}

func (c *DefineNonFungibleTokenAttributes) ToCBOR() *tokens.DefineNonFungibleTokenAttributes {
	var icon *tokens.Icon
	if c.Icon != nil {
		icon = &tokens.Icon{Type: c.Icon.Type, Data: c.Icon.Data}
	}
	return &tokens.DefineNonFungibleTokenAttributes{
		Symbol:                   c.Symbol,
		Name:                     c.Name,
		Icon:                     icon,
		ParentTypeID:             c.ParentTypeID,
		SubTypeCreationPredicate: c.SubTypeCreationPredicate,
		TokenMintingPredicate:    c.TokenMintingPredicate,
		TokenTypeOwnerPredicate:  c.TokenTypeOwnerPredicate,
		DataUpdatePredicate:      c.DataUpdatePredicate,
	}
}

func (a *MintNonFungibleTokenAttributes) ToCBOR() *tokens.MintNonFungibleTokenAttributes {
	return &tokens.MintNonFungibleTokenAttributes{
		OwnerPredicate:      a.OwnerPredicate,
		Name:                a.Name,
		URI:                 a.Uri,
		Data:                a.Data,
		DataUpdatePredicate: a.DataUpdatePredicate,
		Nonce:               a.Nonce,
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

func (p *PredicateInput) Proof(sigBytes []byte) ([]byte, error) {
	if p == nil {
		return nil, nil
	}
	if p.AccountKey != nil {
		signer, err := abcrypto.NewInMemorySecp256K1SignerFromKey(p.AccountKey.PrivKey)
		if err != nil {
			return nil, err
		}
		sig, err := signer.SignBytes(sigBytes)
		if err != nil {
			return nil, err
		}
		return templates.NewP2pkh256SignatureBytes(sig, p.AccountKey.PubKey), nil
	}
	return p.Argument, nil
}
