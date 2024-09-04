package types

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/types"
)

type (
	Options struct {
		Timeout           uint64
		FeeCreditRecordID types.UnitID
		MaxFee            uint64
		ReferenceNumber   []byte
	}

	Option func(*Options)
)

func NewTransactionOrder(payload *types.Payload) *types.TransactionOrder {
	return &types.TransactionOrder{
		Payload: payload,
	}
}

// NewPayload creates a new transaction payload.
func NewPayload(systemID types.SystemID, unitID types.UnitID, txType string, attr any, opts ...Option) (*types.Payload, error) {
	attrBytes, err := types.Cbor.Marshal(attr)
	if err != nil {
		return nil, err
	}

	o := OptionsWithDefaults(opts)
	return &types.Payload{
		SystemID:   systemID,
		Type:       txType,
		UnitID:     unitID,
		Attributes: attrBytes,
		ClientMetadata: &types.ClientMetadata{
			Timeout:           o.Timeout,
			MaxTransactionFee: o.MaxFee,
			FeeCreditRecordID: o.FeeCreditRecordID,
			ReferenceNumber:   o.ReferenceNumber,
		},
	}, nil
}

func WithReferenceNumber(referenceNumber []byte) Option {
	return func(os *Options) {
		os.ReferenceNumber = referenceNumber
	}
}

func WithTimeout(timeout uint64) Option {
	return func(os *Options) {
		os.Timeout = timeout
	}
}

func WithFeeCreditRecordID(feeCreditRecordID types.UnitID) Option {
	return func(os *Options) {
		os.FeeCreditRecordID = feeCreditRecordID
	}
}

func WithMaxFee(maxFee uint64) Option {
	return func(os *Options) {
		os.MaxFee = maxFee
	}
}

func OptionsWithDefaults(txOptions []Option) *Options {
	opts := &Options{
		MaxFee: 10,
	}
	for _, txOption := range txOptions {
		txOption(opts)
	}
	return opts
}

// NewP2pkhSignature creates a standard P2PKH predicate signature aka the "OwnerProof"
func NewP2pkhSignature(txo *types.TransactionOrder, signer crypto.Signer) ([]byte, error) {
	sigBytes, err := txo.PayloadBytes()
	if err != nil {
		return nil, err
	}
	sig, err := signer.SignBytes(sigBytes)
	if err != nil {
		return nil, err
	}
	verifier, err := signer.Verifier()
	if err != nil {
		return nil, err
	}
	pubKey, err := verifier.MarshalPublicKey()
	if err != nil {
		return nil, err
	}
	return templates.NewP2pkh256SignatureBytes(sig, pubKey), nil
}

// NewP2pkhSignatureFromKey creates a standard P2PKH predicate signature aka the "OwnerProof"
func NewP2pkhSignatureFromKey(txo *types.TransactionOrder, privKey []byte) ([]byte, error) {
	signer, err := crypto.NewInMemorySecp256K1SignerFromKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer from private key: %w", err)
	}
	return NewP2pkhSignature(txo, signer)
}

// NewP2pkhFeeSignature creates a standard P2PKH fee predicate signature aka the "FeeProof"
func NewP2pkhFeeSignature(txo *types.TransactionOrder, signer crypto.Signer) ([]byte, error) {
	sigBytes, err := txo.FeeProofSigBytes()
	if err != nil {
		return nil, err
	}
	sig, err := signer.SignBytes(sigBytes)
	if err != nil {
		return nil, err
	}
	pubKey, err := extractPubKey(signer)
	if err != nil {
		return nil, err
	}
	return templates.NewP2pkh256SignatureBytes(sig, pubKey), nil
}

// NewP2pkhFeeSignatureFromKey creates a standard P2PKH fee predicate signature aka the "FeeProof"
func NewP2pkhFeeSignatureFromKey(txo *types.TransactionOrder, privKey []byte) ([]byte, error) {
	signer, err := crypto.NewInMemorySecp256K1SignerFromKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer from private key: %w", err)
	}
	return NewP2pkhFeeSignature(txo, signer)
}

func extractPubKey(signer crypto.Signer) ([]byte, error) {
	verifier, err := signer.Verifier()
	if err != nil {
		return nil, err
	}
	pubKey, err := verifier.MarshalPublicKey()
	if err != nil {
		return nil, err
	}
	return pubKey, nil
}
