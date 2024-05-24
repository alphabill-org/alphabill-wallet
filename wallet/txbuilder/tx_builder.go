package txbuilder

import (
	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/types"
)

const MaxFee = uint64(1)

// NewTxPayload creates a new transaction payload.
func NewTxPayload(systemID types.SystemID, txType string, unitID, fcrID types.UnitID, timeout uint64, refNo []byte, attr interface{}) (*types.Payload, error) {
	attrBytes, err := types.Cbor.Marshal(attr)
	if err != nil {
		return nil, err
	}
	return &types.Payload{
		SystemID:   systemID,
		Type:       txType,
		UnitID:     unitID,
		Attributes: attrBytes,
		ClientMetadata: &types.ClientMetadata{
			Timeout:           timeout,
			MaxTransactionFee: MaxFee,
			FeeCreditRecordID: fcrID,
			ReferenceNumber:   refNo,
		},
	}, nil
}

// SignPayload signs transaction payload.
func SignPayload(payload *types.Payload, signingPrivateKey []byte) ([]byte, error) {
	signer, err := crypto.NewInMemorySecp256K1SignerFromKey(signingPrivateKey)
	if err != nil {
		return nil, err
	}
	payloadBytes, err := payload.Bytes()
	if err != nil {
		return nil, err
	}
	payloadSig, err := signer.SignBytes(payloadBytes)
	if err != nil {
		return nil, err
	}
	return payloadSig, nil
}

// NewTransactionOrderP2PKH returns P2PKH transaction order.
func NewTransactionOrderP2PKH(payload *types.Payload, payloadSig []byte, signingPublicKey []byte) *types.TransactionOrder {
	return &types.TransactionOrder{
		Payload:    payload,
		OwnerProof: templates.NewP2pkh256SignatureBytes(payloadSig, signingPublicKey),
	}
}
