package tokens

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	test "github.com/alphabill-org/alphabill-wallet/internal/testutils"
)

func TestGetTokensForDC(t *testing.T) {
	typeID1 := test.RandomBytes(32)
	typeID2 := test.RandomBytes(32)
	typeID3 := test.RandomBytes(32)
	typeID4 := test.RandomBytes(32)

	allTokens := []*TokenUnit{
		{ID: test.RandomBytes(32), Kind: Fungible, Symbol: "AB1", TypeID: typeID1, Amount: 100},
		{ID: test.RandomBytes(32), Kind: Fungible, Symbol: "AB1", TypeID: typeID1, Amount: 100},
		{ID: test.RandomBytes(32), Kind: Fungible, Symbol: "AB2", TypeID: typeID2, Amount: 100},
		{ID: test.RandomBytes(32), Kind: Fungible, Symbol: "AB2", TypeID: typeID2, Amount: 100},
		{ID: test.RandomBytes(32), Kind: NonFungible, Symbol: "AB3", TypeID: typeID3},
		{ID: test.RandomBytes(32), Kind: Fungible, Symbol: "AB4", TypeID: typeID4, Locked: 1},
	}

	be := &mockTokensRpcClient{
		getTokens: func(_ context.Context, kind Kind, owner []byte) ([]*TokenUnit, error) {
			require.Equal(t, Fungible, kind)
			var res []*TokenUnit
			for _, tok := range allTokens {
				if tok.Kind != kind {
					continue
				}
				res = append(res, tok)
			}
			return res, nil
		},
	}
	tw := initTestWallet(t, be)
	key, err := tw.GetAccountManager().GetPublicKey(0)
	require.NoError(t, err)

	tests := []struct {
		allowedTypes []TokenTypeID
		expected     map[string][]*TokenUnit
	}{
		{
			allowedTypes: nil,
			expected:     map[string][]*TokenUnit{string(typeID1): allTokens[:2], string(typeID2): allTokens[2:4]},
		},
		{
			allowedTypes: make([]TokenTypeID, 0),
			expected:     map[string][]*TokenUnit{string(typeID1): allTokens[:2], string(typeID2): allTokens[2:4]},
		},
		{
			allowedTypes: []TokenTypeID{test.RandomBytes(32)},
			expected:     map[string][]*TokenUnit{},
		},
		{
			allowedTypes: []TokenTypeID{typeID3},
			expected:     map[string][]*TokenUnit{},
		},
		{
			allowedTypes: []TokenTypeID{typeID1},
			expected:     map[string][]*TokenUnit{string(typeID1): allTokens[:2]},
		},
		{
			allowedTypes: []TokenTypeID{typeID2},
			expected:     map[string][]*TokenUnit{string(typeID2): allTokens[2:4]},
		},
		{
			allowedTypes: []TokenTypeID{typeID1, typeID2},
			expected:     map[string][]*TokenUnit{string(typeID1): allTokens[:2], string(typeID2): allTokens[2:4]},
		},
		{
			allowedTypes: []TokenTypeID{typeID4},
			expected:     map[string][]*TokenUnit{},
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v", tt.allowedTypes), func(t *testing.T) {
			tokens, err := tw.getTokensForDC(context.Background(), key, tt.allowedTypes)
			require.NoError(t, err)
			require.EqualValues(t, tt.expected, tokens)
		})
	}
}
