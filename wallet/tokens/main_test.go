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

	"github.com/alphabill-org/alphabill-wallet/client"
	"github.com/alphabill-org/alphabill-wallet/client/tx"
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
	w, err := New(tokens.DefaultSystemID, rpcClient, nil, false, nil, logger.New(t))
	require.NoError(t, err)

	roundNumber, err := w.GetRoundNumber(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 42, roundNumber)
}

func Test_ListTokenTypes(t *testing.T) {
	var firstPubKey *sdktypes.PubKey
	rpcClient := &mockTokensPartitionClient{
		getFungibleTokenTypes: func(ctx context.Context, pubKey sdktypes.PubKey) ([]sdktypes.FungibleTokenType, error) {
			if !bytes.Equal(pubKey, *firstPubKey) {
				return []sdktypes.FungibleTokenType{}, nil
			}

			t1, err := client.NewFungibleTokenType(&client.FungibleTokenTypeParams{})
			require.NoError(t, err)
			t2, err := client.NewFungibleTokenType(&client.FungibleTokenTypeParams{})
			require.NoError(t, err)
			return []sdktypes.FungibleTokenType{t1, t2}, nil
		},
		getNonFungibleTokenTypes: func(ctx context.Context, pubKey sdktypes.PubKey) ([]sdktypes.NonFungibleTokenType, error) {
			if !bytes.Equal(pubKey, *firstPubKey) {
				return []sdktypes.NonFungibleTokenType{}, nil
			}
			params := &client.NonFungibleTokenTypeParams{}
			t1, err := client.NewNonFungibleTokenType(params)
			require.NoError(t, err)
			t2, err := client.NewNonFungibleTokenType(params)
			require.NoError(t, err)
			return []sdktypes.NonFungibleTokenType{t1, t2}, nil
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
		getFungibleTokenTypeHierarchy: func(ctx context.Context, id sdktypes.TokenTypeID) ([]sdktypes.FungibleTokenType, error) {
			tx, found := recTxs[string(id)]
			if found {
				attrs := &tokens.CreateFungibleTokenTypeAttributes{}
				require.NoError(t, tx.UnmarshalAttributes(attrs))
				tokenType, err := client.NewFungibleTokenType(&client.FungibleTokenTypeParams{
					ID:            tx.UnitID(),
					ParentTypeID:  attrs.ParentTypeID,
					DecimalPlaces: attrs.DecimalPlaces,
				})
				require.NoError(t, err)
				return []sdktypes.FungibleTokenType{tokenType}, nil
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
		tt1, err := client.NewFungibleTokenType(&client.FungibleTokenTypeParams{
			ID:                       typeID,
			Symbol:                   "AB",
			Name:                     "Long name for AB",
			Icon:                     &tokens.Icon{Type: "image/png", Data: []byte{1}},
			DecimalPlaces:            0,
			ParentTypeID:             nil,
			SubTypeCreationPredicate: sdktypes.Predicate(templates.AlwaysFalseBytes()),
			TokenCreationPredicate:   sdktypes.Predicate(templates.AlwaysTrueBytes()),
			InvariantPredicate:       sdktypes.Predicate(templates.AlwaysTrueBytes()),
		})
		result, err := tw.NewFungibleType(context.Background(), 1, tt1, nil)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.EqualValues(t, typeID, result.GetUnit())
		tx, found := recTxs[string(typeID)]
		require.True(t, found)
		require.EqualValues(t, tx.Timeout(), 11)

		// new subtype
		tt2, err := client.NewFungibleTokenType(&client.FungibleTokenTypeParams{
			Symbol:                   "AB",
			Name:                     "Long name for AB",
			DecimalPlaces:            2,
			ParentTypeID:             typeID,
			SubTypeCreationPredicate: sdktypes.Predicate(templates.AlwaysFalseBytes()),
			TokenCreationPredicate:   sdktypes.Predicate(templates.AlwaysTrueBytes()),
			InvariantPredicate:       sdktypes.Predicate(templates.AlwaysTrueBytes()),
		})
		require.NoError(t, err)
		_, err = tw.NewFungibleType(context.Background(), 1, tt2, nil)
		//check decimal places are validated against the parent type
		require.ErrorContains(t, err, "parent type requires 0 decimal places, got 2")
	})

	t.Run("non-fungible type", func(t *testing.T) {
		typeID := tokens.NewNonFungibleTokenTypeID(nil, test.RandomBytes(32))
		tt, err := client.NewNonFungibleTokenType(&client.NonFungibleTokenTypeParams{
			ID:                       typeID,
			Symbol:                   "ABNFT",
			Name:                     "Long name for ABNFT",
			Icon:                     &tokens.Icon{Type: "image/svg", Data: []byte{2}},
			ParentTypeID:             nil,
			SubTypeCreationPredicate: sdktypes.Predicate(templates.AlwaysFalseBytes()),
			TokenCreationPredicate:   sdktypes.Predicate(templates.AlwaysTrueBytes()),
			InvariantPredicate:       sdktypes.Predicate(templates.AlwaysTrueBytes()),
		})

		result, err := tw.NewNonFungibleType(context.Background(), 1, tt, nil)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.EqualValues(t, typeID, result.GetUnit())
		tx, found := recTxs[string(typeID)]
		require.True(t, found)
		require.EqualValues(t, tx.Timeout(), 11)
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

			ft, err := client.NewFungibleToken(&client.FungibleTokenParams{
				TypeID: typeID,
				Amount: amount,
				OwnerPredicate: bearerPredicateFromHash(key.PubKeyHash.Sha256),
			})
			require.NoError(t, err)

			result, err := tw.NewFungibleToken(context.Background(), tt.accountNumber, ft, nil)
			require.NoError(t, err)
			tx := recTxs[len(recTxs)-1]
			// TODO: tx construction should be tested in client/token_test.go
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

func newFungibleToken(_ *testing.T, id sdktypes.TokenID, typeID sdktypes.TokenTypeID, symbol string, amount, lockStatus uint64) *mockFungibleToken {
	return &mockFungibleToken{
		mockToken: mockToken{
			id:         id,
			typeID:     typeID,
			symbol:     symbol,
			lockStatus: lockStatus,
		},
		amount: amount,
	}
}

func newNonFungibleToken(t *testing.T, symbol string, ownerPredicate []byte, lockStatus, counter uint64) sdktypes.NonFungibleToken {
	nftID, err := tokens.NewRandomNonFungibleTokenID(nil)
	require.NoError(t, err)
	nftTypeID, err := tokens.NewRandomNonFungibleTokenTypeID(nil)
	require.NoError(t, err)

	return &mockNonFungibleToken{
		mockToken: mockToken{
			id:             nftID,
			typeID:         nftTypeID,
			symbol:         symbol,
			ownerPredicate: ownerPredicate,
			lockStatus:     lockStatus,
			counter:        counter,
		},
	}
}

func TestSendFungible(t *testing.T) {
	recTxs := make([]*types.TransactionOrder, 0)
	typeId := test.RandomBytes(32)
	typeId2 := test.RandomBytes(32)
	typeIdForOverflow := test.RandomBytes(32)
	rpcClient := &mockTokensPartitionClient{
		getFungibleTokens: func(ctx context.Context, ownerID []byte) ([]sdktypes.FungibleToken, error) {
			return []sdktypes.FungibleToken{
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
			result, err := tw.SendFungible(context.Background(), 1, tt.tokenTypeID, tt.targetAmount, nil, nil, defaultProof(key))
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
				require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(key.PubKeyHash.Sha256), tok.Bearer)
			},
		},
		{
			name:          "pub key bearer predicate, account 1, predefined token ID",
			accountNumber: uint64(1),
			validateOwner: func(t *testing.T, accountNumber uint64, tok *tokens.MintNonFungibleTokenAttributes) {
				key, err := tw.am.GetAccountKey(accountNumber - 1)
				require.NoError(t, err)
				require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(key.PubKeyHash.Sha256), tok.Bearer)
			},
		},
		{
			name:          "pub key bearer predicate, account 2",
			accountNumber: uint64(2),
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
			nftParams, err := client.NewNonFungibleToken(&client.NonFungibleTokenParams{
				SystemID:            tokens.DefaultSystemID,
				TypeID:              tokens.NewNonFungibleTokenTypeID(nil, test.RandomBytes(32)),
				OwnerPredicate:      bearerPredicateFromHash(key.PubKeyHash.Sha256),
				URI:                 "https://alphabill.org",
				Data:                nil,
				DataUpdatePredicate: sdktypes.Predicate(templates.AlwaysTrueBytes()),
			})
			result, err := tw.NewNFT(context.Background(), tt.accountNumber, nftParams, nil)
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
	tokenz := make(map[string]sdktypes.NonFungibleToken)
	recTxs := make(map[string]*types.TransactionOrder)
	rpcClient := &mockTokensPartitionClient{
		getNonFungibleToken: func(ctx context.Context, id sdktypes.TokenID) (sdktypes.NonFungibleToken, error) {
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
		token         sdktypes.NonFungibleToken
		key           sdktypes.PubKey
		validateOwner func(t *testing.T, accountNumber uint64, key sdktypes.PubKey, tok *tokens.TransferNonFungibleTokenAttributes)
		wantErr       string
	}{
		{
			name:  "to 'always true' predicate",
			token: newNonFungibleToken(t, "AB", templates.NewP2pkh256BytesFromKey(ak.PubKey), 0, 0),
			key:   nil,
			validateOwner: func(t *testing.T, accountNumber uint64, key sdktypes.PubKey, tok *tokens.TransferNonFungibleTokenAttributes) {
				require.EqualValues(t, templates.AlwaysTrueBytes(), tok.NewBearer)
			},
		},
		{
			name:  "to public key hash predicate",
			token: newNonFungibleToken(t, "AB", templates.NewP2pkh256BytesFromKey(ak.PubKey), 0, 0),
			key:   first(hexutil.Decode("0x0290a43bc454babf1ea8b0b76fcbb01a8f27a989047cf6d6d76397cc4756321e64")),
			validateOwner: func(t *testing.T, accountNumber uint64, key sdktypes.PubKey, tok *tokens.TransferNonFungibleTokenAttributes) {
				require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(key)), tok.NewBearer)
			},
		},
		{
			name:    "locked token is not sent",
			token: newNonFungibleToken(t, "AB", templates.NewP2pkh256BytesFromKey(ak.PubKey), 1, 0),
			wantErr: "token is locked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenz[string(tt.token.ID())] = tt.token
			result, err := tw.TransferNFT(context.Background(), 1, tt.token.ID(), tt.key, nil, defaultProof(ak))
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
	tokenz := make(map[string]sdktypes.NonFungibleToken)
	recTxs := make(map[string]*types.TransactionOrder)
	rpcClient := &mockTokensPartitionClient{
		getNonFungibleToken: func(ctx context.Context, id sdktypes.TokenID) (sdktypes.NonFungibleToken, error) {
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
	tokenz[string(tok.ID())] = tok

	ak, err := tw.am.GetAccountKey(0)
	require.NoError(t, err)

	// test data, counter and predicate inputs are submitted correctly
	data := test.RandomBytes(64)
	result, err := tw.UpdateNFTData(context.Background(), 1, tok.ID(), data, []*PredicateInput{{Argument: nil}, {AccountKey: ak}})
	require.NoError(t, err)
	require.NotNil(t, result)
	tx, found := recTxs[string(tok.ID())]
	require.True(t, found)
	require.EqualValues(t, tok.ID(), tx.UnitID())
	require.Equal(t, tokens.PayloadTypeUpdateNFT, tx.PayloadType())

	// test that locked token tx is not sent
	lockedToken := newNonFungibleToken(t, "AB", nil, 1, 0)
	tokenz[string(tok.ID())] = lockedToken
	result, err = tw.UpdateNFTData(context.Background(), 1, tok.ID(), data, []*PredicateInput{{Argument: nil}, {AccountKey: ak}})
	require.ErrorContains(t, err, "token is locked")
	require.Nil(t, result)
}

func TestLockToken(t *testing.T) {
	var token sdktypes.NonFungibleToken
	recTxs := make(map[string]*types.TransactionOrder)
	rpcClient := &mockTokensPartitionClient{
		getNonFungibleToken: func(ctx context.Context, id sdktypes.TokenID) (sdktypes.NonFungibleToken, error) {
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
	result, err := tw.LockToken(context.Background(), 1, token.ID(), []*PredicateInput{{Argument: nil}}, defaultProof(ak))
	require.ErrorContains(t, err, "token is already locked")
	require.Nil(t, result)

	// test lock token ok
	token = newNonFungibleToken(t, "AB", templates.NewP2pkh256BytesFromKey(ak.PubKey), 0, 0)
	result, err = tw.LockToken(context.Background(), 1, token.ID(), []*PredicateInput{{Argument: nil}}, defaultProof(ak))
	require.NoError(t, err)
	require.NotNil(t, result)
	tx, found := recTxs[string(token.ID())]
	require.True(t, found)
	require.EqualValues(t, token.ID(), tx.UnitID())
	require.Equal(t, tokens.PayloadTypeLockToken, tx.PayloadType())
}

func TestUnlockToken(t *testing.T) {
	var token sdktypes.NonFungibleToken
	recTxs := make(map[string]*types.TransactionOrder)
	rpcClient := &mockTokensPartitionClient{
		getNonFungibleToken: func(ctx context.Context, id sdktypes.TokenID) (sdktypes.NonFungibleToken, error) {
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
	result, err := tw.UnlockToken(context.Background(), 1, token.ID(), []*PredicateInput{{Argument: nil}}, defaultProof(ak))
	require.ErrorContains(t, err, "token is already unlocked")
	require.Nil(t, result)

	// test unlock token ok
	token = newNonFungibleToken(t, "AB", templates.NewP2pkh256BytesFromKey(ak.PubKey), wallet.LockReasonManual, 0)
	result, err = tw.UnlockToken(context.Background(), 1, token.ID(), []*PredicateInput{{Argument: nil}}, defaultProof(ak))
	require.NoError(t, err)
	require.NotNil(t, result)
	tx, found := recTxs[string(token.ID())]
	require.True(t, found)
	require.EqualValues(t, token.ID(), tx.UnitID())
	require.Equal(t, tokens.PayloadTypeUnlockToken, tx.PayloadType())
}

func TestSendFungibleByID(t *testing.T) {
	t.Parallel()

	token := newFungibleToken(t, test.RandomBytes(32), test.RandomBytes(32), "AB", 100, 0)

	be := &mockTokensPartitionClient{
		getFungibleToken: func(ctx context.Context, id sdktypes.TokenID) (sdktypes.FungibleToken, error) {
			if bytes.Equal(id, token.ID()) {
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
	token.ownerPredicate = templates.NewP2pkh256BytesFromKey(pk)

	// Test sending fungible token by ID
	sub, err := w.SendFungibleByID(context.Background(), 1, token.ID(), 50, nil, nil)
	require.NoError(t, err)
	// ensure it's a split
	require.Equal(t, tokens.PayloadTypeSplitFungibleToken, sub.Submissions[0].Transaction.PayloadType())

	sub, err = w.SendFungibleByID(context.Background(), 1, token.ID(), 100, nil, nil)
	require.NoError(t, err)
	// ensure it's a transfer
	require.Equal(t, tokens.PayloadTypeTransferFungibleToken, sub.Submissions[0].Transaction.PayloadType())

	// Test sending fungible token by ID with insufficient balance
	_, err = w.SendFungibleByID(context.Background(), 1, token.ID(), 200, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient FT value")

	// Test sending fungible token by ID with invalid account number
	_, err = w.SendFungibleByID(context.Background(), 0, token.ID(), 50, nil, nil)
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
	getFungibleToken              func(ctx context.Context, id sdktypes.TokenID) (sdktypes.FungibleToken, error)
	getFungibleTokens             func(ctx context.Context, ownerID []byte) ([]sdktypes.FungibleToken, error)
	getFungibleTokenTypes         func(ctx context.Context, creator sdktypes.PubKey) ([]sdktypes.FungibleTokenType, error)
	getFungibleTokenTypeHierarchy func(ctx context.Context, id sdktypes.TokenTypeID) ([]sdktypes.FungibleTokenType, error)

	getNonFungibleToken           func(ctx context.Context, id sdktypes.TokenID) (sdktypes.NonFungibleToken, error)
	getNonFungibleTokens          func(ctx context.Context, ownerID []byte) ([]sdktypes.NonFungibleToken, error)
	getNonFungibleTokenTypes      func(ctx context.Context, creator sdktypes.PubKey) ([]sdktypes.NonFungibleTokenType, error)

	getRoundNumber                func(ctx context.Context) (uint64, error)
	sendTransaction               func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error)
	confirmTransaction            func(ctx context.Context, tx *types.TransactionOrder, log *slog.Logger) (*sdktypes.Proof, error)
	getTransactionProof           func(ctx context.Context, txHash types.Bytes) (*sdktypes.Proof, error)
	getFeeCreditRecordByOwnerID   func(ctx context.Context, ownerID []byte) (sdktypes.FeeCreditRecord, error)
	getBlock                      func(ctx context.Context, roundNumber uint64) (*types.Block, error)
	getUnitsByOwnerID             func(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error)
}

func (m *mockTokensPartitionClient) GetNodeInfo(ctx context.Context) (*sdktypes.NodeInfoResponse, error) {
	return &sdktypes.NodeInfoResponse{
		SystemID: 2,
		Name:     "tokens node",
	}, nil
}

func (m *mockTokensPartitionClient) GetFungibleToken(ctx context.Context, id sdktypes.TokenID) (sdktypes.FungibleToken, error) {
	if m.getFungibleToken != nil {
		return m.getFungibleToken(ctx, id)
	}
	return nil, fmt.Errorf("GetFungibleToken not implemented")
}

func (m *mockTokensPartitionClient) GetFungibleTokens(ctx context.Context, ownerID []byte) ([]sdktypes.FungibleToken, error) {
	if m.getFungibleTokens != nil {
		return m.getFungibleTokens(ctx, ownerID)
	}
	return nil, fmt.Errorf("GetFungibleTokens not implemented")
}

func (m *mockTokensPartitionClient) GetFungibleTokenTypes(ctx context.Context, creator sdktypes.PubKey) ([]sdktypes.FungibleTokenType, error) {
	if m.getFungibleTokenTypes != nil {
		return m.getFungibleTokenTypes(ctx, creator)
	}
	return nil, fmt.Errorf("GetFungibleTokenTypes not implemented")
}

func (m *mockTokensPartitionClient) GetFungibleTokenTypeHierarchy(ctx context.Context, id sdktypes.TokenTypeID) ([]sdktypes.FungibleTokenType, error) {
	if m.getFungibleTokenTypeHierarchy != nil {
		return m.getFungibleTokenTypeHierarchy(ctx, id)
	}
	return nil, fmt.Errorf("GetFungibleTokenTypeHierarchy not implemented")
}

func (m *mockTokensPartitionClient) GetNonFungibleToken(ctx context.Context, id sdktypes.TokenID) (sdktypes.NonFungibleToken, error) {
	if m.getNonFungibleToken != nil {
		return m.getNonFungibleToken(ctx, id)
	}
	return nil, fmt.Errorf("GetNonFungibleToken not implemented")
}

func (m *mockTokensPartitionClient) GetNonFungibleTokens(ctx context.Context, ownerID []byte) ([]sdktypes.NonFungibleToken, error) {
	if m.getNonFungibleTokens != nil {
		return m.getNonFungibleTokens(ctx, ownerID)
	}
	return nil, fmt.Errorf("GetNonFungibleTokens not implemented")
}

func (m *mockTokensPartitionClient) GetNonFungibleTokenTypes(ctx context.Context, creator sdktypes.PubKey) ([]sdktypes.NonFungibleTokenType, error) {
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

func (m *mockTokensPartitionClient) GetFeeCreditRecordByOwnerID(ctx context.Context, ownerID []byte) (sdktypes.FeeCreditRecord, error) {
	if m.getFeeCreditRecordByOwnerID != nil {
		return m.getFeeCreditRecordByOwnerID(ctx, ownerID)
	}
	c := uint64(2)
	id := tokens.NewFeeCreditRecordID(nil, []byte{1})
	return client.NewFeeCreditRecord(tokens.DefaultSystemID, id, 100000, 0, &c), nil
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

type mockToken struct {
	id             sdktypes.TokenID
	symbol         string
	typeID         sdktypes.TokenTypeID
	typeName       string
	ownerPredicate []byte
	nonce          []byte
	counter        uint64
	lockStatus     uint64
}

type mockFungibleToken struct {
	mockToken
	amount        uint64
	decimalPlaces uint32
	burned        bool
}

type mockNonFungibleToken struct {
	mockToken
	name                string
	uri                 string
	data                []byte
	dataUpdatePredicate sdktypes.Predicate
}

func (m *mockToken) SystemID() types.SystemID {
	return tokens.DefaultSystemID
}

func (m *mockToken) ID() sdktypes.TokenID {
	return m.id
}

func (m *mockToken) TypeID() sdktypes.TokenTypeID {
	return m.typeID
}

func (m *mockToken) TypeName() string {
	return m.typeName
}

func (m *mockToken) Symbol() string {
	return m.symbol
}

func (m *mockToken) OwnerPredicate() []byte {
	return m.ownerPredicate
}

func (m *mockToken) Nonce() []byte {
	return m.nonce
}

func (m *mockToken) LockStatus() uint64 {
	return m.lockStatus
}

func (m *mockToken) Counter() uint64 {
	return m.counter
}

func (m *mockToken) IncreaseCounter() {
	m.counter += 1
}

func (m *mockToken) Lock(lockStatus uint64, txOptions ...tx.Option) (*types.TransactionOrder, error) {
	return newMockTx(m.id, tokens.PayloadTypeLockToken), nil
}

func (m *mockToken) Unlock(txOptions ...tx.Option) (*types.TransactionOrder, error) {
	return newMockTx(m.id, tokens.PayloadTypeUnlockToken), nil
}

func (m *mockFungibleToken) Amount() uint64 {
	return m.amount
}

func (m *mockFungibleToken) DecimalPlaces() uint32 {
	return m.decimalPlaces
}

func (m *mockFungibleToken) Burned() bool {
	return m.burned
}

func (m *mockFungibleToken) Create(txOptions ...tx.Option) (*types.TransactionOrder, error) {
	return newMockTx(m.id, tokens.PayloadTypeMintFungibleToken), nil
}

func (m *mockFungibleToken) Transfer(ownerPredicate []byte, txOptions ...tx.Option) (*types.TransactionOrder, error) {
	tx := newMockTx(m.id, tokens.PayloadTypeTransferFungibleToken)
	tx.Payload.SetAttributes(&tokens.TransferFungibleTokenAttributes{
		Value: m.amount,
	})
	return tx, nil
}

func (m *mockFungibleToken) Split(amount uint64, ownerPredicate []byte, txOptions ...tx.Option) (*types.TransactionOrder, error) {
	tx := newMockTx(m.id, tokens.PayloadTypeSplitFungibleToken)
	tx.Payload.SetAttributes(&tokens.SplitFungibleTokenAttributes{
		TargetValue: amount,
	})
	return tx, nil

}

func (m *mockFungibleToken) Burn(targetTokenID types.UnitID, targetTokenCounter uint64, txOptions ...tx.Option) (*types.TransactionOrder, error) {
	return newMockTx(m.id, tokens.PayloadTypeBurnFungibleToken), nil
}

func (m *mockFungibleToken) Join(burnTxs []*types.TransactionRecord, burnProofs []*types.TxProof, txOptions ...tx.Option) (*types.TransactionOrder, error) {
	return newMockTx(m.id, tokens.PayloadTypeJoinFungibleToken), nil
}

func (m *mockNonFungibleToken) Name() string {
	return m.name
}

func (m *mockNonFungibleToken) URI() string {
	return m.uri
}

func (m *mockNonFungibleToken) Data() []byte {
	return m.data
}

func (m *mockNonFungibleToken) DataUpdatePredicate() sdktypes.Predicate {
	return m.dataUpdatePredicate
}

func (m *mockNonFungibleToken) Create(txOptions ...tx.Option) (*types.TransactionOrder, error) {
	return newMockTx(m.id, tokens.PayloadTypeMintNFT), nil
}

func (m *mockNonFungibleToken) Transfer(ownerPredicate []byte, txOptions ...tx.Option) (*types.TransactionOrder, error) {
	return newMockTx(m.id, tokens.PayloadTypeTransferNFT), nil
}

func (m *mockNonFungibleToken) Update(data []byte, txOptions ...tx.Option) (*types.TransactionOrder, error) {
	return newMockTx(m.id, tokens.PayloadTypeUpdateNFT), nil
}

func newMockTx(id types.UnitID, payloadType string) *types.TransactionOrder {
	return &types.TransactionOrder{
		Payload: &types.Payload{
			Type:   payloadType,
			UnitID: id,
		},
	}
}
