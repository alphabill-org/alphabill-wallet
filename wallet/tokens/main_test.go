package tokens

import (
	"bytes"
	"context"
	"crypto"
	"fmt"
	"math"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	test "github.com/alphabill-org/alphabill-wallet/internal/testutils"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/logger"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/api"
)

func Test_GetRoundNumber_OK(t *testing.T) {
	t.Parallel()

	observe := observability.NewFactory(t)
	rpcClient := &mockTokensRpcClient{
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 42, nil
		},
	}
	w, err := New(tokens.DefaultSystemID, rpcClient, nil, false, nil, observe.DefaultLogger())
	require.NoError(t, err)

	roundNumber, err := w.GetRoundNumber(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 42, roundNumber)
}

func Test_GetFeeCreditBill_OK(t *testing.T) {
	t.Parallel()

	observe := observability.NewFactory(t)
	expectedFCB := &api.FeeCreditBill{ID: []byte{1}, FeeCreditRecord: &fc.FeeCreditRecord{Balance: 100}}
	rpcClient := &mockTokensRpcClient{}
	w, err := New(tokens.DefaultSystemID, rpcClient, nil, false, nil, observe.DefaultLogger())
	require.NoError(t, err)

	// verify that correct fee credit bill is returned
	rpcClient.getFeeCreditRecord = func(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.FeeCreditBill, error) {
		return expectedFCB, nil
	}
	actualFCB, err := w.GetFeeCreditBill(context.Background(), []byte{1})
	require.NoError(t, err)
	require.Equal(t, expectedFCB, actualFCB)

	// verify that no error is returned when api returns not found error
	rpcClient.getFeeCreditRecord = func(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.FeeCreditBill, error) {
		return nil, api.ErrNotFound
	}
	actualFCB, err = w.GetFeeCreditBill(context.Background(), []byte{1})
	require.NoError(t, err)
	require.Nil(t, actualFCB)
}

func Test_ListTokens(t *testing.T) {
	rpcClient := &mockTokensRpcClient{
		getTokens: func(ctx context.Context, kind Kind, ownerID []byte) ([]*TokenUnit, error) {
			fungible := []*TokenUnit{
				{
					ID:   test.RandomBytes(32),
					Kind: Fungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: Fungible,
				},
			}
			nfts := []*TokenUnit{
				{
					ID:   test.RandomBytes(32),
					Kind: NonFungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: NonFungible,
				},
			}
			switch kind {
			case Fungible:
				return fungible, nil
			case NonFungible:
				return nfts, nil
			case Any:
				return append(fungible, nfts...), nil
			}
			return nil, fmt.Errorf("invalid kind")
		},
	}
	tw := initTestWallet(t, rpcClient)
	tokenz, err := tw.ListTokens(context.Background(), Any, AllAccounts)
	require.NoError(t, err)
	require.Len(t, tokenz[1], 4)

	tokenz, err = tw.ListTokens(context.Background(), Fungible, AllAccounts)
	require.NoError(t, err)
	require.Len(t, tokenz[1], 2)

	tokenz, err = tw.ListTokens(context.Background(), NonFungible, AllAccounts)
	require.NoError(t, err)
	require.Len(t, tokenz[1], 2)
}

