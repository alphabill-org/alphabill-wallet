package tokens

import (
	"bytes"
	"context"
	"crypto"
	"fmt"
	"log/slog"
	"math"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	test "github.com/alphabill-org/alphabill-wallet/internal/testutils"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/logger"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
)

const (
	transferFCLatestAdditionTime = 65536
	fcrTimeout                   = 1000 + transferFCLatestAdditionTime
)

func Test_GetRoundNumber_OK(t *testing.T) {
	t.Parallel()

	rpcClient := &mockTokensPartitionClient{
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 42, nil
		},
	}
	w, err := New(tokens.DefaultSystemID, rpcClient, nil, false, nil, 0, logger.New(t))
	require.NoError(t, err)

	roundNumber, err := w.GetRoundNumber(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 42, roundNumber)
}

func Test_ListTokenTypes(t *testing.T) {
	var firstPubKey *sdktypes.PubKey
	rpcClient := &mockTokensPartitionClient{
		getFungibleTokenTypes: func(ctx context.Context, pubKey sdktypes.PubKey) ([]*sdktypes.FungibleTokenType, error) {
			if !bytes.Equal(pubKey, *firstPubKey) {
				return []*sdktypes.FungibleTokenType{}, nil
			}
			return []*sdktypes.FungibleTokenType{
				{ID: test.RandomBytes(33)},
				{ID: test.RandomBytes(33)},
			}, nil
		},
		getNonFungibleTokenTypes: func(ctx context.Context, pubKey sdktypes.PubKey) ([]*sdktypes.NonFungibleTokenType, error) {
			if !bytes.Equal(pubKey, *firstPubKey) {
				return []*sdktypes.NonFungibleTokenType{}, nil
			}
			return []*sdktypes.NonFungibleTokenType{
				{ID: test.RandomBytes(33)},
				{ID: test.RandomBytes(33)},
			}, nil
		},
	}

	tw := initTestWallet(t, rpcClient)
	key, err := tw.GetAccountManager().GetPublicKey(0)
	require.NoError(t, err)
	firstPubKey = (*sdktypes.PubKey)(&key)

	fts, err := tw.ListFungibleTokenTypes(context.Background(), 0)
	require.NoError(t, err)
	require.Len(t, fts, 2)

	nfts, err := tw.ListNonFungibleTokenTypes(context.Background(), 0)
	require.NoError(t, err)
	require.Len(t, nfts, 2)

	_, err = tw.ListNonFungibleTokenTypes(context.Background(), 2)
	require.ErrorContains(t, err, "account does not exist")
}

