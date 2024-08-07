package types

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/stretchr/testify/require"
)

func TestFungibleTokenTypeCreate(t *testing.T) {
	tt := &FungibleTokenType{
		SystemID:     tokens.DefaultSystemID,
		ID:           tokens.NewFungibleTokenTypeID(nil, []byte{2}),
		ParentTypeID: tokens.NewFungibleTokenTypeID(nil, []byte{1}),
		Symbol:       "symbol",
		Name:         "name",
		Icon: &tokens.Icon{
			Type: "image/png",
			Data: []byte{3, 2, 1},
		},
		SubTypeCreationPredicate: []byte{1},
		TokenCreationPredicate:   []byte{2},
		InvariantPredicate:       []byte{3},
		DecimalPlaces:            8,
	}
	timeout := uint64(11)
	refNo := "asdf"
	tx, err := tt.Create(
		WithTimeout(timeout),
		WithReferenceNumber([]byte(refNo)))
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.PayloadType(), tokens.PayloadTypeCreateFungibleTokenType)
	require.EqualValues(t, tt.SystemID, tx.SystemID())
	require.NotNil(t, tt.ID)
	require.True(t, tt.ID.HasType(tokens.FungibleTokenTypeUnitType))
	require.EqualValues(t, tt.ID, tx.UnitID())
	require.EqualValues(t, timeout, tx.Timeout())
	require.EqualValues(t, refNo, tx.Payload.ClientMetadata.ReferenceNumber)
	require.Nil(t, tx.OwnerProof)

	attr := &tokens.CreateFungibleTokenTypeAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, tt.ParentTypeID, attr.ParentTypeID)
	require.Equal(t, tt.Symbol, attr.Symbol)
	require.Equal(t, tt.Name, attr.Name)
	require.Equal(t, tt.Icon, attr.Icon)
	require.EqualValues(t, tt.SubTypeCreationPredicate, attr.SubTypeCreationPredicate)
	require.EqualValues(t, tt.TokenCreationPredicate, attr.TokenCreationPredicate)
	require.EqualValues(t, tt.InvariantPredicate, attr.InvariantPredicate)
	require.Equal(t, tt.DecimalPlaces, attr.DecimalPlaces)
}

func TestNonFungibleTokenTypeCreate(t *testing.T) {
	tt := &NonFungibleTokenType{
		SystemID:     tokens.DefaultSystemID,
		ID:           tokens.NewFungibleTokenTypeID(nil, []byte{2}),
		ParentTypeID: tokens.NewFungibleTokenTypeID(nil, []byte{1}),
		Symbol:       "symbol",
		Name:         "name",
		Icon: &tokens.Icon{
			Type: "image/png",
			Data: []byte{3, 2, 1},
		},
		SubTypeCreationPredicate: []byte{1},
		TokenCreationPredicate:   []byte{2},
		InvariantPredicate:       []byte{3},
		DataUpdatePredicate:      []byte{4},
	}
	tx, err := tt.Create()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.PayloadType(), tokens.PayloadTypeCreateNFTType)
	require.EqualValues(t, tt.SystemID, tx.SystemID())
	require.NotNil(t, tt.ID)
	require.True(t, tt.ID.HasType(tokens.FungibleTokenTypeUnitType))
	require.EqualValues(t, tt.ID, tx.UnitID())
	require.Nil(t, tx.OwnerProof)

	attr := &tokens.CreateNonFungibleTokenTypeAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, tt.ParentTypeID, attr.ParentTypeID)
	require.Equal(t, tt.Symbol, attr.Symbol)
	require.Equal(t, tt.Name, attr.Name)
	require.Equal(t, tt.Icon, attr.Icon)
	require.EqualValues(t, tt.SubTypeCreationPredicate, attr.SubTypeCreationPredicate)
	require.EqualValues(t, tt.TokenCreationPredicate, attr.TokenCreationPredicate)
	require.EqualValues(t, tt.InvariantPredicate, attr.InvariantPredicate)
	require.EqualValues(t, tt.DataUpdatePredicate, attr.DataUpdatePredicate)
}