func Test_ListTokenTypes(t *testing.T) {
	var firstPubKey *wallet.PubKey
	rpcClient := &mockTokensRpcClient{
		getTokenTypes: func(ctx context.Context, kind Kind, pubKey wallet.PubKey) ([]*TokenUnitType, error) {
			if !bytes.Equal(pubKey, *firstPubKey) {
				return []*TokenUnitType{}, nil
			}

			fungible := []*TokenUnitType{
				{
					ID:   test.RandomBytes(32),
					Kind: Fungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: Fungible,
				},
			}
			nfts := []*TokenUnitType{
				{
					ID:   test.RandomBytes(32),
					Kind: NonFungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: NonFungible,
				},
			}
			switch kind {
			case Fungible:
				return fungible, nil
			case NonFungible:
				return nfts, nil
			case Any:
				return append(fungible, nfts...), nil
			}
			return nil, fmt.Errorf("invalid kind")
		},
	}

	tw := initTestWallet(t, rpcClient)
	key, err := tw.GetAccountManager().GetPublicKey(0)
	require.NoError(t, err)
	firstPubKey = (*wallet.PubKey)(&key)

	typez, err := tw.ListTokenTypes(context.Background(), 0, Any)
	require.NoError(t, err)
	require.Len(t, typez, 4)

	typez, err = tw.ListTokenTypes(context.Background(), 0, Fungible)
	require.NoError(t, err)
	require.Len(t, typez, 2)

	typez, err = tw.ListTokenTypes(context.Background(), 0, NonFungible)
	require.NoError(t, err)
	require.Len(t, typez, 2)

	_, err = tw.ListTokenTypes(context.Background(), 2, NonFungible)
	require.ErrorContains(t, err, "account does not exist")

	_, _, err = tw.am.AddAccount()
	require.NoError(t, err)

	typez, err = tw.ListTokenTypes(context.Background(), 2, Any)
	require.NoError(t, err)
	require.Len(t, typez, 0)
}

func TestNewTypes(t *testing.T) {
	t.Parallel()

	recTxs := make(map[string]*types.TransactionOrder)
	rpcClient := &mockTokensRpcClient{
		getTypeHierarchy: func(ctx context.Context, id TokenTypeID) ([]*TokenUnitType, error) {
			tx, found := recTxs[string(id)]
			if found {
				tokenType := &TokenUnitType{ID: tx.UnitID()}
				if tx.PayloadType() == tokens.PayloadTypeCreateFungibleTokenType {
					tokenType.Kind = Fungible
					attrs := &tokens.CreateFungibleTokenTypeAttributes{}
					require.NoError(t, tx.UnmarshalAttributes(attrs))
					tokenType.ParentTypeID = attrs.ParentTypeID
					tokenType.DecimalPlaces = attrs.DecimalPlaces
				} else {
					tokenType.Kind = NonFungible
					attrs := &tokens.CreateNonFungibleTokenTypeAttributes{}
					require.NoError(t, tx.UnmarshalAttributes(attrs))
					tokenType.ParentTypeID = attrs.ParentTypeID
				}
				return []*TokenUnitType{tokenType}, nil
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
		getFeeCreditRecord: func(ctx context.Context, unitID types.UnitID, includeProof bool) (*api.FeeCreditBill, error) {
			return &api.FeeCreditBill{
				ID: []byte{1},
				FeeCreditRecord: &fc.FeeCreditRecord{
					Balance:  100000,
					Backlink: []byte{2},
				},
			}, nil
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
			ParentTypeId:             nil,
			SubTypeCreationPredicate: wallet.Predicate(templates.AlwaysFalseBytes()),
			TokenCreationPredicate:   wallet.Predicate(templates.AlwaysTrueBytes()),
			InvariantPredicate:       wallet.Predicate(templates.AlwaysTrueBytes()),
		}
		result, err := tw.NewFungibleType(context.Background(), 1, a, typeID, nil)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.EqualValues(t, typeID, result.TokenTypeID)
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
			ParentTypeId:             typeID,
			SubTypeCreationPredicate: wallet.Predicate(templates.AlwaysFalseBytes()),
			TokenCreationPredicate:   wallet.Predicate(templates.AlwaysTrueBytes()),
			InvariantPredicate:       wallet.Predicate(templates.AlwaysTrueBytes()),
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
		require.True(t, result.TokenTypeID.HasType(tokens.FungibleTokenTypeUnitType))
	})

	t.Run("non-fungible type", func(t *testing.T) {
		typeId := tokens.NewNonFungibleTokenTypeID(nil, test.RandomBytes(32))
		a := CreateNonFungibleTokenTypeAttributes{
			Symbol:                   "ABNFT",
			Name:                     "Long name for ABNFT",
			Icon:                     &Icon{Type: "image/svg", Data: []byte{2}},
			ParentTypeId:             nil,
			SubTypeCreationPredicate: wallet.Predicate(templates.AlwaysFalseBytes()),
			TokenCreationPredicate:   wallet.Predicate(templates.AlwaysTrueBytes()),
			InvariantPredicate:       wallet.Predicate(templates.AlwaysTrueBytes()),
		}
		result, err := tw.NewNonFungibleType(context.Background(), 1, a, typeId, nil)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.EqualValues(t, typeId, result.TokenTypeID)
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
		require.True(t, result.TokenTypeID.HasType(tokens.NonFungibleTokenTypeUnitType))
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
		getFeeCreditRecord: func(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.FeeCreditBill, error) {
			return &api.FeeCreditBill{
				ID:              []byte{1},
				FeeCreditRecord: &fc.FeeCreditRecord{Balance: 100000, Backlink: []byte{2}},
			}, nil
		},
	}
	tw := initTestWallet(t, rpcClient)
	_, _, err := tw.am.AddAccount()
	require.NoError(t, err)

	tests := []struct {
		name  string
		accNr uint64
	}{
		{
			name:  "pub key bearer predicate, account 1",
			accNr: uint64(1),
		},
		{
			name:  "pub key bearer predicate, account 2",
			accNr: uint64(2),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typeID := test.RandomBytes(33)
			amount := uint64(100)
			key, err := tw.am.GetAccountKey(tt.accNr - 1)
			require.NoError(t, err)
			result, err := tw.NewFungibleToken(context.Background(), tt.accNr, typeID, amount, bearerPredicateFromHash(key.PubKeyHash.Sha256), nil)
			require.NoError(t, err)
			tx := recTxs[len(recTxs)-1]
			newToken := &tokens.MintFungibleTokenAttributes{}
			require.NotNil(t, result)
			require.EqualValues(t, tx.UnitID(), result.TokenTypeID)
			require.Nil(t, result.TokenID)
			require.NoError(t, tx.UnmarshalAttributes(newToken))
			require.NotEqual(t, []byte{0}, tx.UnitID())
			require.Len(t, tx.UnitID(), 33)
			require.Equal(t, amount, newToken.Value)
			require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(key.PubKeyHash.Sha256), newToken.Bearer)
		})
	}
}

