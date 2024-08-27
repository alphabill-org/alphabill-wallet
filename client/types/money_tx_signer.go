package types

import (
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
)

// MoneyTxSigner helper struct to generate standard transaction signatures.
type MoneyTxSigner struct {
	signer abcrypto.Signer
}

func NewMoneyTxSigner(signer abcrypto.Signer) (*MoneyTxSigner, error) {
	if signer == nil {
		return nil, errors.New("signer is nil")
	}
	return &MoneyTxSigner{signer: signer}, nil
}

func NewMoneyTxSignerFromKey(privKey []byte) (*MoneyTxSigner, error) {
	signer, err := abcrypto.NewInMemorySecp256K1SignerFromKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create tx signer: %w", err)
	}
	return NewMoneyTxSigner(signer)
}

// SignTx generates transaction specific P2PKH AuthProof and FeeProof.
func (s *MoneyTxSigner) SignTx(tx *types.TransactionOrder) error {
	if err := s.AddAuthProof(tx); err != nil {
		return fmt.Errorf("failed to add auth proof: %w", err)
	}
	if err := s.AddFeeProof(tx); err != nil {
		return fmt.Errorf("failed to add fee proof: %w", err)
	}
	return nil
}

func (s *MoneyTxSigner) AddAuthProof(tx *types.TransactionOrder) error {
	ownerProof, err := NewPp2khSignature(tx, s.signer)
	if err != nil {
		return fmt.Errorf("failed to create owner proof: %w", err)
	}
	authProof, err := s.newAuthProof(tx, ownerProof)
	if err != nil {
		return fmt.Errorf("failed to create auth proof struct: %w", err)
	}
	if err = tx.SetAuthProof(authProof); err != nil {
		return fmt.Errorf("failed to set auth proof: %w", err)
	}
	return nil
}

func (s *MoneyTxSigner) AddFeeProof(tx *types.TransactionOrder) error {
	feeProof, err := NewP2pkhFeeSignature(tx, s.signer)
	if err != nil {
		return fmt.Errorf("failed to create fee proof: %w", err)
	}
	tx.FeeProof = feeProof
	return nil
}

func (s *MoneyTxSigner) newAuthProof(tx *types.TransactionOrder, ownerProof []byte) (any, error) {
	switch tx.PayloadType() {
	case money.PayloadTypeTransfer:
		return money.TransferAuthProof{OwnerProof: ownerProof}, nil
	case money.PayloadTypeSplit:
		return money.SplitAuthProof{OwnerProof: ownerProof}, nil
	case money.PayloadTypeTransDC:
		return money.TransferDCAuthProof{OwnerProof: ownerProof}, nil
	case money.PayloadTypeSwapDC:
		return money.SwapDCAuthProof{OwnerProof: ownerProof}, nil
	case money.PayloadTypeLock:
		return money.LockAuthProof{OwnerProof: ownerProof}, nil
	case money.PayloadTypeUnlock:
		return money.UnlockAuthProof{OwnerProof: ownerProof}, nil
	default:
		return nil, fmt.Errorf("unsupported payload type: %s", tx.PayloadType())
	}
}