func TestNewTypes(t *testing.T) {
	t.Parallel()

	recTxs := make(map[string]*types.TransactionOrder)
	rpcClient := &mockTokensPartitionClient{
		getFungibleTokenTypeHierarchy: func(ctx context.Context, id sdktypes.TokenTypeID) ([]*sdktypes.FungibleTokenType, error) {
			tx, found := recTxs[string(id)]
			if found {
				attrs := &tokens.DefineFungibleTokenAttributes{}
				require.NoError(t, tx.UnmarshalAttributes(attrs))
				tokenType := &sdktypes.FungibleTokenType{
					ID:            tx.UnitID(),
					ParentTypeID:  attrs.ParentTypeID,
					DecimalPlaces: attrs.DecimalPlaces,
				}
				return []*sdktypes.FungibleTokenType{tokenType}, nil
			}
			return nil, fmt.Errorf("not found")
		},
		getNonFungibleTokenTypeHierarchy: func(ctx context.Context, id sdktypes.TokenTypeID) ([]*sdktypes.NonFungibleTokenType, error) {
			tx, found := recTxs[string(id)]
			if found {
				attrs := &tokens.DefineNonFungibleTokenAttributes{}
				require.NoError(t, tx.UnmarshalAttributes(attrs))
				tokenType := &sdktypes.NonFungibleTokenType{
					ID:           tx.UnitID(),
					ParentTypeID: attrs.ParentTypeID,
				}
				return []*sdktypes.NonFungibleTokenType{tokenType}, nil
			}
			return nil, fmt.Errorf("not found")
		},
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			recTxs[string(tx.UnitID())] = tx
			return tx.Hash(crypto.SHA256), nil
		},
		getUnitsByOwnerID: func(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error) {
			// by default returns only the fee credit record id
			fcrID := tokens.NewFeeCreditRecordIDFromPublicKeyHash(nil, ownerID, fcrTimeout)
			return []types.UnitID{fcrID}, nil
		},
	}
	tw := initTestWallet(t, rpcClient)

	t.Run("fungible type", func(t *testing.T) {
		typeID := tokens.NewFungibleTokenTypeID(nil, test.RandomBytes(32))
		tt1 := &sdktypes.FungibleTokenType{
			ID:                       typeID,
			Symbol:                   "AB",
			Name:                     "Long name for AB",
			Icon:                     &tokens.Icon{Type: "image/png", Data: []byte{1}},
			DecimalPlaces:            0,
			ParentTypeID:             nil,
			SubTypeCreationPredicate: sdktypes.Predicate(templates.AlwaysFalseBytes()),
			TokenMintingPredicate:    sdktypes.Predicate(templates.AlwaysTrueBytes()),
			TokenTypeOwnerPredicate:  sdktypes.Predicate(templates.AlwaysTrueBytes()),
		}
		result, err := tw.NewFungibleType(context.Background(), 1, tt1, nil)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.EqualValues(t, typeID, result.GetUnit())
		tx, found := recTxs[string(typeID)]
		require.True(t, found)
		newFungibleTx := &tokens.DefineFungibleTokenAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(newFungibleTx))
		require.Equal(t, typeID, tx.UnitID())
		require.Equal(t, tt1.Symbol, newFungibleTx.Symbol)
		require.Equal(t, tt1.Name, newFungibleTx.Name)
		require.Equal(t, tt1.Icon.Type, newFungibleTx.Icon.Type)
		require.Equal(t, tt1.Icon.Data, newFungibleTx.Icon.Data)
		require.Equal(t, tt1.DecimalPlaces, newFungibleTx.DecimalPlaces)
		require.EqualValues(t, tx.Timeout(), 11)

		// new subtype
		tt2 := &sdktypes.FungibleTokenType{
			Symbol:                   "AB",
			Name:                     "Long name for AB",
			DecimalPlaces:            2,
			ParentTypeID:             typeID,
			SubTypeCreationPredicate: sdktypes.Predicate(templates.AlwaysFalseBytes()),
			TokenMintingPredicate:    sdktypes.Predicate(templates.AlwaysTrueBytes()),
			TokenTypeOwnerPredicate:  sdktypes.Predicate(templates.AlwaysTrueBytes()),
		}
		require.NoError(t, err)

		//check decimal places are validated against the parent type
		_, err = tw.NewFungibleType(context.Background(), 1, tt2, nil)
		require.ErrorContains(t, err, "parent type requires 0 decimal places, got 2")

		//check typeId length validation
		tt2.ID = []byte{2}
		_, err = tw.NewFungibleType(context.Background(), 1, tt2, nil)
		require.ErrorContains(t, err, "invalid token type ID: expected hex length is 66 characters (33 bytes)")

		//check typeId unit type validation
		tt2.ID = make([]byte, tokens.UnitIDLength)
		_, err = tw.NewFungibleType(context.Background(), 1, tt2, nil)
		require.ErrorContains(t, err, "invalid token type ID: expected unit type is 0x20")

		//check typeId generation if typeId parameter is nil
		tt2.ID = nil
		tt2.DecimalPlaces = 0
		result, err = tw.NewFungibleType(context.Background(), 1, tt2, nil)
		require.NoError(t, err)
		require.True(t, result.GetUnit().HasType(tokens.FungibleTokenTypeUnitType))

		//check fungible token type hierarchy
		ftType, err := tw.GetFungibleTokenType(context.Background(), tt2.ID)
		require.NoError(t, err)
		require.NotNil(t, ftType)
	})

	t.Run("non-fungible type", func(t *testing.T) {
		typeID := tokens.NewNonFungibleTokenTypeID(nil, test.RandomBytes(32))
		tt := &sdktypes.NonFungibleTokenType{
			ID:                       typeID,
			Symbol:                   "ABNFT",
			Name:                     "Long name for ABNFT",
			Icon:                     &tokens.Icon{Type: "image/svg", Data: []byte{2}},
			ParentTypeID:             nil,
			SubTypeCreationPredicate: sdktypes.Predicate(templates.AlwaysFalseBytes()),
			TokenMintingPredicate:    sdktypes.Predicate(templates.AlwaysTrueBytes()),
			TokenTypeOwnerPredicate:  sdktypes.Predicate(templates.AlwaysTrueBytes()),
		}

		result, err := tw.NewNonFungibleType(context.Background(), 1, tt, nil)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.EqualValues(t, typeID, result.GetUnit())
		tx, found := recTxs[string(typeID)]
		require.True(t, found)
		newNFTTx := &tokens.DefineNonFungibleTokenAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(newNFTTx))
		require.Equal(t, typeID, tx.UnitID())
		require.Equal(t, tt.Symbol, newNFTTx.Symbol)
		require.Equal(t, tt.Icon.Type, newNFTTx.Icon.Type)
		require.Equal(t, tt.Icon.Data, newNFTTx.Icon.Data)
		require.EqualValues(t, tx.Timeout(), 11)

		//check typeId length validation
		tt.ID = []byte{2}
		_, err = tw.NewNonFungibleType(context.Background(), 1, tt, nil)
		require.ErrorContains(t, err, "invalid token type ID: expected hex length is 66 characters (33 bytes)")

		//check typeId unit type validation
		tt.ID = make([]byte, tokens.UnitIDLength)
		_, err = tw.NewNonFungibleType(context.Background(), 1, tt, nil)
		require.ErrorContains(t, err, "invalid token type ID: expected unit type is 0x22")

		//check typeId generation if typeId parameter is nil
		tt.ID = nil
		result, _ = tw.NewNonFungibleType(context.Background(), 1, tt, nil)
		require.True(t, result.GetUnit().HasType(tokens.NonFungibleTokenTypeUnitType))

		//check non-fungible token type hierarchy
		nftType, err := tw.GetNonFungibleTokenType(context.Background(), tt.ID)
		require.NoError(t, err)
		require.NotNil(t, nftType)
	})
}