func TestSendFungible(t *testing.T) {
	recTxs := make([]*types.TransactionOrder, 0)
	typeId := test.RandomBytes(32)
	typeId2 := test.RandomBytes(32)
	typeIdForOverflow := test.RandomBytes(32)
	rpcClient := &mockTokensRpcClient{
		getTokens: func(ctx context.Context, kind Kind, ownerID []byte) ([]*TokenUnit, error) {
			return []*TokenUnit{
				{ID: test.RandomBytes(32), Kind: Fungible, Symbol: "AB", TypeID: typeId, Amount: 3},
				{ID: test.RandomBytes(32), Kind: Fungible, Symbol: "AB", TypeID: typeId, Amount: 5},
				{ID: test.RandomBytes(32), Kind: Fungible, Symbol: "AB", TypeID: typeId, Amount: 7},
				{ID: test.RandomBytes(32), Kind: Fungible, Symbol: "AB", TypeID: typeId, Amount: 18},
				{ID: test.RandomBytes(32), Kind: Fungible, Symbol: "AB2", TypeID: typeIdForOverflow, Amount: math.MaxUint64},
				{ID: test.RandomBytes(32), Kind: Fungible, Symbol: "AB2", TypeID: typeIdForOverflow, Amount: 1},
				{ID: test.RandomBytes(32), Kind: Fungible, Symbol: "AB3", TypeID: typeId2, Amount: 1, Locked: 1},
			}, nil
		},
		getFeeCreditRecord: func(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.FeeCreditBill, error) {
			return &api.FeeCreditBill{
				ID:              []byte{1},
				FeeCreditRecord: &fc.FeeCreditRecord{Balance: 100000, Backlink: []byte{2}},
			}, nil
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
		tokenTypeID        TokenTypeID
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
			expectedErrorMsg: fmt.Sprintf("insufficient tokens of type %s: got 33, need 60", TokenTypeID(typeId)),
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
			name:         "total balance uint64 overflow, transfer is submitted with maxuint64",
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
			expectedErrorMsg: fmt.Sprintf("insufficient tokens of type %s: got 0, need 1", TokenTypeID(typeId2)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recTxs = make([]*types.TransactionOrder, 0)
			result, err := tw.SendFungible(context.Background(), 1, tt.tokenTypeID, tt.targetAmount, nil, nil)
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
	tokenID := test.RandomBytes(32)
	accNr := uint64(1)
	tests := []struct {
		name       string
		attrs      MintNonFungibleTokenAttributes
		wantErrStr string
	}{
		{
			name: "invalid name",
			attrs: MintNonFungibleTokenAttributes{
				Name: fmt.Sprintf("%x", test.RandomBytes(129))[:257],
			},
			wantErrStr: "name exceeds the maximum allowed size of 256 bytes",
		},
		{
			name: "invalid URI",
			attrs: MintNonFungibleTokenAttributes{
				Uri: "invalid_uri",
			},
			wantErrStr: "URI 'invalid_uri' is invalid",
		},
		{
			name: "URI exceeds maximum allowed length",
			attrs: MintNonFungibleTokenAttributes{
				Uri: string(test.RandomBytes(4097)),
			},
			wantErrStr: "URI exceeds the maximum allowed size of 4096 bytes",
		},
		{
			name: "data exceeds maximum allowed length",
			attrs: MintNonFungibleTokenAttributes{
				Data: test.RandomBytes(65537),
			},
			wantErrStr: "data exceeds the maximum allowed size of 65536 bytes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Wallet{log: logger.New(t)}
			got, err := w.NewNFT(context.Background(), accNr, tt.attrs, tokenID, nil)
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
		getFeeCreditRecord: func(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.FeeCreditBill, error) {
			return &api.FeeCreditBill{
				ID:              []byte{1},
				FeeCreditRecord: &fc.FeeCreditRecord{Balance: 100000, Backlink: []byte{2}},
			}, nil
		},
	}
	tw := initTestWallet(t, rpcClient)
	_, _, err := tw.am.AddAccount()
	require.NoError(t, err)

	tests := []struct {
		name          string
		accNr         uint64
		typeID        TokenTypeID
		validateOwner func(t *testing.T, accNr uint64, tok *tokens.MintNonFungibleTokenAttributes)
	}{
		{
			name:   "pub key bearer predicate, account 1",
			accNr:  uint64(1),
			typeID: tokens.NewNonFungibleTokenTypeID(nil, test.RandomBytes(32)),
			validateOwner: func(t *testing.T, accNr uint64, tok *tokens.MintNonFungibleTokenAttributes) {
				key, err := tw.am.GetAccountKey(accNr - 1)
				require.NoError(t, err)
				require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(key.PubKeyHash.Sha256), tok.Bearer)
			},
		},
		{
			name:   "pub key bearer predicate, account 1, predefined token ID",
			accNr:  uint64(1),
			typeID: tokens.NewNonFungibleTokenTypeID(nil, test.RandomBytes(32)),
			validateOwner: func(t *testing.T, accNr uint64, tok *tokens.MintNonFungibleTokenAttributes) {
				key, err := tw.am.GetAccountKey(accNr - 1)
				require.NoError(t, err)
				require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(key.PubKeyHash.Sha256), tok.Bearer)
			},
		},
		{
			name:   "pub key bearer predicate, account 2",
			accNr:  uint64(2),
			typeID: tokens.NewNonFungibleTokenTypeID(nil, test.RandomBytes(32)),
			validateOwner: func(t *testing.T, accNr uint64, tok *tokens.MintNonFungibleTokenAttributes) {
				key, err := tw.am.GetAccountKey(accNr - 1)
				require.NoError(t, err)
				require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(key.PubKeyHash.Sha256), tok.Bearer)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := tw.am.GetAccountKey(tt.accNr - 1)
			require.NoError(t, err)
			a := MintNonFungibleTokenAttributes{
				Bearer:              bearerPredicateFromHash(key.PubKeyHash.Sha256),
				Uri:                 "https://alphabill.org",
				Data:                nil,
				DataUpdatePredicate: wallet.Predicate(templates.AlwaysTrueBytes()),
			}
			result, err := tw.NewNFT(context.Background(), tt.accNr, a, tt.typeID, nil)
			require.NoError(t, err)
			tx := recTxs[len(recTxs)-1]
			newToken := &tokens.MintNonFungibleTokenAttributes{}
			require.NotNil(t, result)
			require.Len(t, tx.UnitID(), 33)
			require.Equal(t, tx.UnitID(), result.TokenTypeID)
			require.Equal(t, tx.UnitID(), tt.typeID)
			require.NoError(t, tx.UnmarshalAttributes(newToken))
			tt.validateOwner(t, tt.accNr, newToken)
		})
	}
}

func TestTransferNFT(t *testing.T) {
	tokenz := make(map[string]*TokenUnit)
	recTxs := make(map[string]*types.TransactionOrder)
	rpcClient := &mockTokensRpcClient{
		getToken: func(ctx context.Context, id TokenID) (*TokenUnit, error) {
			return tokenz[string(id)], nil
		},
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			recTxs[string(tx.UnitID())] = tx
			return tx.Hash(crypto.SHA256), nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditRecord: func(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.FeeCreditBill, error) {
			return &api.FeeCreditBill{
				ID:              []byte{1},
				FeeCreditRecord: &fc.FeeCreditRecord{Balance: 100000, Backlink: []byte{2}},
			}, nil
		},
	}
	tw := initTestWallet(t, rpcClient)

	first := func(s wallet.PubKey, e error) wallet.PubKey {
		require.NoError(t, e)
		return s
	}
	tests := []struct {
		name          string
		token         *TokenUnit
		key           wallet.PubKey
		validateOwner func(t *testing.T, accNr uint64, key wallet.PubKey, tok *tokens.TransferNonFungibleTokenAttributes)
		wantErr       string
	}{
		{
			name:  "to 'always true' predicate",
			token: &TokenUnit{ID: test.RandomBytes(32), Kind: NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32)},
			key:   nil,
			validateOwner: func(t *testing.T, accNr uint64, key wallet.PubKey, tok *tokens.TransferNonFungibleTokenAttributes) {
				require.EqualValues(t, templates.AlwaysTrueBytes(), tok.NewBearer)
			},
		},
		{
			name:  "to public key hash predicate",
			token: &TokenUnit{ID: test.RandomBytes(32), Kind: NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32)},
			key:   first(hexutil.Decode("0x0290a43bc454babf1ea8b0b76fcbb01a8f27a989047cf6d6d76397cc4756321e64")),
			validateOwner: func(t *testing.T, accNr uint64, key wallet.PubKey, tok *tokens.TransferNonFungibleTokenAttributes) {
				require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(key)), tok.NewBearer)
			},
		},
		{
			name:    "locked token is not sent",
			token:   &TokenUnit{ID: test.RandomBytes(32), Kind: NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32), Locked: 1},
			wantErr: "token is locked",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenz[string(tt.token.ID)] = tt.token
			result, err := tw.TransferNFT(context.Background(), 1, tt.token.ID, tt.key, nil)
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
	tokenz := make(map[string]*TokenUnit)
	recTxs := make(map[string]*types.TransactionOrder)
	rpcClient := &mockTokensRpcClient{
		getToken: func(ctx context.Context, id TokenID) (*TokenUnit, error) {
			return tokenz[string(id)], nil
		},
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			recTxs[string(tx.UnitID())] = tx
			return tx.Hash(crypto.SHA256), nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditRecord: func(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.FeeCreditBill, error) {
			return &api.FeeCreditBill{
				ID:              []byte{1},
				FeeCreditRecord: &fc.FeeCreditRecord{Balance: 100000, Backlink: []byte{2}},
			}, nil
		},
	}
	tw := initTestWallet(t, rpcClient)

	parseNFTDataUpdate := func(t *testing.T, tx *types.TransactionOrder) *tokens.UpdateNonFungibleTokenAttributes {
		t.Helper()
		newTransfer := &tokens.UpdateNonFungibleTokenAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(newTransfer))
		return newTransfer
	}

	tok := &TokenUnit{ID: test.RandomBytes(32), Kind: NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32), Counter: 0}
	tokenz[string(tok.ID)] = tok

	// test data, backlink and predicate inputs are submitted correctly
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
	lockedToken := &TokenUnit{ID: test.RandomBytes(32), Kind: NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32), Counter: 0, Locked: 1}
	tokenz[string(tok.ID)] = lockedToken
	result, err = tw.UpdateNFTData(context.Background(), 1, tok.ID, data2, []*PredicateInput{{Argument: nil}, {AccountNumber: 1}})
	require.ErrorContains(t, err, "token is locked")
	require.Nil(t, result)
}

