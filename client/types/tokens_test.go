package types

import (
	"testing"

	tokenid "github.com/alphabill-org/alphabill-go-base/testutils/tokens"
	"github.com/alphabill-org/alphabill-go-base/txsystem/nop"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/stretchr/testify/require"
)

func TestFungibleTokenTypeCreate(t *testing.T) {
	pdr := tokenid.PDR()
	tt := &FungibleTokenType{
		NetworkID:    types.NetworkLocal,
		PartitionID:  tokens.DefaultPartitionID,
		ID:           tokenid.NewFungibleTokenTypeID(t),
		ParentTypeID: tokenid.NewFungibleTokenTypeID(t),
		Symbol:       "symbol",
		Name:         "name",
		Icon: &tokens.Icon{
			Type: "image/png",
			Data: []byte{3, 2, 1},
		},
		SubTypeCreationPredicate: []byte{1},
		TokenMintingPredicate:    []byte{2},
		TokenTypeOwnerPredicate:  []byte{3},
		DecimalPlaces:            8,
	}
	timeout := uint64(11)
	refNo := "asdf"
	tx, err := tt.Define(
		WithTimeout(timeout),
		WithReferenceNumber([]byte(refNo)))
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, tokens.TransactionTypeDefineFT)
	require.EqualValues(t, tt.PartitionID, tx.GetPartitionID())
	require.NotNil(t, tt.ID)
	require.NoError(t, tt.ID.TypeMustBe(tokens.FungibleTokenTypeUnitType, &pdr))
	require.EqualValues(t, tt.ID, tx.GetUnitID())
	require.EqualValues(t, timeout, tx.Timeout())
	require.EqualValues(t, refNo, tx.Payload.ClientMetadata.ReferenceNumber)
	require.Nil(t, tx.AuthProof)

	attr := &tokens.DefineFungibleTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, tt.ParentTypeID, attr.ParentTypeID)
	require.Equal(t, tt.Symbol, attr.Symbol)
	require.Equal(t, tt.Name, attr.Name)
	require.Equal(t, tt.Icon, attr.Icon)
	require.EqualValues(t, tt.SubTypeCreationPredicate, attr.SubTypeCreationPredicate)
	require.EqualValues(t, tt.TokenMintingPredicate, attr.TokenMintingPredicate)
	require.EqualValues(t, tt.TokenTypeOwnerPredicate, attr.TokenTypeOwnerPredicate)
	require.Equal(t, tt.DecimalPlaces, attr.DecimalPlaces)
}

func TestNonFungibleTokenTypeCreate(t *testing.T) {
	pdr := tokenid.PDR()
	tt := &NonFungibleTokenType{
		NetworkID:    types.NetworkLocal,
		PartitionID:  tokens.DefaultPartitionID,
		ID:           tokenid.NewFungibleTokenTypeID(t),
		ParentTypeID: tokenid.NewFungibleTokenTypeID(t),
		Symbol:       "symbol",
		Name:         "name",
		Icon: &tokens.Icon{
			Type: "image/png",
			Data: []byte{3, 2, 1},
		},
		SubTypeCreationPredicate: []byte{1},
		TokenMintingPredicate:    []byte{2},
		TokenTypeOwnerPredicate:  []byte{3},
		DataUpdatePredicate:      []byte{4},
	}
	tx, err := tt.Define()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, tokens.TransactionTypeDefineNFT)
	require.EqualValues(t, tt.PartitionID, tx.GetPartitionID())
	require.NotNil(t, tt.ID)
	require.NoError(t, tt.ID.TypeMustBe(tokens.FungibleTokenTypeUnitType, &pdr))
	require.EqualValues(t, tt.ID, tx.GetUnitID())
	require.Nil(t, tx.AuthProof)

	attr := &tokens.DefineNonFungibleTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, tt.ParentTypeID, attr.ParentTypeID)
	require.Equal(t, tt.Symbol, attr.Symbol)
	require.Equal(t, tt.Name, attr.Name)
	require.Equal(t, tt.Icon, attr.Icon)
	require.EqualValues(t, tt.SubTypeCreationPredicate, attr.SubTypeCreationPredicate)
	require.EqualValues(t, tt.TokenMintingPredicate, attr.TokenMintingPredicate)
	require.EqualValues(t, tt.TokenTypeOwnerPredicate, attr.TokenTypeOwnerPredicate)
	require.EqualValues(t, tt.DataUpdatePredicate, attr.DataUpdatePredicate)
}

