package tx

import (
	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/types"
)

type (
	options struct {
		timeout              uint64
		feeCreditRecordID    types.UnitID
		maxFee               uint64
		referenceNumber      []byte
		ownerProofGenerator  types.ProofGenerator
		feeProofGenerator    types.ProofGenerator
		extraProofGenerators []types.ProofGenerator
	}

	Option func(*options)
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

	o := optionsWithDefaults(opts)
	return &types.Payload{
		SystemID:   systemID,
		Type:       txType,
		UnitID:     unitID,
		Attributes: attrBytes,
		ClientMetadata: &types.ClientMetadata{
			Timeout:           o.timeout,
			MaxTransactionFee: o.maxFee,
			FeeCreditRecordID: o.feeCreditRecordID,
			ReferenceNumber:   o.referenceNumber,
		},
	}, nil
}

func GenerateAndSetProofs(tx *types.TransactionOrder, attr any, attrField *[][]byte, opts ...Option) error {
	o := optionsWithDefaults(opts)

	if o.ownerProofGenerator != nil {
		if err := tx.SetOwnerProof(o.ownerProofGenerator); err != nil {
			return err
		}
	}

	if o.extraProofGenerators != nil && attr != nil {
		proofs, err := generateProofs(tx, o.extraProofGenerators)
		if err != nil {
			return err
		}

		*attrField = proofs
		if err = tx.Payload.SetAttributes(attr); err != nil {
			return err
		}
	}

	if o.feeProofGenerator != nil {
		if err := tx.SetFeeProof(o.feeProofGenerator); err != nil {
			return err
		}
	}

	return nil
}

func WithReferenceNumber(referenceNumber []byte) Option {
	return func(os *options) {
		os.referenceNumber = referenceNumber
	}
}

func WithTimeout(timeout uint64) Option {
	return func(os *options) {
		os.timeout = timeout
	}
}

func WithFeeCreditRecordID(feeCreditRecordID types.UnitID) Option {
	return func(os *options) {
		os.feeCreditRecordID = feeCreditRecordID
	}
}

func WithMaxFee(maxFee uint64) Option {
	return func(os *options) {
		os.maxFee = maxFee
	}
}

func WithOwnerProof(proofGenerator types.ProofGenerator) Option {
	return func(os *options) {
		os.ownerProofGenerator = proofGenerator
	}
}

func WithFeeProof(proofGenerator types.ProofGenerator) Option {
	return func(os *options) {
		os.feeProofGenerator = proofGenerator
	}
}

func WithExtraProofs(proofGenerators []types.ProofGenerator) Option {
	return func(os *options) {
		os.extraProofGenerators = proofGenerators
	}
}

func optionsWithDefaults(txOptions []Option) *options {
	opts := &options{
		maxFee: 10,
	}
	for _, txOption := range txOptions {
		txOption(opts)
	}
	return opts
}

func NewP2pkhProofGenerator(pubKey []byte, privKey []byte) types.ProofGenerator {
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