func TestLockToken(t *testing.T) {
	var token *TokenUnit
	recTxs := make(map[string]*types.TransactionOrder)
	rpcClient := &mockTokensRpcClient{
		getToken: func(ctx context.Context, id TokenID) (*TokenUnit, error) {
			return token, nil
		},
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			recTxs[string(tx.UnitID())] = tx
			return tx.Hash(crypto.SHA256), nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditRecord: func(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.FeeCreditBill, error) {
			return &api.FeeCreditBill{
				ID:              []byte{1},
				FeeCreditRecord: &fc.FeeCreditRecord{Balance: 100000, Backlink: []byte{2}},
			}, nil
		},
	}
	tw := initTestWallet(t, rpcClient)

	// test token is already locked
	token = &TokenUnit{ID: test.RandomBytes(32), Kind: NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32), Locked: wallet.LockReasonManual}
	result, err := tw.LockToken(context.Background(), 1, token.ID, []*PredicateInput{{Argument: nil}})
	require.ErrorContains(t, err, "token is already locked")
	require.Nil(t, result)

	// test lock token ok
	token = &TokenUnit{ID: test.RandomBytes(32), Kind: NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32)}
	result, err = tw.LockToken(context.Background(), 1, token.ID, []*PredicateInput{{Argument: nil}})
	require.NoError(t, err)
	require.NotNil(t, result)
	tx, found := recTxs[string(token.ID)]
	require.True(t, found)
	require.EqualValues(t, token.ID, tx.UnitID())
	require.Equal(t, tokens.PayloadTypeLockToken, tx.PayloadType())
}

