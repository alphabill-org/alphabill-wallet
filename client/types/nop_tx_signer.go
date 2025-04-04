package types

import (
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/txsystem/nop"
	"github.com/alphabill-org/alphabill-go-base/types"
)

// NopTxSigner helper struct to sign "nop" transaction with standard predicates.
type NopTxSigner struct {
	signer abcrypto.Signer
}

func NewNopTxSigner(signer abcrypto.Signer) (*NopTxSigner, error) {
	if signer == nil {
		return nil, errors.New("signer is nil")
	}
	return &NopTxSigner{signer: signer}, nil
}

func NewNopTxSignerFromKey(privKey []byte) (*NopTxSigner, error) {
	signer, err := abcrypto.NewInMemorySecp256K1SignerFromKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create tx signer: %w", err)
	}
	return NewNopTxSigner(signer)
}

// SignTx generates transaction specific P2PKH AuthProof and FeeProof.
func (s *NopTxSigner) SignTx(tx *types.TransactionOrder) error {
	if err := s.AddAuthProof(tx); err != nil {
		return fmt.Errorf("failed to add auth proof: %w", err)
	}
	if err := s.AddFeeProof(tx); err != nil {
		return fmt.Errorf("failed to add fee proof: %w", err)
	}
	return nil
}

// SignCommitTx generates transaction specific P2PKH StateUnlockProof for commit, and AuthProof and FeeProof.
func (s *NopTxSigner) SignCommitTx(tx *types.TransactionOrder) error {
	unlockProof, err := NewP2pkhStateLockProofSignature(tx, s.signer)
	if err != nil {
		return fmt.Errorf("failed to create state unlock proof: %w", err)
	}
	tx.AddStateUnlockCommitProof(unlockProof)
	return s.SignTx(tx)
}

// SignRollbackTx generates transaction specific P2PKH StateUnlockProof for rollback, and AuthProof and FeeProof.
func (s *NopTxSigner) SignRollbackTx(tx *types.TransactionOrder) error {
	unlockProof, err := NewP2pkhStateLockProofSignature(tx, s.signer)
	if err != nil {
		return fmt.Errorf("failed to create state unlock proof: %w", err)
	}
	tx.AddStateUnlockRollbackProof(unlockProof)
	return s.SignTx(tx)
}

func (s *NopTxSigner) AddAuthProof(tx *types.TransactionOrder) error {
	ownerProof, err := NewP2pkhAuthProofSignature(tx, s.signer)
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

func (s *NopTxSigner) AddFeeProof(tx *types.TransactionOrder) error {
	feeProof, err := NewP2pkhFeeProofSignature(tx, s.signer)
	if err != nil {
		return fmt.Errorf("failed to create fee proof: %w", err)
	}
	tx.FeeProof = feeProof
	return nil
}

func (s *NopTxSigner) newAuthProof(tx *types.TransactionOrder, ownerProof []byte) (any, error) {
	if tx.Type != nop.TransactionTypeNOP {
		return nil, fmt.Errorf("unsupported transaction type: %d", tx.Type)
	}
	return nop.AuthProof{OwnerProof: ownerProof}, nil
}

func (s *NopTxSigner) Signer() abcrypto.Signer {
	return s.signer
}
