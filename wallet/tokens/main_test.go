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
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
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

	rpcClient := &mockTokensRpcClient{
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 42, nil
		},
	}
	w, err := New(tokens.DefaultSystemID, rpcClient, nil, false, nil, logger.New(t))
	require.NoError(t, err)

	roundNumber, err := w.GetRoundNumber(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 42, roundNumber)
}

func Test_ListTokens(t *testing.T) {
	rpcClient := &mockTokensRpcClient{
		getTokens: func(ctx context.Context, kind sdktypes.Kind, ownerID []byte) ([]*sdktypes.TokenUnit, error) {
			fungible := []*sdktypes.TokenUnit{
				{
					ID:   test.RandomBytes(32),
					Kind: sdktypes.Fungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: sdktypes.Fungible,
				},
			}
			nfts := []*sdktypes.TokenUnit{
				{
					ID:   test.RandomBytes(32),
					Kind: sdktypes.NonFungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: sdktypes.NonFungible,
				},
			}
			switch kind {
			case sdktypes.Fungible:
				return fungible, nil
			case sdktypes.NonFungible:
				return nfts, nil
			case sdktypes.Any:
				return append(fungible, nfts...), nil
			}
			return nil, fmt.Errorf("invalid kind")
		},
	}
	tw := initTestWallet(t, rpcClient)
	tokenz, err := tw.ListTokens(context.Background(), sdktypes.Any, AllAccounts)
	require.NoError(t, err)
	require.Len(t, tokenz[1], 4)

	tokenz, err = tw.ListTokens(context.Background(), sdktypes.Fungible, AllAccounts)
	require.NoError(t, err)
	require.Len(t, tokenz[1], 2)

	tokenz, err = tw.ListTokens(context.Background(), sdktypes.NonFungible, AllAccounts)
	require.NoError(t, err)
	require.Len(t, tokenz[1], 2)
}

func Test_ListTokenTypes(t *testing.T) {
	var firstPubKey *sdktypes.PubKey
	rpcClient := &mockTokensRpcClient{
		getTokenTypes: func(ctx context.Context, kind sdktypes.Kind, pubKey sdktypes.PubKey) ([]*sdktypes.TokenTypeUnit, error) {
			if !bytes.Equal(pubKey, *firstPubKey) {
				return []*sdktypes.TokenTypeUnit{}, nil
			}

			fungible := []*sdktypes.TokenTypeUnit{
				{
					ID:   test.RandomBytes(32),
					Kind: sdktypes.Fungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: sdktypes.Fungible,
				},
			}
			nfts := []*sdktypes.TokenTypeUnit{
				{
					ID:   test.RandomBytes(32),
					Kind: sdktypes.NonFungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: sdktypes.NonFungible,
				},
			}
			switch kind {
			case sdktypes.Fungible:
				return fungible, nil
			case sdktypes.NonFungible:
				return nfts, nil
			case sdktypes.Any:
				return append(fungible, nfts...), nil
			}
			return nil, fmt.Errorf("invalid kind")
		},
	}

	tw := initTestWallet(t, rpcClient)
	key, err := tw.GetAccountManager().GetPublicKey(0)
	require.NoError(t, err)
	firstPubKey = (*sdktypes.PubKey)(&key)

	typez, err := tw.ListTokenTypes(context.Background(), 0, sdktypes.Any)
	require.NoError(t, err)
	require.Len(t, typez, 4)

	typez, err = tw.ListTokenTypes(context.Background(), 0, sdktypes.Fungible)
	require.NoError(t, err)
	require.Len(t, typez, 2)

	typez, err = tw.ListTokenTypes(context.Background(), 0, sdktypes.NonFungible)
	require.NoError(t, err)
	require.Len(t, typez, 2)

	_, err = tw.ListTokenTypes(context.Background(), 2, sdktypes.NonFungible)
	require.ErrorContains(t, err, "account does not exist")

	_, _, err = tw.am.AddAccount()
	require.NoError(t, err)

	typez, err = tw.ListTokenTypes(context.Background(), 2, sdktypes.Any)
	require.NoError(t, err)
	require.Len(t, typez, 0)
}

