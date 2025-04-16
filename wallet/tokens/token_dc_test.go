package tokens

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils"
)

func TestGetTokensForDC(t *testing.T) {
	typeID1 := testutils.RandomBytes(32)
	typeID2 := testutils.RandomBytes(32)
	typeID3 := testutils.RandomBytes(32)
	typeID4 := testutils.RandomBytes(32)

	allTokens := []*types.FungibleToken{
		newFungibleToken(t, testutils.RandomBytes(32), typeID1, "AB1", 100, nil),
		newFungibleToken(t, testutils.RandomBytes(32), typeID1, "AB1", 100, nil),
		newFungibleToken(t, testutils.RandomBytes(32), typeID2, "AB2", 100, nil),
		newFungibleToken(t, testutils.RandomBytes(32), typeID2, "AB2", 100, nil),
		newFungibleToken(t, testutils.RandomBytes(32), typeID4, "AB4", 0, []byte{1}),
	}

	be := &mockTokensPartitionClient{
		getFungibleTokens: func(_ context.Context, owner []byte) ([]*types.FungibleToken, error) {
			return allTokens, nil
		},
	}
	tw := initTestWallet(t, be)
	key, err := tw.GetAccountManager().GetPublicKey(0)
	require.NoError(t, err)

	tests := []struct {
		allowedTypes []types.TokenTypeID
		expected     map[string][]*types.FungibleToken
	}{
		{
			allowedTypes: nil,
			expected:     map[string][]*types.FungibleToken{string(typeID1): allTokens[:2], string(typeID2): allTokens[2:4]},
		},
		{
			allowedTypes: make([]types.TokenTypeID, 0),
			expected:     map[string][]*types.FungibleToken{string(typeID1): allTokens[:2], string(typeID2): allTokens[2:4]},
		},
		{
			allowedTypes: []types.TokenTypeID{testutils.RandomBytes(32)},
			expected:     map[string][]*types.FungibleToken{},
		},
		{
			allowedTypes: []types.TokenTypeID{typeID3},
			expected:     map[string][]*types.FungibleToken{},
		},
		{
			allowedTypes: []types.TokenTypeID{typeID1},
			expected:     map[string][]*types.FungibleToken{string(typeID1): allTokens[:2]},
		},
		{
			allowedTypes: []types.TokenTypeID{typeID2},
			expected:     map[string][]*types.FungibleToken{string(typeID2): allTokens[2:4]},
		},
		{
			allowedTypes: []types.TokenTypeID{typeID1, typeID2},
			expected:     map[string][]*types.FungibleToken{string(typeID1): allTokens[:2], string(typeID2): allTokens[2:4]},
		},
		{
			allowedTypes: []types.TokenTypeID{typeID4},
			expected:     map[string][]*types.FungibleToken{},
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