func TestFungibleTokenCreate(t *testing.T) {
	pdr := tokenid.PDR()
	ft := &FungibleToken{
		NetworkID:      types.NetworkLocal,
		PartitionID:    tokens.DefaultPartitionID,
		OwnerPredicate: []byte{99},
		TypeID:         tokenid.NewFungibleTokenTypeID(t),
		Amount:         uint64(50),
	}
	tx, err := ft.Mint(&pdr)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, tokens.TransactionTypeMintFT)
	require.EqualValues(t, ft.PartitionID, tx.GetPartitionID())
	require.NotNil(t, ft.ID)
	require.NoError(t, ft.ID.TypeMustBe(tokens.FungibleTokenUnitType, &pdr))
	require.EqualValues(t, ft.ID, tx.GetUnitID())
	require.Nil(t, tx.AuthProof)

	attr := &tokens.MintFungibleTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, ft.OwnerPredicate, attr.OwnerPredicate)
	require.Equal(t, ft.TypeID, attr.TypeID)
	require.Equal(t, ft.Amount, attr.Value)
}

func TestFungibleTokenTransfer(t *testing.T) {
	ft := &FungibleToken{
		NetworkID:      types.NetworkLocal,
		PartitionID:    tokens.DefaultPartitionID,
		ID:             tokenid.NewFungibleTokenID(t),
		OwnerPredicate: []byte{2},
		TypeID:         tokenid.NewFungibleTokenTypeID(t),
		Amount:         uint64(4),
	}
	newOwnerPredicate := []byte{5}
	tx, err := ft.Transfer(newOwnerPredicate)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, tokens.TransactionTypeTransferFT)
	require.EqualValues(t, ft.PartitionID, tx.GetPartitionID())
	require.EqualValues(t, ft.ID, tx.GetUnitID())

	attr := &tokens.TransferFungibleTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, newOwnerPredicate, attr.NewOwnerPredicate)
	require.Equal(t, ft.Amount, attr.Value)
	require.Equal(t, ft.TypeID, attr.TypeID)
}

func TestFungibleTokenSplit(t *testing.T) {
	ft := &FungibleToken{
		NetworkID:      types.NetworkLocal,
		PartitionID:    tokens.DefaultPartitionID,
		ID:             tokenid.NewFungibleTokenID(t),
		OwnerPredicate: []byte{2},
		TypeID:         tokenid.NewFungibleTokenTypeID(t),
		Amount:         uint64(4),
	}
	newOwnerPredicate := []byte{5}
	tx, err := ft.Split(3, newOwnerPredicate)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, tokens.TransactionTypeSplitFT)
	require.Equal(t, ft.PartitionID, tx.GetPartitionID())
	require.Equal(t, ft.ID, tx.GetUnitID())

	attr := &tokens.SplitFungibleTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, newOwnerPredicate, attr.NewOwnerPredicate)
	require.EqualValues(t, 3, attr.TargetValue)
	require.Equal(t, ft.TypeID, attr.TypeID)
}

func TestFungibleTokenBurn(t *testing.T) {
	ft := &FungibleToken{
		NetworkID:      types.NetworkLocal,
		PartitionID:    tokens.DefaultPartitionID,
		ID:             tokenid.NewFungibleTokenID(t),
		OwnerPredicate: []byte{2},
		TypeID:         tokenid.NewFungibleTokenTypeID(t),
		Amount:         uint64(4),
	}
	targetTokenID := tokenid.NewFungibleTokenID(t)
	targetTokenCounter := uint64(6)

	tx, err := ft.Burn(targetTokenID, targetTokenCounter)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, tokens.TransactionTypeBurnFT)
	require.Equal(t, ft.PartitionID, tx.GetPartitionID())
	require.Equal(t, ft.ID, tx.GetUnitID())

	attr := &tokens.BurnFungibleTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, targetTokenID, attr.TargetTokenID)
	require.Equal(t, targetTokenCounter, attr.TargetTokenCounter)
	require.Equal(t, ft.TypeID, attr.TypeID)
	require.Equal(t, ft.Amount, attr.Value)
}

func TestFungibleTokenJoin(t *testing.T) {
	ft := &FungibleToken{
		NetworkID:      types.NetworkLocal,
		PartitionID:    tokens.DefaultPartitionID,
		ID:             tokenid.NewFungibleTokenID(t),
		OwnerPredicate: []byte{2},
		TypeID:         tokenid.NewFungibleTokenTypeID(t),
		Amount:         uint64(4),
	}

	tx, err := ft.Join(nil)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, tokens.TransactionTypeJoinFT)
	require.Equal(t, ft.PartitionID, tx.GetPartitionID())
	require.Equal(t, ft.ID, tx.GetUnitID())

	attr := &tokens.JoinFungibleTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Nil(t, attr.BurnTokenProofs)
}