func TestNewTypes(t *testing.T) {
	t.Parallel()

	recTxs := make(map[string]*types.TransactionOrder)
	rpcClient := &mockTokensRpcClient{
		getTypeHierarchy: func(ctx context.Context, id sdktypes.TokenTypeID) ([]*sdktypes.TokenTypeUnit, error) {
			tx, found := recTxs[string(id)]
			if found {
				tokenType := &sdktypes.TokenTypeUnit{ID: tx.UnitID()}
				if tx.PayloadType() == tokens.PayloadTypeCreateFungibleTokenType {
					tokenType.Kind = sdktypes.Fungible
					attrs := &tokens.CreateFungibleTokenTypeAttributes{}
					require.NoError(t, tx.UnmarshalAttributes(attrs))
					tokenType.ParentTypeID = attrs.ParentTypeID
					tokenType.DecimalPlaces = attrs.DecimalPlaces
				} else {
					tokenType.Kind = sdktypes.NonFungible
					attrs := &tokens.CreateNonFungibleTokenTypeAttributes{}
					require.NoError(t, tx.UnmarshalAttributes(attrs))
					tokenType.ParentTypeID = attrs.ParentTypeID
				}
				return []*sdktypes.TokenTypeUnit{tokenType}, nil
			}
			return nil, fmt.Errorf("not found")
		},
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			recTxs[string(tx.UnitID())] = tx
			return tx.Hash(crypto.SHA256), nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditRecordByOwnerID: func(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
			return &sdktypes.FeeCreditRecord{
				ID: []byte{1},
				Data: &fc.FeeCreditRecord{
					Balance: 100000,
					Counter: 2,
				},
			}, nil
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
		a := CreateFungibleTokenTypeAttributes{
			Symbol:                   "AB",
			Name:                     "Long name for AB",
			Icon:                     &Icon{Type: "image/png", Data: []byte{1}},
			DecimalPlaces:            0,
			ParentTypeID:             nil,
			SubTypeCreationPredicate: sdktypes.Predicate(templates.AlwaysFalseBytes()),
			TokenCreationPredicate:   sdktypes.Predicate(templates.AlwaysTrueBytes()),
			InvariantPredicate:       sdktypes.Predicate(templates.AlwaysTrueBytes()),
		}
		result, err := tw.NewFungibleType(context.Background(), 1, a, typeID, nil)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.EqualValues(t, typeID, result.GetUnit())
		tx, found := recTxs[string(typeID)]
		require.True(t, found)
		newFungibleTx := &tokens.CreateFungibleTokenTypeAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(newFungibleTx))
		require.Equal(t, typeID, tx.UnitID())
		require.Equal(t, a.Symbol, newFungibleTx.Symbol)
		require.Equal(t, a.Name, newFungibleTx.Name)
		require.Equal(t, a.Icon.Type, newFungibleTx.Icon.Type)
		require.Equal(t, a.Icon.Data, newFungibleTx.Icon.Data)
		require.Equal(t, a.DecimalPlaces, newFungibleTx.DecimalPlaces)
		require.EqualValues(t, tx.Timeout(), 11)

		// new subtype
		b := CreateFungibleTokenTypeAttributes{
			Symbol:                   "AB",
			Name:                     "Long name for AB",
			DecimalPlaces:            2,
			ParentTypeID:             typeID,
			SubTypeCreationPredicate: sdktypes.Predicate(templates.AlwaysFalseBytes()),
			TokenCreationPredicate:   sdktypes.Predicate(templates.AlwaysTrueBytes()),
			InvariantPredicate:       sdktypes.Predicate(templates.AlwaysTrueBytes()),
		}
		//check decimal places are validated against the parent type
		_, err = tw.NewFungibleType(context.Background(), 1, b, nil, nil)
		require.ErrorContains(t, err, "parent type requires 0 decimal places, got 2")

		//check typeId length validation
		_, err = tw.NewFungibleType(context.Background(), 1, a, []byte{2}, nil)
		require.ErrorContains(t, err, "invalid token type ID: expected hex length is 66 characters (33 bytes)")

		//check typeId unit type validation
		_, err = tw.NewFungibleType(context.Background(), 1, a, make([]byte, tokens.UnitIDLength), nil)
		require.ErrorContains(t, err, "invalid token type ID: expected unit type is 0x20")

		//check typeId generation if typeId parameter is nil
		result, _ = tw.NewFungibleType(context.Background(), 1, a, nil, nil)
		require.True(t, result.GetUnit().HasType(tokens.FungibleTokenTypeUnitType))
	})

	t.Run("non-fungible type", func(t *testing.T) {
		typeId := tokens.NewNonFungibleTokenTypeID(nil, test.RandomBytes(32))
		a := CreateNonFungibleTokenTypeAttributes{
			Symbol:                   "ABNFT",
			Name:                     "Long name for ABNFT",
			Icon:                     &Icon{Type: "image/svg", Data: []byte{2}},
			ParentTypeID:             nil,
			SubTypeCreationPredicate: sdktypes.Predicate(templates.AlwaysFalseBytes()),
			TokenCreationPredicate:   sdktypes.Predicate(templates.AlwaysTrueBytes()),
			InvariantPredicate:       sdktypes.Predicate(templates.AlwaysTrueBytes()),
		}
		result, err := tw.NewNonFungibleType(context.Background(), 1, a, typeId, nil)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.EqualValues(t, typeId, result.GetUnit())
		tx, found := recTxs[string(typeId)]
		require.True(t, found)
		newNFTTx := &tokens.CreateNonFungibleTokenTypeAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(newNFTTx))
		require.Equal(t, typeId, tx.UnitID())
		require.Equal(t, a.Symbol, newNFTTx.Symbol)
		require.Equal(t, a.Icon.Type, newNFTTx.Icon.Type)
		require.Equal(t, a.Icon.Data, newNFTTx.Icon.Data)

		//check typeId length validation
		_, err = tw.NewNonFungibleType(context.Background(), 1, a, []byte{2}, nil)
		require.ErrorContains(t, err, "invalid token type ID: expected hex length is 66 characters (33 bytes)")

		//check typeId unit type validation
		_, err = tw.NewNonFungibleType(context.Background(), 1, a, make([]byte, tokens.UnitIDLength), nil)
		require.ErrorContains(t, err, "invalid token type ID: expected unit type is 0x22")

		//check typeId generation if typeId parameter is nil
		result, _ = tw.NewNonFungibleType(context.Background(), 1, a, nil, nil)
		require.True(t, result.GetUnit().HasType(tokens.NonFungibleTokenTypeUnitType))
	})
}