func TestNewFungibleToken(t *testing.T) {
	recTxs := make([]*types.TransactionOrder, 0)
	rpcClient := &mockTokensPartitionClient{
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			recTxs = append(recTxs, tx)
			return tx.Hash(crypto.SHA256), nil
		},
		getUnitsByOwnerID: func(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error) {
			// by default returns only the fee credit record id
			fcrID := tokens.NewFeeCreditRecordIDFromPublicKeyHash(nil, ownerID, fcrTimeout)
			return []types.UnitID{fcrID}, nil
		},
	}
	tw := initTestWallet(t, rpcClient)
	_, _, err := tw.am.AddAccount()
	require.NoError(t, err)

	tests := []struct {
		name          string
		accountNumber uint64
	}{
		{
			name:          "pub key bearer predicate, account 1",
			accountNumber: uint64(1),
		},
		{
			name:          "pub key bearer predicate, account 2",
			accountNumber: uint64(2),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typeID := test.RandomBytes(33)
			amount := uint64(100)
			key, err := tw.am.GetAccountKey(tt.accountNumber - 1)
			require.NoError(t, err)

			ft := &sdktypes.FungibleToken{
				TypeID:         typeID,
				Amount:         amount,
				OwnerPredicate: ownerPredicateFromHash(key.PubKeyHash.Sha256),
			}
			require.NoError(t, err)

			result, err := tw.NewFungibleToken(context.Background(), tt.accountNumber, ft, nil)
			require.NoError(t, err)
			tx := recTxs[len(recTxs)-1]
			attr := &tokens.MintFungibleTokenAttributes{}
			require.NotNil(t, result)
			require.Len(t, tx.UnitID(), 33)
			require.True(t, tx.UnitID().HasType(tokens.FungibleTokenUnitType))
			require.EqualValues(t, tx.UnitID(), result.GetUnit())
			require.EqualValues(t, tx.UnitID(), ft.ID)

			require.NoError(t, tx.UnmarshalAttributes(attr))
			require.Equal(t, ft.TypeID, attr.TypeID)
			require.Equal(t, ft.Amount, attr.Value)
			require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(key.PubKeyHash.Sha256), attr.OwnerPredicate)
		})
	}
}

func newFungibleToken(_ *testing.T, id sdktypes.TokenID, typeID sdktypes.TokenTypeID, symbol string, amount, lockStatus uint64) *sdktypes.FungibleToken {
	return &sdktypes.FungibleToken{
		ID:         id,
		TypeID:     typeID,
		Symbol:     symbol,
		LockStatus: lockStatus,
		Amount:     amount,
	}
}

func newNonFungibleToken(t *testing.T, symbol string, ownerPredicate []byte, lockStatus, counter uint64) *sdktypes.NonFungibleToken {
	nftID, err := tokens.NewRandomNonFungibleTokenID(nil)
	require.NoError(t, err)
	nftTypeID, err := tokens.NewRandomNonFungibleTokenTypeID(nil)
	require.NoError(t, err)

	return &sdktypes.NonFungibleToken{
		ID:             nftID,
		TypeID:         nftTypeID,
		Symbol:         symbol,
		OwnerPredicate: ownerPredicate,
		LockStatus:     lockStatus,
		Counter:        counter,
	}
}

