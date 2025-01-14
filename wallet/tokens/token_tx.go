package tokens

import (
	"context"
	"fmt"
	"sort"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
)

const (
	txTimeoutRoundCount = 10
)

func ownerPredicateFromHash(receiverPubKeyHash []byte) sdktypes.Predicate {
	var bytes []byte
	if receiverPubKeyHash != nil {
		bytes = templates.NewP2pkh256BytesFromKeyHash(receiverPubKeyHash)
	} else {
		bytes = templates.AlwaysTrueBytes()
	}
	return bytes
}

func OwnerPredicateFromPubKey(receiverPubKey sdktypes.PubKey) sdktypes.Predicate {
	var h []byte
	if receiverPubKey != nil {
		h = hash.Sum256(receiverPubKey)
	}
	return ownerPredicateFromHash(h)
}

// assumes there's sufficient balance for the given amount, sends transactions immediately
func (w *Wallet) doSendMultiple(ctx context.Context, amount uint64, tokens []*sdktypes.FungibleToken, acc *accountKey, fcrID, receiverPubKey []byte, ownerProof *PredicateInput, typeOwnerPredicateInputs []*PredicateInput) (*SubmissionResult, error) {
	var accumulatedSum uint64
	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].Amount > tokens[j].Amount
	})

	batch := txsubmitter.NewBatch(w.tokensClient, w.log)
	roundNumber, err := w.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}

	for _, t := range tokens {
		remainingAmount := amount - accumulatedSum
		sub, err := w.prepareSplitOrTransferTx(acc, remainingAmount, t, fcrID, receiverPubKey, roundNumber+txTimeoutRoundCount, ownerProof, typeOwnerPredicateInputs)
		if err != nil {
			return nil, err
		}
		batch.Add(sub)
		accumulatedSum += t.Amount
		if accumulatedSum >= amount {
			break
		}
	}
	if err = batch.SendTx(ctx, w.confirmTx); err != nil {
		return nil, err
	}
	var feeSum uint64
	for _, sub := range batch.Submissions() {
		if sub.Confirmed() {
			feeSum += sub.ActualFee()
		}
	}
	return &SubmissionResult{Submissions: batch.Submissions(), FeeSum: feeSum, AccountNumber: acc.AccountNumber()}, nil
}

func (w *Wallet) prepareSplitOrTransferTx(acc *accountKey, amount uint64, ft *sdktypes.FungibleToken, fcrID, receiverPubKey []byte, timeout uint64, ownerPredicateInput *PredicateInput, typeOwnerPredicateInputs []*PredicateInput) (*txsubmitter.TxSubmission, error) {
	if amount >= ft.Amount {
		tx, err := ft.Transfer(OwnerPredicateFromPubKey(receiverPubKey),
			sdktypes.WithTimeout(timeout),
			sdktypes.WithFeeCreditRecordID(fcrID),
			sdktypes.WithMaxFee(w.maxFee),
		)
		if err != nil {
			return nil, err
		}

		payloadBytes, err := tx.AuthProofSigBytes()
		if err != nil {
			return nil, err
		}
		typeOwnerProofs, err := newProofs(payloadBytes, typeOwnerPredicateInputs)
		if err != nil {
			return nil, err
		}
		ownerProof, err := ownerPredicateInput.Proof(payloadBytes)
		if err != nil {
			return nil, err
		}
		err = tx.SetAuthProof(tokens.TransferFungibleTokenAuthProof{
			OwnerProof:           ownerProof,
			TokenTypeOwnerProofs: typeOwnerProofs,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to set auth proof: %w", err)
		}
		tx.FeeProof, err = sdktypes.NewP2pkhFeeSignatureFromKey(tx, acc.PrivKey)
		if err != nil {
			return nil, fmt.Errorf("failed to sign tx fee proof: %w", err)
		}
		return txsubmitter.New(tx)
	} else {
		tx, err := ft.Split(amount, OwnerPredicateFromPubKey(receiverPubKey),
			sdktypes.WithTimeout(timeout),
			sdktypes.WithFeeCreditRecordID(fcrID),
			sdktypes.WithMaxFee(w.maxFee),
		)
		if err != nil {
			return nil, err
		}
		payloadBytes, err := tx.AuthProofSigBytes()
		if err != nil {
			return nil, err
		}
		typeOwnerProofs, err := newProofs(payloadBytes, typeOwnerPredicateInputs)
		if err != nil {
			return nil, err
		}
		ownerProof, err := ownerPredicateInput.Proof(payloadBytes)
		if err != nil {
			return nil, err
		}
		err = tx.SetAuthProof(tokens.SplitFungibleTokenAuthProof{
			OwnerProof:           ownerProof,
			TokenTypeOwnerProofs: typeOwnerProofs,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to set auth proof: %w", err)
		}
		tx.FeeProof, err = sdktypes.NewP2pkhFeeSignatureFromKey(tx, acc.PrivKey)
		if err != nil {
			return nil, fmt.Errorf("failed to sign tx fee proof: %w", err)
		}
		return txsubmitter.New(tx)
	}
}