func TestMintFungibleToken(t *testing.T) {
	recTxs := make([]*types.TransactionOrder, 0)
	rpcClient := &mockTokensRpcClient{
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			recTxs = append(recTxs, tx)
			return tx.Hash(crypto.SHA256), nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditRecordByOwnerID: func(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
			return &sdktypes.FeeCreditRecord{
				ID:   []byte{1},
				Data: &fc.FeeCreditRecord{Balance: 100000, Counter: 2},
			}, nil
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
			result, err := tw.NewFungibleToken(context.Background(), tt.accountNumber, typeID, amount, bearerPredicateFromHash(key.PubKeyHash.Sha256), nil)
			require.NoError(t, err)
			tx := recTxs[len(recTxs)-1]
			attr := &tokens.MintFungibleTokenAttributes{}
			require.NotNil(t, result)
			require.EqualValues(t, tx.UnitID(), result.GetUnit())
			require.NoError(t, tx.UnmarshalAttributes(attr))
			require.NotEqual(t, []byte{0}, tx.UnitID())
			require.Len(t, tx.UnitID(), 33)
			require.Equal(t, amount, attr.Value)
			require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(key.PubKeyHash.Sha256), attr.Bearer)
		})
	}
}

func TestSendFungible(t *testing.T) {
	recTxs := make([]*types.TransactionOrder, 0)
	typeId := test.RandomBytes(32)
	typeId2 := test.RandomBytes(32)
	typeIdForOverflow := test.RandomBytes(32)
	rpcClient := &mockTokensRpcClient{
		getTokens: func(ctx context.Context, kind sdktypes.Kind, ownerID []byte) ([]*sdktypes.TokenUnit, error) {
			return []*sdktypes.TokenUnit{
				{ID: test.RandomBytes(32), Kind: sdktypes.Fungible, Symbol: "AB", TypeID: typeId, Amount: 3},
				{ID: test.RandomBytes(32), Kind: sdktypes.Fungible, Symbol: "AB", TypeID: typeId, Amount: 5},
				{ID: test.RandomBytes(32), Kind: sdktypes.Fungible, Symbol: "AB", TypeID: typeId, Amount: 7},
				{ID: test.RandomBytes(32), Kind: sdktypes.Fungible, Symbol: "AB", TypeID: typeId, Amount: 18},
				{ID: test.RandomBytes(32), Kind: sdktypes.Fungible, Symbol: "AB2", TypeID: typeIdForOverflow, Amount: math.MaxUint64},
				{ID: test.RandomBytes(32), Kind: sdktypes.Fungible, Symbol: "AB2", TypeID: typeIdForOverflow, Amount: 1},
				{ID: test.RandomBytes(32), Kind: sdktypes.Fungible, Symbol: "AB3", TypeID: typeId2, Amount: 1, Locked: 1},
			}, nil
		},
		getFeeCreditRecordByOwnerID: func(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
			return &sdktypes.FeeCreditRecord{
				ID:   []byte{1},
				Data: &fc.FeeCreditRecord{Balance: 100000, Counter: 2},
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
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
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
					case tokens.PayloadTypeTransferFungibleToken:
						attrs := &tokens.TransferFungibleTokenAttributes{}
						require.NoError(t, tx.UnmarshalAttributes(attrs))
						total += attrs.Value
					case tokens.PayloadTypeSplitFungibleToken:
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
				require.Equal(t, uint64(math.MaxUint64-2), newSplit.RemainingValue)
			},
		},
		{
			name:             "locked tokens are ignored",
			tokenTypeID:      typeId2,
			targetAmount:     1,
			expectedErrorMsg: fmt.Sprintf("insufficient tokens of type %s: got 0, need 1", sdktypes.TokenTypeID(typeId2)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recTxs = make([]*types.TransactionOrder, 0)
			result, err := tw.SendFungible(context.Background(), 1, tt.tokenTypeID, tt.targetAmount, nil, nil, defaultProof(1))
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

func TestMintNFT_InvalidInputs(t *testing.T) {
	accountNumber := uint64(1)
	tests := []struct {
		name       string
		attrs      tokens.MintNonFungibleTokenAttributes
		wantErrStr string
	}{
		{
			name: "invalid name",
			attrs: tokens.MintNonFungibleTokenAttributes{
				Name: fmt.Sprintf("%x", test.RandomBytes(129))[:257],
			},
			wantErrStr: "name exceeds the maximum allowed size of 256 bytes",
		},
		{
			name: "invalid URI",
			attrs: tokens.MintNonFungibleTokenAttributes{
				URI: "invalid_uri",
			},
			wantErrStr: "URI 'invalid_uri' is invalid",
		},
		{
			name: "URI exceeds maximum allowed length",
			attrs: tokens.MintNonFungibleTokenAttributes{
				URI: string(test.RandomBytes(4097)),
			},
			wantErrStr: "URI exceeds the maximum allowed size of 4096 bytes",
		},
		{
			name: "data exceeds maximum allowed length",
			attrs: tokens.MintNonFungibleTokenAttributes{
				Data: test.RandomBytes(65537),
			},
			wantErrStr: "data exceeds the maximum allowed size of 65536 bytes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Wallet{log: logger.New(t)}
			got, err := w.NewNFT(context.Background(), accountNumber, &tt.attrs, nil)
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, got)
		})
	}
}

func TestMintNFT(t *testing.T) {
	recTxs := make([]*types.TransactionOrder, 0)
	rpcClient := &mockTokensRpcClient{
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			recTxs = append(recTxs, tx)
			return tx.Hash(crypto.SHA256), nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditRecordByOwnerID: func(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
			return &sdktypes.FeeCreditRecord{
				ID:   []byte{1},
				Data: &fc.FeeCreditRecord{Balance: 100000, Counter: 2},
			}, nil
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
		typeID        sdktypes.TokenTypeID
		validateOwner func(t *testing.T, accountNumber uint64, tok *tokens.MintNonFungibleTokenAttributes)
	}{
		{
			name:          "pub key bearer predicate, account 1",
			accountNumber: uint64(1),
			typeID:        tokens.NewNonFungibleTokenTypeID(nil, test.RandomBytes(32)),
			validateOwner: func(t *testing.T, accountNumber uint64, tok *tokens.MintNonFungibleTokenAttributes) {
				key, err := tw.am.GetAccountKey(accountNumber - 1)
				require.NoError(t, err)
				require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(key.PubKeyHash.Sha256), tok.Bearer)
			},
		},
		{
			name:          "pub key bearer predicate, account 1, predefined token ID",
			accountNumber: uint64(1),
			typeID:        tokens.NewNonFungibleTokenTypeID(nil, test.RandomBytes(32)),
			validateOwner: func(t *testing.T, accountNumber uint64, tok *tokens.MintNonFungibleTokenAttributes) {
				key, err := tw.am.GetAccountKey(accountNumber - 1)
				require.NoError(t, err)
				require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(key.PubKeyHash.Sha256), tok.Bearer)
			},
		},
		{
			name:          "pub key bearer predicate, account 2",
			accountNumber: uint64(2),
			typeID:        tokens.NewNonFungibleTokenTypeID(nil, test.RandomBytes(32)),
			validateOwner: func(t *testing.T, accountNumber uint64, tok *tokens.MintNonFungibleTokenAttributes) {
				key, err := tw.am.GetAccountKey(accountNumber - 1)
				require.NoError(t, err)
				require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(key.PubKeyHash.Sha256), tok.Bearer)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := tw.am.GetAccountKey(tt.accountNumber - 1)
			require.NoError(t, err)
			a := &tokens.MintNonFungibleTokenAttributes{
				TypeID:              tt.typeID,
				Bearer:              bearerPredicateFromHash(key.PubKeyHash.Sha256),
				URI:                 "https://alphabill.org",
				Data:                nil,
				DataUpdatePredicate: sdktypes.Predicate(templates.AlwaysTrueBytes()),
			}
			result, err := tw.NewNFT(context.Background(), tt.accountNumber, a, nil)
			require.NoError(t, err)
			tx := recTxs[len(recTxs)-1]
			require.NotNil(t, result)
			require.EqualValues(t, tx.UnitID(), result.GetUnit())
			require.NotEqual(t, []byte{0}, tx.UnitID())
			require.Len(t, tx.UnitID(), 33)

			attr := &tokens.MintNonFungibleTokenAttributes{}
			require.NoError(t, tx.UnmarshalAttributes(attr))
			tt.validateOwner(t, tt.accountNumber, attr)
		})
	}
}