func TestFungibleTokenLock(t *testing.T) {
	ft := &FungibleToken{
		NetworkID:      types.NetworkLocal,
		PartitionID:    tokens.DefaultPartitionID,
		ID:             tokenid.NewFungibleTokenID(t),
		OwnerPredicate: []byte{2},
		TypeID:         tokenid.NewFungibleTokenTypeID(t),
		Amount:         4,
		Counter:        5,
	}

	stateLock := &types.StateLock{
		ExecutionPredicate: []byte{1},
		RollbackPredicate:  []byte{2},
	}
	tx, err := ft.Lock(stateLock)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, nop.TransactionTypeNOP)
	require.Equal(t, ft.PartitionID, tx.GetPartitionID())
	require.Equal(t, ft.ID, tx.GetUnitID())
	require.Equal(t, stateLock, tx.StateLock)

	attr := &nop.Attributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.EqualValues(t, 5, *attr.Counter)
}

func TestNonFungibleTokenUnlock(t *testing.T) {
	ft := &FungibleToken{
		NetworkID:      types.NetworkLocal,
		PartitionID:    tokens.DefaultPartitionID,
		ID:             tokenid.NewFungibleTokenID(t),
		OwnerPredicate: []byte{2},
		TypeID:         tokenid.NewFungibleTokenTypeID(t),
		Amount:         uint64(4),
	}

	tx, err := ft.Unlock()
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, nop.TransactionTypeNOP)
	require.Equal(t, ft.PartitionID, tx.GetPartitionID())
	require.Equal(t, ft.ID, tx.GetUnitID())

	attr := &nop.Attributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
}

func TestNonFungibleTokenCreate(t *testing.T) {
	pdr := tokenid.PDR()
	nft := &NonFungibleToken{
		NetworkID:           types.NetworkLocal,
		PartitionID:         tokens.DefaultPartitionID,
		OwnerPredicate:      []byte{1},
		TypeID:              tokenid.NewFungibleTokenTypeID(t),
		Name:                "name",
		URI:                 "uri",
		Data:                []byte{3},
		DataUpdatePredicate: []byte{4},
	}
	tx, err := nft.Mint(&pdr)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, tokens.TransactionTypeMintNFT)
	require.EqualValues(t, nft.PartitionID, tx.GetPartitionID())
	require.NotNil(t, nft.ID)
	require.NoError(t, nft.ID.TypeMustBe(tokens.NonFungibleTokenUnitType, &pdr))
	require.EqualValues(t, nft.ID, tx.GetUnitID())
	require.Nil(t, tx.AuthProof)

	attr := &tokens.MintNonFungibleTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, nft.OwnerPredicate, attr.OwnerPredicate)
	require.Equal(t, nft.TypeID, attr.TypeID)
	require.Equal(t, nft.Name, attr.Name)
	require.Equal(t, nft.URI, attr.URI)
	require.Equal(t, nft.Data, attr.Data)
	require.EqualValues(t, nft.DataUpdatePredicate, attr.DataUpdatePredicate)
}

func TestNonFungibleTokenTransfer(t *testing.T) {
	nft := &NonFungibleToken{
		NetworkID:      types.NetworkLocal,
		PartitionID:    tokens.DefaultPartitionID,
		ID:             tokenid.NewNonFungibleTokenID(t),
		OwnerPredicate: []byte{2},
		TypeID:         tokenid.NewNonFungibleTokenTypeID(t),
	}
	newOwnerPredicate := []byte{4}
	tx, err := nft.Transfer(newOwnerPredicate)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, tokens.TransactionTypeTransferNFT)
	require.EqualValues(t, nft.PartitionID, tx.GetPartitionID())
	require.EqualValues(t, nft.ID, tx.GetUnitID())

	attr := &tokens.TransferNonFungibleTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, newOwnerPredicate, attr.NewOwnerPredicate)
	require.Equal(t, nft.TypeID, attr.TypeID)
}

func TestNonFungibleTokenUpdate(t *testing.T) {
	nft := &NonFungibleToken{
		NetworkID:      types.NetworkLocal,
		PartitionID:    tokens.DefaultPartitionID,
		ID:             tokenid.NewNonFungibleTokenID(t),
		OwnerPredicate: []byte{2},
		TypeID:         tokenid.NewNonFungibleTokenTypeID(t),
		Data:           []byte{4},
	}
	newData := []byte{5}
	tx, err := nft.Update(newData)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.Equal(t, tx.Type, tokens.TransactionTypeUpdateNFT)
	require.EqualValues(t, nft.PartitionID, tx.GetPartitionID())
	require.EqualValues(t, nft.ID, tx.GetUnitID())

	attr := &tokens.UpdateNonFungibleTokenAttributes{}
	require.NoError(t, tx.UnmarshalAttributes(attr))
	require.Equal(t, newData, attr.Data)
}