func TestUnlockToken(t *testing.T) {
	var token *TokenUnit
	recTxs := make(map[string]*types.TransactionOrder)
	rpcClient := &mockTokensRpcClient{
		getToken: func(ctx context.Context, id TokenID) (*TokenUnit, error) {
			return token, nil
		},
		sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
			recTxs[string(tx.UnitID())] = tx
			return tx.Hash(crypto.SHA256), nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditRecord: func(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.FeeCreditBill, error) {
			return &api.FeeCreditBill{
				ID:              []byte{1},
				FeeCreditRecord: &fc.FeeCreditRecord{Balance: 100000, Backlink: []byte{2}},
			}, nil
		},
	}
	tw := initTestWallet(t, rpcClient)

	// test token is already unlocked
	token = &TokenUnit{ID: test.RandomBytes(32), Kind: NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32)}
	result, err := tw.UnlockToken(context.Background(), 1, token.ID, []*PredicateInput{{Argument: nil}})
	require.ErrorContains(t, err, "token is already unlocked")
	require.Nil(t, result)

	// test unlock token ok
	token = &TokenUnit{ID: test.RandomBytes(32), Kind: NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32), Locked: wallet.LockReasonManual}
	result, err = tw.UnlockToken(context.Background(), 1, token.ID, []*PredicateInput{{Argument: nil}})
	require.NoError(t, err)
	require.NotNil(t, result)
	tx, found := recTxs[string(token.ID)]
	require.True(t, found)
	require.EqualValues(t, token.ID, tx.UnitID())
	require.Equal(t, tokens.PayloadTypeUnlockToken, tx.PayloadType())
}