func TestTransferNFT(t *testing.T) {
	tokenz := make(map[string]*sdktypes.TokenUnit)
	recTxs := make(map[string]*types.TransactionOrder)
	rpcClient := &mockTokensRpcClient{
		getToken: func(ctx context.Context, id sdktypes.TokenID) (*sdktypes.TokenUnit, error) {
			return tokenz[string(id)], nil
		},
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			recTxs[string(tx.UnitID())] = tx
			return tx.Hash(crypto.SHA256), nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditRecordByOwnerID: func(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
			return &sdktypes.FeeCreditRecord{
				ID:   []byte{1},
				Data: &fc.FeeCreditRecord{Balance: 100000, Counter: 2},
			}, nil
		},
		getUnitsByOwnerID: func(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error) {
			// by default returns only the fee credit record id
			fcrID := tokens.NewFeeCreditRecordIDFromPublicKeyHash(nil, ownerID, fcrTimeout)
			return []types.UnitID{fcrID}, nil
		},
	}
	tw := initTestWallet(t, rpcClient)
	pk, err := tw.am.GetPublicKey(0)
	require.NoError(t, err)

	first := func(s sdktypes.PubKey, e error) sdktypes.PubKey {
		require.NoError(t, e)
		return s
	}
	tests := []struct {
		name          string
		token         *sdktypes.TokenUnit
		key           sdktypes.PubKey
		validateOwner func(t *testing.T, accountNumber uint64, key sdktypes.PubKey, tok *tokens.TransferNonFungibleTokenAttributes)
		wantErr       string
	}{
		{
			name:  "to 'always true' predicate",
			token: &sdktypes.TokenUnit{ID: test.RandomBytes(32), Kind: sdktypes.NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32), Owner: templates.NewP2pkh256BytesFromKey(pk)},
			key:   nil,
			validateOwner: func(t *testing.T, accountNumber uint64, key sdktypes.PubKey, tok *tokens.TransferNonFungibleTokenAttributes) {
				require.EqualValues(t, templates.AlwaysTrueBytes(), tok.NewBearer)
			},
		},
		{
			name:  "to public key hash predicate",
			token: &sdktypes.TokenUnit{ID: test.RandomBytes(32), Kind: sdktypes.NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32), Owner: templates.NewP2pkh256BytesFromKey(pk)},
			key:   first(hexutil.Decode("0x0290a43bc454babf1ea8b0b76fcbb01a8f27a989047cf6d6d76397cc4756321e64")),
			validateOwner: func(t *testing.T, accountNumber uint64, key sdktypes.PubKey, tok *tokens.TransferNonFungibleTokenAttributes) {
				require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(key)), tok.NewBearer)
			},
		},
		{
			name:    "locked token is not sent",
			token:   &sdktypes.TokenUnit{ID: test.RandomBytes(32), Kind: sdktypes.NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32), Locked: 1, Owner: templates.NewP2pkh256BytesFromKey(pk)},
			wantErr: "token is locked",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenz[string(tt.token.ID)] = tt.token
			result, err := tw.TransferNFT(context.Background(), 1, tt.token.ID, tt.key, nil, defaultProof(1))
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
	tokenz := make(map[string]*sdktypes.TokenUnit)
	recTxs := make(map[string]*types.TransactionOrder)
	rpcClient := &mockTokensRpcClient{
		getToken: func(ctx context.Context, id sdktypes.TokenID) (*sdktypes.TokenUnit, error) {
			return tokenz[string(id)], nil
		},
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			recTxs[string(tx.UnitID())] = tx
			return tx.Hash(crypto.SHA256), nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditRecordByOwnerID: func(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
			return &sdktypes.FeeCreditRecord{
				ID:   []byte{1},
				Data: &fc.FeeCreditRecord{Balance: 100000, Counter: 2},
			}, nil
		},
		getUnitsByOwnerID: func(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error) {
			// by default returns only the fee credit record id
			fcrID := tokens.NewFeeCreditRecordIDFromPublicKeyHash(nil, ownerID, fcrTimeout)
			return []types.UnitID{fcrID}, nil
		},
	}
	tw := initTestWallet(t, rpcClient)

	parseNFTDataUpdate := func(t *testing.T, tx *types.TransactionOrder) *tokens.UpdateNonFungibleTokenAttributes {
		t.Helper()
		newTransfer := &tokens.UpdateNonFungibleTokenAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(newTransfer))
		return newTransfer
	}

	tok := &sdktypes.TokenUnit{ID: test.RandomBytes(32), Kind: sdktypes.NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32), Counter: 0}
	tokenz[string(tok.ID)] = tok

	// test data, counter and predicate inputs are submitted correctly
	data := test.RandomBytes(64)
	result, err := tw.UpdateNFTData(context.Background(), 1, tok.ID, data, []*PredicateInput{{Argument: nil}})
	require.NoError(t, err)
	require.NotNil(t, result)
	tx, found := recTxs[string(tok.ID)]
	require.True(t, found)

	dataUpdate := parseNFTDataUpdate(t, tx)
	require.Equal(t, data, dataUpdate.Data)
	require.Equal(t, tok.Counter, dataUpdate.Counter)
	require.Equal(t, [][]byte{nil}, dataUpdate.DataUpdateSignatures)

	// test that wallet not only sends the tx, but also reads it correctly
	data2 := test.RandomBytes(64)
	result, err = tw.UpdateNFTData(context.Background(), 1, tok.ID, data2, []*PredicateInput{{Argument: nil}, {AccountNumber: 1}})
	require.NoError(t, err)
	require.NotNil(t, result)
	tx, found = recTxs[string(tok.ID)]
	require.True(t, found)
	dataUpdate = parseNFTDataUpdate(t, tx)
	require.NotEqual(t, data, dataUpdate.Data)
	require.Equal(t, data2, dataUpdate.Data)
	require.Len(t, dataUpdate.DataUpdateSignatures, 2)
	require.Equal(t, []byte(nil), dataUpdate.DataUpdateSignatures[0])
	require.Len(t, dataUpdate.DataUpdateSignatures[1], 103)

	// test that locked token tx is not sent
	lockedToken := &sdktypes.TokenUnit{ID: test.RandomBytes(32), Kind: sdktypes.NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32), Counter: 0, Locked: 1}
	tokenz[string(tok.ID)] = lockedToken
	result, err = tw.UpdateNFTData(context.Background(), 1, tok.ID, data2, []*PredicateInput{{Argument: nil}, {AccountNumber: 1}})
	require.ErrorContains(t, err, "token is locked")
	require.Nil(t, result)
}