func TestSendFungible(t *testing.T) {
	recTxs := make([]*types.TransactionOrder, 0)
	typeId := test.RandomBytes(32)
	typeId2 := test.RandomBytes(32)
	typeIdForOverflow := test.RandomBytes(32)
	rpcClient := &mockTokensPartitionClient{
		getFungibleTokens: func(ctx context.Context, ownerID []byte) ([]*sdktypes.FungibleToken, error) {
			return []*sdktypes.FungibleToken{
				newFungibleToken(t, test.RandomBytes(32), typeId, "AB", 3, 0),
				newFungibleToken(t, test.RandomBytes(32), typeId, "AB", 5, 0),
				newFungibleToken(t, test.RandomBytes(32), typeId, "AB", 7, 0),
				newFungibleToken(t, test.RandomBytes(32), typeId, "AB", 18, 0),

				newFungibleToken(t, test.RandomBytes(32), typeIdForOverflow, "AB2", math.MaxUint64, 0),
				newFungibleToken(t, test.RandomBytes(32), typeIdForOverflow, "AB2", 1, 0),
				newFungibleToken(t, test.RandomBytes(32), typeId2, "AB3", 1, 1),
			}, nil
		},
		getUnitsByOwnerID: func(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error) {
			// by default returns only the fee credit record id
			fcrID := tokens.NewFeeCreditRecordIDFromPublicKeyHash(nil, ownerID, fcrTimeout)
			return []types.UnitID{fcrID}, nil
		},
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			recTxs = append(recTxs, tx)
			return tx.Hash(crypto.SHA256), nil
		},
	}
	tw := initTestWallet(t, rpcClient)
	_, _, err := tw.am.AddAccount()
	require.NoError(t, err)

	tests := []struct {
		name               string
		tokenTypeID        sdktypes.TokenTypeID
		targetAmount       uint64
		expectedErrorMsg   string
		verifyTransactions func(t *testing.T)
	}{
		{
			name:         "one bill is transferred",
			tokenTypeID:  typeId,
			targetAmount: 3,
			verifyTransactions: func(t *testing.T) {
				require.Equal(t, 1, len(recTxs))
				tx := recTxs[0]
				newTransfer := &tokens.TransferFungibleTokenAttributes{}
				require.NoError(t, tx.UnmarshalAttributes(newTransfer))
				require.Equal(t, uint64(3), newTransfer.Value)
			},
		},
		{
			name:         "one bill is split",
			tokenTypeID:  typeId,
			targetAmount: 4,
			verifyTransactions: func(t *testing.T) {
				require.Equal(t, 1, len(recTxs))
				tx := recTxs[0]
				newSplit := &tokens.SplitFungibleTokenAttributes{}
				require.NoError(t, tx.UnmarshalAttributes(newSplit))
				require.Equal(t, uint64(4), newSplit.TargetValue)
			},
		},
		{
			name:         "both split and transfer are submitted",
			tokenTypeID:  typeId,
			targetAmount: 26,
			verifyTransactions: func(t *testing.T) {
				var total = uint64(0)
				for _, tx := range recTxs {
					switch tx.PayloadType() {
					case tokens.PayloadTypeTransferFT:
						attrs := &tokens.TransferFungibleTokenAttributes{}
						require.NoError(t, tx.UnmarshalAttributes(attrs))
						total += attrs.Value
					case tokens.PayloadTypeSplitFT:
						attrs := &tokens.SplitFungibleTokenAttributes{}
						require.NoError(t, tx.UnmarshalAttributes(attrs))
						total += attrs.TargetValue
					default:
						t.Errorf("unexpected tx type: %s", tx.PayloadType())
					}
				}
				require.Equal(t, uint64(26), total)
			},
		},
		{
			name:             "insufficient balance",
			tokenTypeID:      typeId,
			targetAmount:     60,
			expectedErrorMsg: fmt.Sprintf("insufficient tokens of type %s: got 33, need 60", sdktypes.TokenTypeID(typeId)),
		},
		{
			name:             "zero amount",
			tokenTypeID:      typeId,
			targetAmount:     0,
			expectedErrorMsg: "invalid amount",
		},
		{
			name:         "total balance uint64 overflow, transfer is submitted",
			tokenTypeID:  typeIdForOverflow,
			targetAmount: 1,
			verifyTransactions: func(t *testing.T) {
				require.Equal(t, 1, len(recTxs))
				tx := recTxs[0]
				newTransfer := &tokens.TransferFungibleTokenAttributes{}
				require.NoError(t, tx.UnmarshalAttributes(newTransfer))
				require.Equal(t, uint64(1), newTransfer.Value)
			},
		},
		{
			name:         "total balance uint64 overflow, transfer is submitted with MaxUint64",
			tokenTypeID:  typeIdForOverflow,
			targetAmount: math.MaxUint64,
			verifyTransactions: func(t *testing.T) {
				require.Equal(t, 1, len(recTxs))
				tx := recTxs[0]
				newTransfer := &tokens.TransferFungibleTokenAttributes{}
				require.NoError(t, tx.UnmarshalAttributes(newTransfer))
				require.Equal(t, uint64(math.MaxUint64), newTransfer.Value)
			},
		},
		{
			name:         "total balance uint64 overflow, split is submitted",
			tokenTypeID:  typeIdForOverflow,
			targetAmount: 2,
			verifyTransactions: func(t *testing.T) {
				require.Equal(t, 1, len(recTxs))
				tx := recTxs[0]
				newSplit := &tokens.SplitFungibleTokenAttributes{}
				require.NoError(t, tx.UnmarshalAttributes(newSplit))
				require.Equal(t, uint64(2), newSplit.TargetValue)
			},
		},
		{
			name:             "locked tokens are ignored",
			tokenTypeID:      typeId2,
			targetAmount:     1,
			expectedErrorMsg: fmt.Sprintf("insufficient tokens of type %s: got 0, need 1", sdktypes.TokenTypeID(typeId2)),
		},
	}

	key, err := tw.am.GetAccountKey(1)
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recTxs = make([]*types.TransactionOrder, 0)
			result, err := tw.SendFungible(context.Background(), 1, tt.tokenTypeID, tt.targetAmount, nil, defaultProof(key), nil)
			if tt.expectedErrorMsg != "" {
				require.ErrorContains(t, err, tt.expectedErrorMsg)
				return
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
			}
			tt.verifyTransactions(t)
		})
	}
}

func TestNewNFT_InvalidInputs(t *testing.T) {
	accountNumber := uint64(1)
	tests := []struct {
		name       string
		nft        *sdktypes.NonFungibleToken
		wantErrStr string
	}{
		{
			name: "invalid name",
			nft: &sdktypes.NonFungibleToken{
				Name: fmt.Sprintf("%x", test.RandomBytes(129))[:257],
			},
			wantErrStr: "name exceeds the maximum allowed size of 256 bytes",
		},
		{
			name: "invalid URI",
			nft: &sdktypes.NonFungibleToken{
				URI: "invalid_uri",
			},
			wantErrStr: "URI 'invalid_uri' is invalid",
		},
		{
			name: "URI exceeds maximum allowed length",
			nft: &sdktypes.NonFungibleToken{
				URI: string(test.RandomBytes(4097)),
			},
			wantErrStr: "URI exceeds the maximum allowed size of 4096 bytes",
		},
		{
			name: "data exceeds maximum allowed length",
			nft: &sdktypes.NonFungibleToken{
				Data: test.RandomBytes(65537),
			},
			wantErrStr: "data exceeds the maximum allowed size of 65536 bytes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Wallet{log: logger.New(t)}
			got, err := w.NewNFT(context.Background(), accountNumber, tt.nft, nil)
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, got)
		})
	}
}