func TestFungibleTokenCreate(t *testing.T) {
	ft := &FungibleToken{
		SystemID:       tokens.DefaultSystemID,
		OwnerPredicate: []byte{99},
		TypeID:         tokens.NewFungibleTokenTypeID(nil, []byte{1}),
		Amount:         uint64(50),
	}
	tx, err := ft.Create()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.PayloadType(), tokens.PayloadTypeMintFungibleToken)
	require.EqualValues(t, ft.SystemID, tx.SystemID())
	require.NotNil(t, ft.ID)
	require.True(t, ft.ID.HasType(tokens.FungibleTokenUnitType))
	require.EqualValues(t, ft.ID, tx.UnitID())
	require.Nil(t, tx.OwnerProof)

	attr := &tokens.MintFungibleTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, ft.OwnerPredicate, attr.Bearer)
	require.Equal(t, ft.TypeID, attr.TypeID)
	require.Equal(t, ft.Amount, attr.Value)
}

func TestFungibleTokenTransfer(t *testing.T) {
	ft := &FungibleToken{
		SystemID:       tokens.DefaultSystemID,
		ID:             tokens.NewFungibleTokenID(nil, []byte{1}),
		OwnerPredicate: []byte{2},
		TypeID:         tokens.NewFungibleTokenTypeID(nil, []byte{3}),
		Amount:         uint64(4),
	}
	newOwnerPredicate := []byte{5}
	tx, err := ft.Transfer(newOwnerPredicate)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.PayloadType(), tokens.PayloadTypeTransferFungibleToken)
	require.EqualValues(t, ft.SystemID, tx.SystemID())
	require.EqualValues(t, ft.ID, tx.UnitID())

	attr := &tokens.TransferFungibleTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, newOwnerPredicate, attr.NewBearer)
	require.Equal(t, ft.Amount, attr.Value)
	require.Equal(t, ft.TypeID, attr.TypeID)
}

func TestFungibleTokenSplit(t *testing.T) {
	ft := &FungibleToken{
		SystemID:       tokens.DefaultSystemID,
		ID:             tokens.NewFungibleTokenID(nil, []byte{1}),
		OwnerPredicate: []byte{2},
		TypeID:         tokens.NewFungibleTokenTypeID(nil, []byte{3}),
		Amount:         uint64(4),
	}
	newOwnerPredicate := []byte{5}
	tx, err := ft.Split(3, newOwnerPredicate)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.PayloadType(), tokens.PayloadTypeSplitFungibleToken)
	require.Equal(t, ft.SystemID, tx.SystemID())
	require.Equal(t, ft.ID, tx.UnitID())

	attr := &tokens.SplitFungibleTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, newOwnerPredicate, attr.NewBearer)
	require.EqualValues(t, 3, attr.TargetValue)
	require.EqualValues(t, 1, attr.RemainingValue)
	require.Equal(t, ft.TypeID, attr.TypeID)
}

func TestFungibleTokenBurn(t *testing.T) {
	ft := &FungibleToken{
		SystemID:       tokens.DefaultSystemID,
		ID:             tokens.NewFungibleTokenID(nil, []byte{1}),
		OwnerPredicate: []byte{2},
		TypeID:         tokens.NewFungibleTokenTypeID(nil, []byte{3}),
		Amount:         uint64(4),
	}
	targetTokenID := tokens.NewFungibleTokenID(nil, []byte{5})
	targetTokenCounter := uint64(6)

	tx, err := ft.Burn(targetTokenID, targetTokenCounter)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.PayloadType(), tokens.PayloadTypeBurnFungibleToken)
	require.Equal(t, ft.SystemID, tx.SystemID())
	require.Equal(t, ft.ID, tx.UnitID())

	attr := &tokens.BurnFungibleTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, targetTokenID, attr.TargetTokenID)
	require.Equal(t, targetTokenCounter, attr.TargetTokenCounter)
	require.Equal(t, ft.TypeID, attr.TypeID)
	require.Equal(t, ft.Amount, attr.Value)
}

func TestFungibleTokenJoin(t *testing.T) {
	ft := &FungibleToken{
		SystemID:       tokens.DefaultSystemID,
		ID:             tokens.NewFungibleTokenID(nil, []byte{1}),
		OwnerPredicate: []byte{2},
		TypeID:         tokens.NewFungibleTokenTypeID(nil, []byte{3}),
		Amount:         uint64(4),
	}

	tx, err := ft.Join(nil, nil)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.PayloadType(), tokens.PayloadTypeJoinFungibleToken)
	require.Equal(t, ft.SystemID, tx.SystemID())
	require.Equal(t, ft.ID, tx.UnitID())

	attr := &tokens.JoinFungibleTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Nil(t, attr.BurnTransactions)
	require.Nil(t, attr.Proofs)
}