func TestLockToken(t *testing.T) {
	var token *sdktypes.TokenUnit
	recTxs := make(map[string]*types.TransactionOrder)
	rpcClient := &mockTokensRpcClient{
		getToken: func(ctx context.Context, id sdktypes.TokenID) (*sdktypes.TokenUnit, error) {
			return token, nil
		},
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			recTxs[string(tx.UnitID())] = tx
			return tx.Hash(crypto.SHA256), nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditRecordByOwnerID: func(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
			return &sdktypes.FeeCreditRecord{
				ID:   []byte{1},
				Data: &fc.FeeCreditRecord{Balance: 100000, Counter: 2},
			}, nil
		},
		getUnitsByOwnerID: func(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error) {
			// by default returns only the fee credit record id
			fcrID := tokens.NewFeeCreditRecordIDFromPublicKeyHash(nil, ownerID, fcrTimeout)
			return []types.UnitID{fcrID}, nil
		},
	}
	tw := initTestWallet(t, rpcClient)
	pk, err := tw.am.GetPublicKey(0)
	require.NoError(t, err)

	// test token is already locked
	token = &sdktypes.TokenUnit{ID: test.RandomBytes(32), Kind: sdktypes.NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32), Locked: wallet.LockReasonManual, Owner: templates.NewP2pkh256BytesFromKey(pk)}
	result, err := tw.LockToken(context.Background(), 1, token.ID, []*PredicateInput{{Argument: nil}}, defaultProof(1))
	require.ErrorContains(t, err, "token is already locked")
	require.Nil(t, result)

	// test lock token ok
	token = &sdktypes.TokenUnit{ID: test.RandomBytes(32), Kind: sdktypes.NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32), Owner: templates.NewP2pkh256BytesFromKey(pk)}
	result, err = tw.LockToken(context.Background(), 1, token.ID, []*PredicateInput{{Argument: nil}}, defaultProof(1))
	require.NoError(t, err)
	require.NotNil(t, result)
	tx, found := recTxs[string(token.ID)]
	require.True(t, found)
	require.EqualValues(t, token.ID, tx.UnitID())
	require.Equal(t, tokens.PayloadTypeLockToken, tx.PayloadType())
}

