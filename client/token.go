package client

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"

	"github.com/alphabill-org/alphabill-wallet/client/tx"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
)

const (
	uriMaxSize  = 4 * 1024
	dataMaxSize = 64 * 1024
	nameMaxSize = 256
)

var (
	errInvalidURILength  = fmt.Errorf("URI exceeds the maximum allowed size of %v bytes", uriMaxSize)
	errInvalidDataLength = fmt.Errorf("data exceeds the maximum allowed size of %v bytes", dataMaxSize)
	errInvalidNameLength = fmt.Errorf("name exceeds the maximum allowed size of %v bytes", nameMaxSize)
)

type (
	token struct {
		systemID       types.SystemID
		id             sdktypes.TokenID
		symbol         string
		typeID         sdktypes.TokenTypeID
		typeName       string
		ownerPredicate []byte // TODO: could use sdktypes.Predicate?
		nonce          []byte // TODO: could be uint64? it is elsewhere
		counter        uint64
		lockStatus     uint64
	}

	fungibleToken struct {
		token

		amount        uint64
		decimalPlaces uint32
		burned        bool
	}

	nonFungibleToken struct {
		token

		name                string
		uri                 string
		data                []byte
		dataUpdatePredicate sdktypes.Predicate
	}
)

func NewFungibleToken(params *sdktypes.FungibleTokenParams) (sdktypes.FungibleToken, error) {
	return &fungibleToken{
		token: token{
			systemID:       params.SystemID,
			typeID:         params.TypeID,
			ownerPredicate: params.OwnerPredicate,
		},
		amount: params.Amount,
	}, nil
}

func NewNonFungibleToken(params *sdktypes.NonFungibleTokenParams) (sdktypes.NonFungibleToken, error) {
	if len(params.Name) > nameMaxSize {
		return nil, errInvalidNameLength
	}
	if len(params.URI) > uriMaxSize {
		return nil, errInvalidURILength
	}
	if params.URI != "" && !util.IsValidURI(params.URI) {
		return nil, fmt.Errorf("URI '%s' is invalid", params.URI)
	}
	if len(params.Data) > dataMaxSize {
		return nil, errInvalidDataLength
	}

	return &nonFungibleToken{
		token: token{
			systemID:       params.SystemID,
			typeID:         params.TypeID,
			ownerPredicate: params.OwnerPredicate,
		},
		name:                params.Name,
		uri:                 params.URI,
		data:                params.Data,
		dataUpdatePredicate: params.DataUpdatePredicate,
	}, nil
}

