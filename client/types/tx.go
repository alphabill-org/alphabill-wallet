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

func NewTransactionOrder(networkID types.NetworkID, partitionID types.PartitionID, unitID types.UnitID, txType uint16, attr any, opts ...Option) (*types.TransactionOrder, error) {
	attrBytes, err := types.Cbor.Marshal(attr)
	if err != nil {
		return nil, err
	}

	o := OptionsWithDefaults(opts)
	return &types.TransactionOrder{
		Payload: types.Payload{
			NetworkID:   networkID,
			PartitionID: partitionID,
			UnitID:      unitID,
			Type:        txType,
			Attributes:  attrBytes,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           o.Timeout,
				MaxTransactionFee: o.MaxFee,
				FeeCreditRecordID: o.FeeCreditRecordID,
				ReferenceNumber:   o.ReferenceNumber,
			},
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

// NewP2pkhAuthProofSignature creates a standard P2PKH predicate signature for AuthProof.
func NewP2pkhAuthProofSignature(txo *types.TransactionOrder, signer crypto.Signer) ([]byte, error) {
	return NewP2pkhSignature(signer, txo.AuthProofSigBytes)
}

// NewP2pkhFeeProofSignature creates a standard P2PKH fee predicate signature for FeeProof.
func NewP2pkhFeeProofSignature(txo *types.TransactionOrder, signer crypto.Signer) ([]byte, error) {
	return NewP2pkhSignature(signer, txo.FeeProofSigBytes)
}

// NewP2pkhSignature creates a standard P2PKH predicate signature.
func NewP2pkhSignature(signer crypto.Signer, sigBytesFn func() ([]byte, error)) ([]byte, error) {
	sigBytes, err := sigBytesFn()
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

// NewP2pkhAuthProofSignatureFromKey creates a standard P2PKH predicate signature for AuthProof.
func NewP2pkhAuthProofSignatureFromKey(txo *types.TransactionOrder, privKey []byte) ([]byte, error) {
	signer, err := crypto.NewInMemorySecp256K1SignerFromKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer from private key: %w", err)
	}
	return NewP2pkhAuthProofSignature(txo, signer)
}

// NewP2pkhFeeSignatureFromKey creates a standard P2PKH fee predicate signature for FeeProof.
func NewP2pkhFeeSignatureFromKey(txo *types.TransactionOrder, privKey []byte) ([]byte, error) {
	signer, err := crypto.NewInMemorySecp256K1SignerFromKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer from private key: %w", err)
	}
	return NewP2pkhFeeProofSignature(txo, signer)
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