func TestUnlockToken(t *testing.T) {
	var token *sdktypes.TokenUnit
	recTxs := make(map[string]*types.TransactionOrder)
	rpcClient := &mockTokensRpcClient{
		getToken: func(ctx context.Context, id sdktypes.TokenID) (*sdktypes.TokenUnit, error) {
			return token, nil
		},
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			recTxs[string(tx.UnitID())] = tx
			return tx.Hash(crypto.SHA256), nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditRecordByOwnerID: func(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
			return &sdktypes.FeeCreditRecord{
				ID:   []byte{1},
				Data: &fc.FeeCreditRecord{Balance: 100000, Counter: 2},
			}, nil
		},
		getUnitsByOwnerID: func(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error) {
			// by default returns only the fee credit record id
			fcrID := tokens.NewFeeCreditRecordIDFromPublicKeyHash(nil, ownerID, fcrTimeout)
			return []types.UnitID{fcrID}, nil
		},
	}
	tw := initTestWallet(t, rpcClient)
	pk, err := tw.am.GetPublicKey(0)
	require.NoError(t, err)

	// test token is already unlocked
	token = &sdktypes.TokenUnit{ID: test.RandomBytes(32), Kind: sdktypes.NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32), Owner: templates.NewP2pkh256BytesFromKey(pk)}
	result, err := tw.UnlockToken(context.Background(), 1, token.ID, []*PredicateInput{{Argument: nil}}, defaultProof(1))
	require.ErrorContains(t, err, "token is already unlocked")
	require.Nil(t, result)

	// test unlock token ok
	token = &sdktypes.TokenUnit{ID: test.RandomBytes(32), Kind: sdktypes.NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32), Locked: wallet.LockReasonManual, Owner: templates.NewP2pkh256BytesFromKey(pk)}
	result, err = tw.UnlockToken(context.Background(), 1, token.ID, []*PredicateInput{{Argument: nil}}, defaultProof(1))
	require.NoError(t, err)
	require.NotNil(t, result)
	tx, found := recTxs[string(token.ID)]
	require.True(t, found)
	require.EqualValues(t, token.ID, tx.UnitID())
	require.Equal(t, tokens.PayloadTypeUnlockToken, tx.PayloadType())
}