func initTestWallet(t *testing.T, rpcClient RpcClient) *Wallet {
	t.Helper()
	return &Wallet{
		am:        initAccountManager(t),
		rpcClient: rpcClient,
		log:       logger.New(t),
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
	getToken            func(ctx context.Context, id TokenID) (*TokenUnit, error)
	getTokens           func(ctx context.Context, kind Kind, ownerID []byte) ([]*TokenUnit, error)
	getTokenTypes       func(ctx context.Context, kind Kind, creator wallet.PubKey) ([]*TokenUnitType, error)
	getTypeHierarchy    func(ctx context.Context, id TokenTypeID) ([]*TokenUnitType, error)
	getRoundNumber      func(ctx context.Context) (uint64, error)
	sendTransaction     func(ctx context.Context, tx *types.TransactionOrder) ([]byte, error)
	getTransactionProof func(ctx context.Context, txHash types.Bytes) (*types.TransactionRecord, *types.TxProof, error)
	getFeeCreditRecord  func(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.FeeCreditBill, error)
	getBlock            func(ctx context.Context, roundNumber uint64) (*types.Block, error)
	getUnitsByOwnerID   func(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error)
}

func (m *mockTokensRpcClient) GetToken(ctx context.Context, id TokenID) (*TokenUnit, error) {
	if m.getToken != nil {
		return m.getToken(ctx, id)
	}
	return nil, fmt.Errorf("GetToken not implemented")
}

func (m *mockTokensRpcClient) GetTokens(ctx context.Context, kind Kind, ownerID []byte) ([]*TokenUnit, error) {
	if m.getTokens != nil {
		return m.getTokens(ctx, kind, ownerID)
	}
	return nil, fmt.Errorf("GetTokens not implemented")
}

func (m *mockTokensRpcClient) GetTokenTypes(ctx context.Context, kind Kind, creator wallet.PubKey) ([]*TokenUnitType, error) {
	if m.getTokenTypes != nil {
		return m.getTokenTypes(ctx, kind, creator)
	}
	return nil, fmt.Errorf("GetTokenTypes not implemented")
}

func (m *mockTokensRpcClient) GetTypeHierarchy(ctx context.Context, id TokenTypeID) ([]*TokenUnitType, error) {
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

func (m *mockTokensRpcClient) GetTransactionProof(ctx context.Context, txHash types.Bytes) (*types.TransactionRecord, *types.TxProof, error) {
	if m.getTransactionProof != nil {
		return m.getTransactionProof(ctx, txHash)
	}
	return nil, nil, fmt.Errorf("GetTxProof not implemented")
}

func (m *mockTokensRpcClient) GetFeeCreditRecord(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.FeeCreditBill, error) {
	if m.getFeeCreditRecord != nil {
		return m.getFeeCreditRecord(ctx, unitID, includeStateProof)
	}
	return nil, fmt.Errorf("GetFeeCreditRecord not implemented")
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
