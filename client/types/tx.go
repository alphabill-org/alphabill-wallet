package types

import (
	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/types"
)

type (
	Options struct {
		Timeout              uint64
		FeeCreditRecordID    types.UnitID
		MaxFee               uint64
		ReferenceNumber      []byte
		OwnerProofGenerator  types.ProofGenerator
		FeeProofGenerator    types.ProofGenerator
		ExtraProofGenerators []types.ProofGenerator
	}

	Option func(*Options)
)

func NewTransactionOrder(payload *types.Payload) *types.TransactionOrder {
	return &types.TransactionOrder{
		Payload:    payload,
		OwnerProof: nil,
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

func GenerateAndSetProofs(tx *types.TransactionOrder, attr any, attrField *[][]byte, opts ...Option) error {
	o := OptionsWithDefaults(opts)

	if o.OwnerProofGenerator != nil {
		if err := tx.SetOwnerProof(o.OwnerProofGenerator); err != nil {
			return err
		}
	}

	if o.ExtraProofGenerators != nil && attr != nil {
		proofs, err := generateProofs(tx, o.ExtraProofGenerators)
		if err != nil {
			return err
		}

		*attrField = proofs
		if err = tx.Payload.SetAttributes(attr); err != nil {
			return err
		}
	}

	if o.FeeProofGenerator != nil {
		if err := tx.SetFeeProof(o.FeeProofGenerator); err != nil {
			return err
		}
	}

	return nil
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

func WithOwnerProof(proofGenerator types.ProofGenerator) Option {
	return func(os *Options) {
		os.OwnerProofGenerator = proofGenerator
	}
}

func WithFeeProof(proofGenerator types.ProofGenerator) Option {
	return func(os *Options) {
		os.FeeProofGenerator = proofGenerator
	}
}

func WithExtraProofs(proofGenerators []types.ProofGenerator) Option {
	return func(os *Options) {
		os.ExtraProofGenerators = proofGenerators
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

func NewP2pkhProofGenerator(privKey []byte, pubKey []byte) types.ProofGenerator {
	return func(payloadBytes []byte) ([]byte, error) {
		sig, err := SignBytes(payloadBytes, privKey)
		if err != nil {
			return nil, err
		}
		return templates.NewP2pkh256SignatureBytes(sig, pubKey), nil
	}
}

// SignBytes signs the given bytes with the given key.
func SignBytes(bytes []byte, signingPrivateKey []byte) ([]byte, error) {
	signer, err := crypto.NewInMemorySecp256K1SignerFromKey(signingPrivateKey)
	if err != nil {
		return nil, err
	}
	sig, err := signer.SignBytes(bytes)
	if err != nil {
		return nil, err
	}
	return sig, nil
}

func generateProofs(tx *types.TransactionOrder, proofGenerators []types.ProofGenerator) ([][]byte, error) {
	payloadBytes, err := tx.PayloadBytes()
	if err != nil {
		return nil, err
	}

	proofs := make([][]byte, 0, len(proofGenerators))
	for _, proofGenerator := range proofGenerators {
		proof, err := proofGenerator(payloadBytes)
		if err != nil {
			return nil, err
		}
		proofs = append(proofs, proof)
	}

	return proofs, nil
}
