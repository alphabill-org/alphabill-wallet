package tokens

import (
	"context"
	"sort"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"

	"github.com/alphabill-org/alphabill-wallet/client/tx"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
)

const (
	txTimeoutRoundCount = 10
)

func bearerPredicateFromHash(receiverPubKeyHash []byte) sdktypes.Predicate {
	var bytes []byte
	if receiverPubKeyHash != nil {
		bytes = templates.NewP2pkh256BytesFromKeyHash(receiverPubKeyHash)
	} else {
		bytes = templates.AlwaysTrueBytes()
	}
	return bytes
}

func BearerPredicateFromPubKey(receiverPubKey sdktypes.PubKey) sdktypes.Predicate {
	var h []byte
	if receiverPubKey != nil {
		h = hash.Sum256(receiverPubKey)
	}
	return bearerPredicateFromHash(h)
}

// assumes there's sufficient balance for the given amount, sends transactions immediately
func (w *Wallet) doSendMultiple(ctx context.Context, amount uint64, tokens []*sdktypes.FungibleToken, acc *accountKey, fcrID, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput, ownerProof *PredicateInput) (*SubmissionResult, error) {
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
		sub, err := w.prepareSplitOrTransferTx(ctx, acc, remainingAmount, t, fcrID, receiverPubKey, invariantPredicateArgs, roundNumber+txTimeoutRoundCount, ownerProof)
		if err != nil {
			return nil, err
		}
		batch.Add(sub)
		accumulatedSum += t.Amount
		if accumulatedSum >= amount {
			break
		}
	}
	err = batch.SendTx(ctx, w.confirmTx)
	feeSum := uint64(0)
	for _, sub := range batch.Submissions() {
		if sub.Confirmed() {
			feeSum += sub.Proof.TxRecord.ServerMetadata.ActualFee
		}
	}
	return &SubmissionResult{Submissions: batch.Submissions(), FeeSum: feeSum, AccountNumber: acc.AccountNumber()}, err
}

func (w *Wallet) prepareSplitOrTransferTx(ctx context.Context, acc *accountKey, amount uint64, ft *sdktypes.FungibleToken, fcrID, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput, timeout uint64, ownerProof *PredicateInput) (*txsubmitter.TxSubmission, error) {
	if amount >= ft.Amount {
		tx, err := ft.Transfer(BearerPredicateFromPubKey(receiverPubKey),
			tx.WithTimeout(timeout),
			tx.WithFeeCreditRecordID(fcrID),
			tx.WithOwnerProof(newProofGenerator(ownerProof)),
			tx.WithFeeProof(newProofGenerator(defaultProof(acc.AccountKey))),
			tx.WithExtraProofs(newProofGenerators(invariantPredicateArgs)))
		if err != nil {
			return nil, err
		}
		return 	txsubmitter.New(tx), nil
	} else {
		tx, err := ft.Split(amount, BearerPredicateFromPubKey(receiverPubKey),
			tx.WithTimeout(timeout),
			tx.WithFeeCreditRecordID(fcrID),
			tx.WithOwnerProof(newProofGenerator(ownerProof)),
			tx.WithFeeProof(newProofGenerator(defaultProof(acc.AccountKey))),
			tx.WithExtraProofs(newProofGenerators(invariantPredicateArgs)))
		if err != nil {
			return nil, err
		}
		return 	txsubmitter.New(tx), nil
	}
}