func TestNewNFT(t *testing.T) {
	recTxs := make([]*types.TransactionOrder, 0)
	rpcClient := &mockTokensPartitionClient{
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			recTxs = append(recTxs, tx)
			return tx.Hash(crypto.SHA256), nil
		},
		getUnitsByOwnerID: func(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error) {
			// by default returns only the fee credit record id
			fcrID := tokens.NewFeeCreditRecordIDFromPublicKeyHash(nil, ownerID, fcrTimeout)
			return []types.UnitID{fcrID}, nil
		},
	}
	tw := initTestWallet(t, rpcClient)
	_, _, err := tw.am.AddAccount()
	require.NoError(t, err)

	tests := []struct {
		name          string
		accountNumber uint64
		validateOwner func(t *testing.T, accountNumber uint64, tok *tokens.MintNonFungibleTokenAttributes)
	}{
		{
			name:          "pub key bearer predicate, account 1",
			accountNumber: uint64(1),
			validateOwner: func(t *testing.T, accountNumber uint64, tok *tokens.MintNonFungibleTokenAttributes) {
				key, err := tw.am.GetAccountKey(accountNumber - 1)
				require.NoError(t, err)
				require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(key.PubKeyHash.Sha256), tok.OwnerPredicate)
			},
		},
		{
			name:          "pub key bearer predicate, account 1, predefined token ID",
			accountNumber: uint64(1),
			validateOwner: func(t *testing.T, accountNumber uint64, tok *tokens.MintNonFungibleTokenAttributes) {
				key, err := tw.am.GetAccountKey(accountNumber - 1)
				require.NoError(t, err)
				require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(key.PubKeyHash.Sha256), tok.OwnerPredicate)
			},
		},
		{
			name:          "pub key bearer predicate, account 2",
			accountNumber: uint64(2),
			validateOwner: func(t *testing.T, accountNumber uint64, tok *tokens.MintNonFungibleTokenAttributes) {
				key, err := tw.am.GetAccountKey(accountNumber - 1)
				require.NoError(t, err)
				require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(key.PubKeyHash.Sha256), tok.OwnerPredicate)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := tw.am.GetAccountKey(tt.accountNumber - 1)
			require.NoError(t, err)
			nft := &sdktypes.NonFungibleToken{
				SystemID:            tokens.DefaultSystemID,
				TypeID:              tokens.NewNonFungibleTokenTypeID(nil, test.RandomBytes(32)),
				OwnerPredicate:      ownerPredicateFromHash(key.PubKeyHash.Sha256),
				URI:                 "https://alphabill.org",
				Data:                nil,
				DataUpdatePredicate: sdktypes.Predicate(templates.AlwaysTrueBytes()),
			}
			result, err := tw.NewNFT(context.Background(), tt.accountNumber, nft, nil)
			require.NoError(t, err)
			tx := recTxs[len(recTxs)-1]
			require.NotNil(t, result)
			require.Len(t, tx.UnitID(), 33)
			require.EqualValues(t, tx.UnitID(), result.GetUnit())
			require.EqualValues(t, tx.UnitID(), nft.ID)
			require.True(t, tx.UnitID().HasType(tokens.NonFungibleTokenUnitType))

			attr := &tokens.MintNonFungibleTokenAttributes{}
			require.NoError(t, tx.UnmarshalAttributes(attr))
			tt.validateOwner(t, tt.accountNumber, attr)
			require.Equal(t, nft.TypeID, attr.TypeID)
			require.Equal(t, nft.URI, attr.URI)
			require.Equal(t, nft.Data, attr.Data)
			require.Equal(t, nft.Name, attr.Name)
			require.EqualValues(t, nft.DataUpdatePredicate, attr.DataUpdatePredicate)
			require.Equal(t, nft.OwnerPredicate, attr.OwnerPredicate)
		})
	}
}

func TestTransferNFT(t *testing.T) {
	tokenz := make(map[string]*sdktypes.NonFungibleToken)
	recTxs := make(map[string]*types.TransactionOrder)
	rpcClient := &mockTokensPartitionClient{
		getNonFungibleToken: func(ctx context.Context, id sdktypes.TokenID) (*sdktypes.NonFungibleToken, error) {
			return tokenz[string(id)], nil
		},
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			recTxs[string(tx.UnitID())] = tx
			return tx.Hash(crypto.SHA256), nil
		},
		getUnitsByOwnerID: func(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error) {
			// by default returns only the fee credit record id
			fcrID := tokens.NewFeeCreditRecordIDFromPublicKeyHash(nil, ownerID, fcrTimeout)
			return []types.UnitID{fcrID}, nil
		},
	}
	tw := initTestWallet(t, rpcClient)
	ak, err := tw.am.GetAccountKey(0)
	require.NoError(t, err)

	first := func(s sdktypes.PubKey, e error) sdktypes.PubKey {
		require.NoError(t, e)
		return s
	}
	tests := []struct {
		name          string
		token         *sdktypes.NonFungibleToken
		key           sdktypes.PubKey
		validateOwner func(t *testing.T, accountNumber uint64, key sdktypes.PubKey, tok *tokens.TransferNonFungibleTokenAttributes)
		wantErr       string
	}{
		{
			name:  "to 'always true' predicate",
			token: newNonFungibleToken(t, "AB", templates.NewP2pkh256BytesFromKey(ak.PubKey), 0, 0),
			key:   nil,
			validateOwner: func(t *testing.T, accountNumber uint64, key sdktypes.PubKey, tok *tokens.TransferNonFungibleTokenAttributes) {
				require.EqualValues(t, templates.AlwaysTrueBytes(), tok.NewOwnerPredicate)
			},
		},
		{
			name:  "to public key hash predicate",
			token: newNonFungibleToken(t, "AB", templates.NewP2pkh256BytesFromKey(ak.PubKey), 0, 0),
			key:   first(hexutil.Decode("0x0290a43bc454babf1ea8b0b76fcbb01a8f27a989047cf6d6d76397cc4756321e64")),
			validateOwner: func(t *testing.T, accountNumber uint64, key sdktypes.PubKey, tok *tokens.TransferNonFungibleTokenAttributes) {
				require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(key)), tok.NewOwnerPredicate)
			},
		},
		{
			name:    "locked token is not sent",
			token:   newNonFungibleToken(t, "AB", templates.NewP2pkh256BytesFromKey(ak.PubKey), 1, 0),
			wantErr: "token is locked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenz[string(tt.token.ID)] = tt.token
			result, err := tw.TransferNFT(context.Background(), 1, tt.token.ID, tt.key, nil, defaultProof(ak))
			if tt.wantErr == "" {
				require.NoError(t, err)
				require.NotNil(t, result)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
				require.Nil(t, result)
			}
		})
	}
}