func TestSendFungibleByID(t *testing.T) {
	t.Parallel()

	token := &sdktypes.TokenUnit{
		ID:     test.RandomBytes(32),
		Kind:   sdktypes.Fungible,
		Symbol: "AB",
		TypeID: test.RandomBytes(32),
		Amount: 100,
	}

	be := &mockTokensRpcClient{
		getToken: func(ctx context.Context, id sdktypes.TokenID) (*sdktypes.TokenUnit, error) {
			if bytes.Equal(id, token.ID) {
				return token, nil
			}
			return nil, fmt.Errorf("not found")
		},
		getFeeCreditRecordByOwnerID: func(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
			return &sdktypes.FeeCreditRecord{
				ID: []byte{1},
				Data: &fc.FeeCreditRecord{
					Balance: 50,
				},
			}, nil
		},
		getUnitsByOwnerID: func(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error) {
			// by default returns only the fee credit record id
			fcrID := tokens.NewFeeCreditRecordIDFromPublicKeyHash(nil, ownerID, fcrTimeout)
			return []types.UnitID{fcrID}, nil
		},
		sendTransaction: func(ctx context.Context, txs *types.TransactionOrder) ([]byte, error) {
			return nil, nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
	}

	// Initialize the wallet
	w := initTestWallet(t, be)
	pk, err := w.am.GetPublicKey(0)
	require.NoError(t, err)
	token.Owner = templates.NewP2pkh256BytesFromKey(pk)

	// Test sending fungible token by ID
	sub, err := w.SendFungibleByID(context.Background(), 1, token.ID, 50, nil, nil)
	require.NoError(t, err)
	// ensure it's a split
	require.Equal(t, tokens.PayloadTypeSplitFungibleToken, sub.Submissions[0].Transaction.PayloadType())

	sub, err = w.SendFungibleByID(context.Background(), 1, token.ID, 100, nil, nil)
	require.NoError(t, err)
	// ensure it's a transfer
	require.Equal(t, tokens.PayloadTypeTransferFungibleToken, sub.Submissions[0].Transaction.PayloadType())

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

type mockTokensRpcClient struct {
	getToken                    func(ctx context.Context, id sdktypes.TokenID) (*sdktypes.TokenUnit, error)
	getTokens                   func(ctx context.Context, kind sdktypes.Kind, ownerID []byte) ([]*sdktypes.TokenUnit, error)
	getTokenTypes               func(ctx context.Context, kind sdktypes.Kind, creator sdktypes.PubKey) ([]*sdktypes.TokenTypeUnit, error)
	getTypeHierarchy            func(ctx context.Context, id sdktypes.TokenTypeID) ([]*sdktypes.TokenTypeUnit, error)
	getRoundNumber              func(ctx context.Context) (uint64, error)
	sendTransaction             func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error)
	confirmTransaction          func(ctx context.Context, tx *types.TransactionOrder, log *slog.Logger) (*sdktypes.Proof, error)
	getTransactionProof         func(ctx context.Context, txHash types.Bytes) (*sdktypes.Proof, error)
	getFeeCreditRecordByOwnerID func(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error)
	getBlock                    func(ctx context.Context, roundNumber uint64) (*types.Block, error)
	getUnitsByOwnerID           func(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error)
}

func (m *mockTokensRpcClient) GetNodeInfo(ctx context.Context) (*sdktypes.NodeInfoResponse, error) {
	return &sdktypes.NodeInfoResponse{
		SystemID: 2,
		Name:     "tokens node",
	}, nil
}

func (m *mockTokensRpcClient) GetToken(ctx context.Context, id sdktypes.TokenID) (*sdktypes.TokenUnit, error) {
	if m.getToken != nil {
		return m.getToken(ctx, id)
	}
	return nil, fmt.Errorf("GetToken not implemented")
}

func (m *mockTokensRpcClient) GetTokens(ctx context.Context, kind sdktypes.Kind, ownerID []byte) ([]*sdktypes.TokenUnit, error) {
	if m.getTokens != nil {
		return m.getTokens(ctx, kind, ownerID)
	}
	return nil, fmt.Errorf("GetTokens not implemented")
}

func (m *mockTokensRpcClient) GetTokenTypes(ctx context.Context, kind sdktypes.Kind, creator sdktypes.PubKey) ([]*sdktypes.TokenTypeUnit, error) {
	if m.getTokenTypes != nil {
		return m.getTokenTypes(ctx, kind, creator)
	}
	return nil, fmt.Errorf("GetTokenTypes not implemented")
}

func (m *mockTokensRpcClient) GetTypeHierarchy(ctx context.Context, id sdktypes.TokenTypeID) ([]*sdktypes.TokenTypeUnit, error) {
	if m.getTypeHierarchy != nil {
		return m.getTypeHierarchy(ctx, id)
	}
	return nil, fmt.Errorf("GetTypeHierarchy not implemented")
}

func (m *mockTokensRpcClient) GetRoundNumber(ctx context.Context) (uint64, error) {
	if m.getRoundNumber != nil {
		return m.getRoundNumber(ctx)
	}
	return 0, fmt.Errorf("GetRoundNumber not implemented")
}

func (m *mockTokensRpcClient) SendTransaction(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
	if m.sendTransaction != nil {
		return m.sendTransaction(ctx, tx)
	}
	return nil, fmt.Errorf("SendTransaction not implemented")
}

func (m *mockTokensRpcClient) ConfirmTransaction(ctx context.Context, tx *types.TransactionOrder, log *slog.Logger) (*sdktypes.Proof, error) {
	if m.confirmTransaction != nil {
		return m.confirmTransaction(ctx, tx, log)
	}
	return nil, fmt.Errorf("ConfirmTransaction not implemented")
}

func (m *mockTokensRpcClient) GetTransactionProof(ctx context.Context, txHash types.Bytes) (*sdktypes.Proof, error) {
	if m.getTransactionProof != nil {
		return m.getTransactionProof(ctx, txHash)
	}
	return nil, fmt.Errorf("GetTxProof not implemented")
}

func (m *mockTokensRpcClient) GetFeeCreditRecordByOwnerID(ctx context.Context, ownerID []byte) (*sdktypes.FeeCreditRecord, error) {
	if m.getFeeCreditRecordByOwnerID != nil {
		return m.getFeeCreditRecordByOwnerID(ctx, ownerID)
	}
	return nil, fmt.Errorf("GetFeeCreditRecordByOwnerID not implemented")
}

func (m *mockTokensRpcClient) GetBlock(ctx context.Context, roundNumber uint64) (*types.Block, error) {
	if m.getBlock != nil {
		return m.getBlock(ctx, roundNumber)
	}
	return nil, fmt.Errorf("GetBlock not implemented")
}

func (m *mockTokensRpcClient) GetUnitsByOwnerID(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error) {
	if m.getUnitsByOwnerID != nil {
		return m.getUnitsByOwnerID(ctx, ownerID)
	}
	return nil, fmt.Errorf("GetUnitsByOwnerID not implemented")
}

func (m *mockTokensRpcClient) Close() {
	// Nothing to close
}