func TestFungibleTokenLock(t *testing.T) {
	ft := &FungibleToken{
		SystemID:       tokens.DefaultSystemID,
		ID:             tokens.NewFungibleTokenID(nil, []byte{1}),
		OwnerPredicate: []byte{2},
		TypeID:         tokens.NewFungibleTokenTypeID(nil, []byte{3}),
		Amount:         uint64(4),
	}

	tx, err := ft.Lock(5)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.PayloadType(), tokens.PayloadTypeLockToken)
	require.Equal(t, ft.SystemID, tx.SystemID())
	require.Equal(t, ft.ID, tx.UnitID())

	attr := &tokens.LockTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.EqualValues(t, 5, attr.LockStatus)
}

func TestNonFungibleTokenUnlock(t *testing.T) {
	ft := &FungibleToken{
		SystemID:       tokens.DefaultSystemID,
		ID:             tokens.NewFungibleTokenID(nil, []byte{1}),
		OwnerPredicate: []byte{2},
		TypeID:         tokens.NewFungibleTokenTypeID(nil, []byte{3}),
		Amount:         uint64(4),
	}

	tx, err := ft.Unlock()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.PayloadType(), tokens.PayloadTypeUnlockToken)
	require.Equal(t, ft.SystemID, tx.SystemID())
	require.Equal(t, ft.ID, tx.UnitID())

	attr := &tokens.UnlockTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
}

func TestNonFungibleTokenCreate(t *testing.T) {
	nft := &NonFungibleToken{
		SystemID:            tokens.DefaultSystemID,
		OwnerPredicate:      []byte{1},
		TypeID:              tokens.NewFungibleTokenTypeID(nil, []byte{2}),
		Name:                "name",
		URI:                 "uri",
		Data:                []byte{3},
		DataUpdatePredicate: []byte{4},
	}
	tx, err := nft.Create()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.PayloadType(), tokens.PayloadTypeMintNFT)
	require.EqualValues(t, nft.SystemID, tx.SystemID())
	require.NotNil(t, nft.ID)
	require.True(t, nft.ID.HasType(tokens.NonFungibleTokenUnitType))
	require.EqualValues(t, nft.ID, tx.UnitID())
	require.Nil(t, tx.OwnerProof)

	attr := &tokens.MintNonFungibleTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, nft.OwnerPredicate, attr.Bearer)
	require.Equal(t, nft.TypeID, attr.TypeID)
	require.Equal(t, nft.Name, attr.Name)
	require.Equal(t, nft.URI, attr.URI)
	require.Equal(t, nft.Data, attr.Data)
	require.EqualValues(t, nft.DataUpdatePredicate, attr.DataUpdatePredicate)
}

func TestNonFungibleTokenTransfer(t *testing.T) {
	nft := &NonFungibleToken{
		SystemID:       tokens.DefaultSystemID,
		ID:             tokens.NewNonFungibleTokenID(nil, []byte{1}),
		OwnerPredicate: []byte{2},
		TypeID:         tokens.NewNonFungibleTokenTypeID(nil, []byte{3}),
	}
	newOwnerPredicate := []byte{4}
	tx, err := nft.Transfer(newOwnerPredicate)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.PayloadType(), tokens.PayloadTypeTransferNFT)
	require.EqualValues(t, nft.SystemID, tx.SystemID())
	require.EqualValues(t, nft.ID, tx.UnitID())

	attr := &tokens.TransferNonFungibleTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, newOwnerPredicate, attr.NewBearer)
	require.Equal(t, nft.TypeID, attr.TypeID)
}

func TestNonFungibleTokenUpdate(t *testing.T) {
	nft := &NonFungibleToken{
		SystemID:       tokens.DefaultSystemID,
		ID:             tokens.NewNonFungibleTokenID(nil, []byte{1}),
		OwnerPredicate: []byte{2},
		TypeID:         tokens.NewNonFungibleTokenTypeID(nil, []byte{3}),
		Data:           []byte{4},
	}
	newData := []byte{5}
	tx, err := nft.Update(newData)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.PayloadType(), tokens.PayloadTypeUpdateNFT)
	require.EqualValues(t, nft.SystemID, tx.SystemID())
	require.EqualValues(t, nft.ID, tx.UnitID())

	attr := &tokens.UpdateNonFungibleTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, newData, attr.Data)
}