func TestUpdateNFTData(t *testing.T) {
	tokenz := make(map[string]*sdktypes.NonFungibleToken)
	recTxs := make(map[string]*types.TransactionOrder)
	rpcClient := &mockTokensPartitionClient{
		getNonFungibleToken: func(ctx context.Context, id sdktypes.TokenID) (*sdktypes.NonFungibleToken, error) {
			return tokenz[string(id)], nil
		},
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			recTxs[string(tx.UnitID())] = tx
			return tx.Hash(crypto.SHA256), nil
		},
		getUnitsByOwnerID: func(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error) {
			// by default returns only the fee credit record id
			fcrID := tokens.NewFeeCreditRecordIDFromPublicKeyHash(nil, ownerID, fcrTimeout)
			return []types.UnitID{fcrID}, nil
		},
	}
	tw := initTestWallet(t, rpcClient)
	tok := newNonFungibleToken(t, "AB", nil, 0, 0)
	tokenz[string(tok.ID)] = tok

	ak, err := tw.am.GetAccountKey(0)
	require.NoError(t, err)

	// test data, counter and predicate inputs are submitted correctly
	data := test.RandomBytes(64)
	result, err := tw.UpdateNFTData(context.Background(), 1, tok.ID, data, &PredicateInput{Argument: nil}, []*PredicateInput{{AccountKey: ak}})
	require.NoError(t, err)
	require.NotNil(t, result)
	tx, found := recTxs[string(tok.ID)]
	require.True(t, found)
	require.EqualValues(t, tok.ID, tx.UnitID())
	require.Equal(t, tokens.PayloadTypeUpdateNFT, tx.PayloadType())

	// test that locked token tx is not sent
	lockedToken := newNonFungibleToken(t, "AB", nil, 1, 0)
	tokenz[string(tok.ID)] = lockedToken
	result, err = tw.UpdateNFTData(context.Background(), 1, tok.ID, data, &PredicateInput{Argument: nil}, []*PredicateInput{{AccountKey: ak}})
	require.ErrorContains(t, err, "token is locked")
	require.Nil(t, result)
}

func TestLockToken(t *testing.T) {
	var token *sdktypes.NonFungibleToken
	recTxs := make(map[string]*types.TransactionOrder)
	rpcClient := &mockTokensPartitionClient{
		getNonFungibleToken: func(ctx context.Context, id sdktypes.TokenID) (*sdktypes.NonFungibleToken, error) {
			return token, nil
		},
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			recTxs[string(tx.UnitID())] = tx
			return tx.Hash(crypto.SHA256), nil
		},
		getUnitsByOwnerID: func(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error) {
			// by default returns only the fee credit record id
			fcrID := tokens.NewFeeCreditRecordIDFromPublicKeyHash(nil, ownerID, fcrTimeout)
			return []types.UnitID{fcrID}, nil
		},
	}
	tw := initTestWallet(t, rpcClient)
	ak, err := tw.am.GetAccountKey(0)
	require.NoError(t, err)

	// test token is already locked
	token = newNonFungibleToken(t, "AB", templates.NewP2pkh256BytesFromKey(ak.PubKey), wallet.LockReasonManual, 0)
	result, err := tw.LockToken(context.Background(), 1, token.ID, &PredicateInput{Argument: nil})
	require.ErrorContains(t, err, "token is already locked")
	require.Nil(t, result)

	// test lock token ok
	token = newNonFungibleToken(t, "AB", templates.NewP2pkh256BytesFromKey(ak.PubKey), 0, 0)
	result, err = tw.LockToken(context.Background(), 1, token.ID, &PredicateInput{Argument: nil})
	require.NoError(t, err)
	require.NotNil(t, result)
	tx, found := recTxs[string(token.ID)]
	require.True(t, found)
	require.EqualValues(t, token.ID, tx.UnitID())
	require.Equal(t, tokens.PayloadTypeLockToken, tx.PayloadType())
}