func (t *fungibleToken) Create(txOptions ...tx.Option) (*types.TransactionOrder, error) {
	attr := &tokens.MintFungibleTokenAttributes{
		Bearer:                           t.ownerPredicate,
		TypeID:                           t.typeID,
		Value:                            t.amount,
		Nonce:                            0,
		TokenCreationPredicateSignatures: nil,
	}
	txPayload, err := tx.NewPayload(t.systemID, nil, tokens.PayloadTypeMintFungibleToken, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	// generate tokenID
	unitPart, err := tokens.HashForNewTokenID(attr, txPayload.ClientMetadata, crypto.SHA256)
	if err != nil {
		return nil, err
	}
	txPayload.UnitID = tokens.NewFungibleTokenID(t.id, unitPart)
	t.id = txPayload.UnitID

	txo := tx.NewTransactionOrder(txPayload)
	err = tx.GenerateAndSetProofs(txo, attr, &attr.TokenCreationPredicateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}

	return txo, nil
}

func (t *fungibleToken) Transfer(ownerPredicate []byte, txOptions ...tx.Option) (*types.TransactionOrder, error) {
	attr := &tokens.TransferFungibleTokenAttributes{
		NewBearer:                    ownerPredicate,
		Value:                        t.amount,
		Nonce:                        t.nonce,
		Counter:                      t.counter,
		TypeID:                       t.typeID,
		InvariantPredicateSignatures: nil,
	}
	txPayload, err := tx.NewPayload(t.systemID, t.id, tokens.PayloadTypeTransferFungibleToken, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	txo := tx.NewTransactionOrder(txPayload)
	err = tx.GenerateAndSetProofs(txo, attr, &attr.InvariantPredicateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}
	return txo, nil
}

func (t *fungibleToken) Split(amount uint64, ownerPredicate []byte, txOptions ...tx.Option) (*types.TransactionOrder, error) {
	attr := &tokens.SplitFungibleTokenAttributes{
		NewBearer:                    ownerPredicate,
		TargetValue:                  amount,
		Nonce:                        nil,
		Counter:                      t.counter,
		TypeID:                       t.typeID,
		RemainingValue:               t.amount - amount,
		InvariantPredicateSignatures: [][]byte{nil}, // TODO: could be just nil?
	}
	txPayload, err := tx.NewPayload(t.systemID, t.id, tokens.PayloadTypeSplitFungibleToken, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	txo := tx.NewTransactionOrder(txPayload)
	err = tx.GenerateAndSetProofs(txo, attr, &attr.InvariantPredicateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}
	return txo, nil
}

func (t *fungibleToken) Burn(targetTokenID types.UnitID, targetTokenCounter uint64, txOptions ...tx.Option) (*types.TransactionOrder, error) {
	attr := &tokens.BurnFungibleTokenAttributes{
		TypeID:                       t.typeID,
		Value:                        t.amount,
		TargetTokenID:                targetTokenID,
		TargetTokenCounter:           targetTokenCounter,
		Counter:                      t.counter,
		InvariantPredicateSignatures: nil,
	}
	txPayload, err := tx.NewPayload(t.systemID, t.id, tokens.PayloadTypeBurnFungibleToken, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	txo := tx.NewTransactionOrder(txPayload)
	err = tx.GenerateAndSetProofs(txo, attr, &attr.InvariantPredicateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}
	return txo, nil
}

func (t *fungibleToken) Join(burnTxs []*types.TransactionRecord, burnProofs []*types.TxProof, txOptions ...tx.Option) (*types.TransactionOrder, error) {
	attr := &tokens.JoinFungibleTokenAttributes{
		BurnTransactions:             burnTxs,
		Proofs:                       burnProofs,
		Counter:                      t.counter,
		InvariantPredicateSignatures: nil,
	}
	txPayload, err := tx.NewPayload(t.systemID, t.id, tokens.PayloadTypeJoinFungibleToken, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	txo := tx.NewTransactionOrder(txPayload)
	err = tx.GenerateAndSetProofs(txo, attr, &attr.InvariantPredicateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}
	return txo, nil
}

func (t *fungibleToken) Amount() uint64 {
	return t.amount
}

func (t *fungibleToken) DecimalPlaces() uint32 {
	return t.decimalPlaces
}

func (t *fungibleToken) Burned() bool {
	return t.burned
}

func (t *nonFungibleToken) Create(txOptions ...tx.Option) (*types.TransactionOrder, error) {
	attr := &tokens.MintNonFungibleTokenAttributes{
		Bearer:                           t.ownerPredicate,
		TypeID:                           t.typeID,
		Name:                             t.name,
		URI:                              t.uri,
		Data:                             t.data,
		DataUpdatePredicate:              t.dataUpdatePredicate,
		Nonce:                            0,
		TokenCreationPredicateSignatures: nil,
	}
	txPayload, err := tx.NewPayload(t.systemID, nil, tokens.PayloadTypeMintNFT, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	// generate tokenID
	unitPart, err := tokens.HashForNewTokenID(attr, txPayload.ClientMetadata, crypto.SHA256)
	if err != nil {
		return nil, err
	}
	txPayload.UnitID = tokens.NewFungibleTokenID(t.id, unitPart)
	t.id = txPayload.UnitID

	txo := tx.NewTransactionOrder(txPayload)
	err = tx.GenerateAndSetProofs(txo, attr, &attr.TokenCreationPredicateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}
	return txo, nil
}

func (t *nonFungibleToken) Transfer(ownerPredicate []byte, txOptions ...tx.Option) (*types.TransactionOrder, error) {
	attr := &tokens.TransferNonFungibleTokenAttributes{
		NewBearer:                    ownerPredicate,
		Nonce:                        t.nonce,
		Counter:                      t.counter,
		TypeID:                       t.typeID,
		InvariantPredicateSignatures: nil,
	}
	txPayload, err := tx.NewPayload(t.systemID, t.id, tokens.PayloadTypeTransferNFT, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	txo := tx.NewTransactionOrder(txPayload)
	err = tx.GenerateAndSetProofs(txo, attr, &attr.InvariantPredicateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}
	return txo, nil
}

func (t *nonFungibleToken) Update(data []byte, txOptions ...tx.Option) (*types.TransactionOrder, error) {
	attr := &tokens.UpdateNonFungibleTokenAttributes{
		Data:                 data,
		Counter:              t.counter,
		DataUpdateSignatures: nil,
	}
	txPayload, err := tx.NewPayload(t.systemID, t.id, tokens.PayloadTypeUpdateNFT, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	txo := tx.NewTransactionOrder(txPayload)
	err = tx.GenerateAndSetProofs(txo, attr, &attr.DataUpdateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}
	return txo, nil
}

func (t *nonFungibleToken) Name() string {
	return t.name
}

func (t *nonFungibleToken) URI() string {
	return t.uri
}

func (t *nonFungibleToken) Data() []byte {
	return t.data
}

func (t *nonFungibleToken) DataUpdatePredicate() sdktypes.Predicate {
	return t.dataUpdatePredicate
}

func (t *token) Lock(lockStatus uint64, txOptions ...tx.Option) (*types.TransactionOrder, error) {
	attr := &tokens.LockTokenAttributes{
		LockStatus:                   lockStatus,
		Counter:                      t.counter,
		InvariantPredicateSignatures: nil,
	}
	txPayload, err := tx.NewPayload(t.systemID, t.id, tokens.PayloadTypeLockToken, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	txo := tx.NewTransactionOrder(txPayload)
	err = tx.GenerateAndSetProofs(txo, attr, &attr.InvariantPredicateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}
	return txo, nil
}

func (t *token) Unlock(txOptions ...tx.Option) (*types.TransactionOrder, error) {
	attr := &tokens.LockTokenAttributes{
		Counter:                      t.counter,
		InvariantPredicateSignatures: nil,
	}
	txPayload, err := tx.NewPayload(t.systemID, t.id, tokens.PayloadTypeUnlockToken, attr, txOptions...)
	if err != nil {
		return nil, err
	}

	txo := tx.NewTransactionOrder(txPayload)
	err = tx.GenerateAndSetProofs(txo, attr, &attr.InvariantPredicateSignatures, txOptions...)
	if err != nil {
		return nil, err
	}
	return txo, nil
}

func (t *token) SystemID() types.SystemID {
	return t.systemID
}

func (t *token) ID() sdktypes.TokenID {
	return t.id
}

func (t *token) TypeID() sdktypes.TokenTypeID {
	return t.typeID
}

func (t *token) TypeName() string {
	return t.typeName
}

func (t *token) Symbol() string {
	return t.symbol
}

func (t *token) OwnerPredicate() []byte {
	return t.ownerPredicate
}

func (t *token) Nonce() []byte {
	return t.nonce
}

func (t *token) LockStatus() uint64 {
	return t.lockStatus
}

func (t *token) Counter() uint64 {
	return t.counter
}

func (t *token) IncreaseCounter() {
	t.counter += 1
}