func TestUnlockToken(t *testing.T) {
	var token *sdktypes.NonFungibleToken
	recTxs := make(map[string]*types.TransactionOrder)
	rpcClient := &mockTokensPartitionClient{
		getNonFungibleToken: func(ctx context.Context, id sdktypes.TokenID) (*sdktypes.NonFungibleToken, error) {
			return token, nil
		},
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			recTxs[string(tx.UnitID())] = tx
			return tx.Hash(crypto.SHA256), nil
		},
		getUnitsByOwnerID: func(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error) {
			// by default returns only the fee credit record id
			fcrID := tokens.NewFeeCreditRecordIDFromPublicKeyHash(nil, ownerID, fcrTimeout)
			return []types.UnitID{fcrID}, nil
		},
	}
	tw := initTestWallet(t, rpcClient)
	ak, err := tw.am.GetAccountKey(0)
	require.NoError(t, err)

	// test token is already unlocked
	token = newNonFungibleToken(t, "AB", templates.NewP2pkh256BytesFromKey(ak.PubKey), 0, 0)
	result, err := tw.UnlockToken(context.Background(), 1, token.ID, &PredicateInput{Argument: nil})
	require.ErrorContains(t, err, "token is already unlocked")
	require.Nil(t, result)

	// test unlock token ok
	token = newNonFungibleToken(t, "AB", templates.NewP2pkh256BytesFromKey(ak.PubKey), wallet.LockReasonManual, 0)
	result, err = tw.UnlockToken(context.Background(), 1, token.ID, &PredicateInput{Argument: nil})
	require.NoError(t, err)
	require.NotNil(t, result)
	tx, found := recTxs[string(token.ID)]
	require.True(t, found)
	require.EqualValues(t, token.ID, tx.UnitID())
	require.Equal(t, tokens.PayloadTypeUnlockToken, tx.PayloadType())
}

func TestSendFungibleByID(t *testing.T) {
	t.Parallel()

	token := newFungibleToken(t, test.RandomBytes(32), test.RandomBytes(32), "AB", 100, 0)

	be := &mockTokensPartitionClient{
		getFungibleToken: func(ctx context.Context, id sdktypes.TokenID) (*sdktypes.FungibleToken, error) {
			if bytes.Equal(id, token.ID) {
				return token, nil
			}
			return nil, fmt.Errorf("not found")
		},
		getUnitsByOwnerID: func(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error) {
			// by default returns only the fee credit record id
			fcrID := tokens.NewFeeCreditRecordIDFromPublicKeyHash(nil, ownerID, fcrTimeout)
			return []types.UnitID{fcrID}, nil
		},
		sendTransaction: func(ctx context.Context, txs *types.TransactionOrder) ([]byte, error) {
			return nil, nil
		},
	}

	// Initialize the wallet
	w := initTestWallet(t, be)
	pk, err := w.am.GetPublicKey(0)
	require.NoError(t, err)
	token.OwnerPredicate = templates.NewP2pkh256BytesFromKey(pk)

	// Test sending fungible token by ID
	sub, err := w.SendFungibleByID(context.Background(), 1, token.ID, 50, nil, nil)
	require.NoError(t, err)
	// ensure it's a split
	require.Equal(t, tokens.PayloadTypeSplitFT, sub.Submissions[0].Transaction.PayloadType())

	sub, err = w.SendFungibleByID(context.Background(), 1, token.ID, 100, nil, nil)
	require.NoError(t, err)
	// ensure it's a transfer
	require.Equal(t, tokens.PayloadTypeTransferFT, sub.Submissions[0].Transaction.PayloadType())

	// Test sending fungible token by ID with insufficient balance
	_, err = w.SendFungibleByID(context.Background(), 1, token.ID, 200, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient FT value")

	// Test sending fungible token by ID with invalid account number
	_, err = w.SendFungibleByID(context.Background(), 0, token.ID, 50, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid account number")
}

func initTestWallet(t *testing.T, tokensClient sdktypes.TokensPartitionClient) *Wallet {
	t.Helper()
	return &Wallet{
		am:           initAccountManager(t),
		tokensClient: tokensClient,
		log:          logger.New(t),
	}
}

func initAccountManager(t *testing.T) account.Manager {
	t.Helper()
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	require.NoError(t, am.CreateKeys(""))
	return am
}

type mockTokensPartitionClient struct {
	getFungibleToken              func(ctx context.Context, id sdktypes.TokenID) (*sdktypes.FungibleToken, error)
	getFungibleTokens             func(ctx context.Context, ownerID []byte) ([]*sdktypes.FungibleToken, error)
	getFungibleTokenTypes         func(ctx context.Context, creator sdktypes.PubKey) ([]*sdktypes.FungibleTokenType, error)
	getFungibleTokenTypeHierarchy func(ctx context.Context, id sdktypes.TokenTypeID) ([]*sdktypes.FungibleTokenType, error)

	getNonFungibleToken              func(ctx context.Context, id sdktypes.TokenID) (*sdktypes.NonFungibleToken, error)
	getNonFungibleTokens             func(ctx context.Context, ownerID []byte) ([]*sdktypes.NonFungibleToken, error)
	getNonFungibleTokenTypes         func(ctx context.Context, creator sdktypes.PubKey) ([]*sdktypes.NonFungibleTokenType, error)
	getNonFungibleTokenTypeHierarchy func(ctx context.Context, id sdktypes.TokenTypeID) ([]*sdktypes.NonFungibleTokenType, error)

	getRoundNumber              func(ctx context.Context) (uint64, error)
	sendTransaction             func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error)
	confirmTransaction          func(ctx context.Context, tx *types.TransactionOrder, log *slog.Logger) (*sdktypes.Proof, error)
	getTransactionProof         func(ctx context.Context, txHash types.Bytes) (*sdktypes.Proof, error)
	getFeeCreditRecordByOwnerID func(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error)
	getBlock                    func(ctx context.Context, roundNumber uint64) (*types.Block, error)
	getUnitsByOwnerID           func(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error)
}

func (m *mockTokensPartitionClient) GetNodeInfo(ctx context.Context) (*sdktypes.NodeInfoResponse, error) {
	return &sdktypes.NodeInfoResponse{
		SystemID: 2,
		Name:     "tokens node",
	}, nil
}

func (m *mockTokensPartitionClient) GetFungibleToken(ctx context.Context, id sdktypes.TokenID) (*sdktypes.FungibleToken, error) {
	if m.getFungibleToken != nil {
		return m.getFungibleToken(ctx, id)
	}
	return nil, fmt.Errorf("GetFungibleToken not implemented")
}

func (m *mockTokensPartitionClient) GetFungibleTokens(ctx context.Context, ownerID []byte) ([]*sdktypes.FungibleToken, error) {
	if m.getFungibleTokens != nil {
		return m.getFungibleTokens(ctx, ownerID)
	}
	return nil, fmt.Errorf("GetFungibleTokens not implemented")
}

func (m *mockTokensPartitionClient) GetFungibleTokenTypes(ctx context.Context, creator sdktypes.PubKey) ([]*sdktypes.FungibleTokenType, error) {
	if m.getFungibleTokenTypes != nil {
		return m.getFungibleTokenTypes(ctx, creator)
	}
	return nil, fmt.Errorf("GetFungibleTokenTypes not implemented")
}

func (m *mockTokensPartitionClient) GetFungibleTokenTypeHierarchy(ctx context.Context, id sdktypes.TokenTypeID) ([]*sdktypes.FungibleTokenType, error) {
	if m.getFungibleTokenTypeHierarchy != nil {
		return m.getFungibleTokenTypeHierarchy(ctx, id)
	}
	return nil, fmt.Errorf("GetFungibleTokenTypeHierarchy not implemented")
}

func (m *mockTokensPartitionClient) GetNonFungibleTokenTypeHierarchy(ctx context.Context, id sdktypes.TokenTypeID) ([]*sdktypes.NonFungibleTokenType, error) {
	if m.getNonFungibleTokenTypeHierarchy != nil {
		return m.getNonFungibleTokenTypeHierarchy(ctx, id)
	}
	return nil, fmt.Errorf("GetNonFungibleTokenTypeHierarchy not implemented")
}

func (m *mockTokensPartitionClient) GetNonFungibleToken(ctx context.Context, id sdktypes.TokenID) (*sdktypes.NonFungibleToken, error) {
	if m.getNonFungibleToken != nil {
		return m.getNonFungibleToken(ctx, id)
	}
	return nil, fmt.Errorf("GetNonFungibleToken not implemented")
}

func (m *mockTokensPartitionClient) GetNonFungibleTokens(ctx context.Context, ownerID []byte) ([]*sdktypes.NonFungibleToken, error) {
	if m.getNonFungibleTokens != nil {
		return m.getNonFungibleTokens(ctx, ownerID)
	}
	return nil, fmt.Errorf("GetNonFungibleTokens not implemented")
}

func (m *mockTokensPartitionClient) GetNonFungibleTokenTypes(ctx context.Context, creator sdktypes.PubKey) ([]*sdktypes.NonFungibleTokenType, error) {
	if m.getNonFungibleTokenTypes != nil {
		return m.getNonFungibleTokenTypes(ctx, creator)
	}
	return nil, fmt.Errorf("GetNonFungibleTokenTypes not implemented")
}

func (m *mockTokensPartitionClient) GetRoundNumber(ctx context.Context) (uint64, error) {
	if m.getRoundNumber != nil {
		return m.getRoundNumber(ctx)
	}
	return 1, nil
}

func (m *mockTokensPartitionClient) SendTransaction(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
	if m.sendTransaction != nil {
		return m.sendTransaction(ctx, tx)
	}
	return nil, fmt.Errorf("SendTransaction not implemented")
}

func (m *mockTokensPartitionClient) ConfirmTransaction(ctx context.Context, tx *types.TransactionOrder, log *slog.Logger) (*sdktypes.Proof, error) {
	if m.confirmTransaction != nil {
		return m.confirmTransaction(ctx, tx, log)
	}
	return nil, fmt.Errorf("ConfirmTransaction not implemented")
}

func (m *mockTokensPartitionClient) GetTransactionProof(ctx context.Context, txHash types.Bytes) (*sdktypes.Proof, error) {
	if m.getTransactionProof != nil {
		return m.getTransactionProof(ctx, txHash)
	}
	return nil, fmt.Errorf("GetTxProof not implemented")
}

func (m *mockTokensPartitionClient) GetFeeCreditRecordByOwnerID(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
	if m.getFeeCreditRecordByOwnerID != nil {
		return m.getFeeCreditRecordByOwnerID(ctx, ownerID)
	}
	c := uint64(2)
	id := tokens.NewFeeCreditRecordID(nil, []byte{1})
	return &sdktypes.FeeCreditRecord{
		SystemID: tokens.DefaultSystemID,
		ID:       id,
		Balance:  100000,
		Counter:  &c,
	}, nil
}

func (m *mockTokensPartitionClient) GetBlock(ctx context.Context, roundNumber uint64) (*types.Block, error) {
	if m.getBlock != nil {
		return m.getBlock(ctx, roundNumber)
	}
	return nil, fmt.Errorf("GetBlock not implemented")
}

func (m *mockTokensPartitionClient) GetUnitsByOwnerID(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error) {
	if m.getUnitsByOwnerID != nil {
		return m.getUnitsByOwnerID(ctx, ownerID)
	}
	return nil, fmt.Errorf("GetUnitsByOwnerID not implemented")
}

func (m *mockTokensPartitionClient) Close() {
	// Nothing to close
}
