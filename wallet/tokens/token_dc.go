package tokens

import (
	"context"
	"fmt"
	"sort"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
)

const maxBurnBatchSize = 100

func (w *Wallet) CollectDust(ctx context.Context, accountNumber uint64, allowedTokenTypes []sdktypes.TokenTypeID, invariantPredicateArgs []*PredicateInput, ownerProof *PredicateInput) (map[uint64][]*SubmissionResult, error) {
	keys, err := w.getAccounts(accountNumber)
	if err != nil {
		return nil, err
	}
	results := make(map[uint64][]*SubmissionResult, len(keys))

	for _, key := range keys {
		tokensByTypes, err := w.getTokensForDC(ctx, key.PubKey, allowedTokenTypes)
		if err != nil {
			return nil, err
		}
		var subResults []*SubmissionResult
		for _, tokenz := range tokensByTypes {
			subResult, err := w.collectDust(ctx, key, tokenz, invariantPredicateArgs)
			if err != nil {
				return results, err
			}
			if subResult != nil {
				subResults = append(subResults, subResult)
			}
		}
		results[key.idx] = subResults
	}
	return results, nil
}

func (w *Wallet) collectDust(ctx context.Context, acc *accountKey, typedTokens []*sdktypes.TokenUnit, invariantPredicateArgs []*PredicateInput) (*SubmissionResult, error) {
	batchCount := ((len(typedTokens) - 1) / maxBurnBatchSize) + 1
	txCount := len(typedTokens) + batchCount*2 // +lock fee and join fee for every batch
	fcrID, err := w.ensureFeeCredit(ctx, acc.AccountKey, txCount)
	if err != nil {
		return nil, err
	}
	// first token to be joined into
	targetToken := typedTokens[0]
	targetTokenID := targetToken.ID
	targetTokenCounter := targetToken.Counter
	totalAmountJoined := targetToken.Amount
	burnTokens := typedTokens[1:]
	totalFees := uint64(0)

	for startIdx := 0; startIdx < len(burnTokens); startIdx += maxBurnBatchSize {
		endIdx := startIdx + maxBurnBatchSize
		if endIdx > len(burnTokens) {
			endIdx = len(burnTokens)
		}
		burnBatch := burnTokens[startIdx:endIdx]

		// check batch overflow before burning the tokens
		totalAmountToBeJoined := totalAmountJoined
		var err error
		for _, token := range burnBatch {
			totalAmountToBeJoined, _, err = util.AddUint64(totalAmountToBeJoined, token.Amount)
			if err != nil {
				w.log.WarnContext(ctx, fmt.Sprintf("unable to join tokens of type '%X', account key '0x%X': %v", token.TypeID, acc.PubKey, err))
				// just stop without returning error, so that we can continue with other token types
				if totalFees > 0 {
					return &SubmissionResult{FeeSum: totalFees}, nil
				}
				return nil, nil
			}
		}

		var lockFee uint64
		lockFee, err = w.lockTokenForDC(ctx, acc, fcrID, targetTokenID, targetTokenCounter, invariantPredicateArgs)
		if err != nil {
			return nil, fmt.Errorf("failed to lock target token: %w", err)
		}

		targetTokenCounter += 1
		burnBatchAmount, burnFee, proofs, err := w.burnTokensForDC(ctx, acc, burnBatch, targetTokenCounter, targetTokenID, fcrID, invariantPredicateArgs)
		if err != nil {
			return nil, err
		}

		// if there's more to burn, update counter to continue
		var joinFee uint64
		joinFee, err = w.joinTokenForDC(ctx, acc, proofs, targetTokenCounter, targetTokenID, fcrID, invariantPredicateArgs)
		if err != nil {
			return nil, err
		}
		targetTokenCounter += 1

		totalAmountJoined += burnBatchAmount
		totalFees += lockFee + burnFee + joinFee
	}
	return &SubmissionResult{FeeSum: totalFees}, nil
}

func (w *Wallet) joinTokenForDC(ctx context.Context, acc *accountKey, burnProofs []*sdktypes.Proof, targetTokenCounter uint64, targetTokenID, fcrID types.UnitID, invariantPredicateArgs []*PredicateInput) (uint64, error) {
	// explicitly sort proofs by unit ids in increasing order
	sort.Slice(burnProofs, func(i, j int) bool {
		a := burnProofs[i].TxRecord.TransactionOrder.UnitID()
		b := burnProofs[j].TxRecord.TransactionOrder.UnitID()
		return a.Compare(b) < 0
	})
	burnTxs := make([]*types.TransactionRecord, len(burnProofs))
	burnTxProofs := make([]*types.TxProof, len(burnProofs))
	for i, proof := range burnProofs {
		burnTxs[i] = proof.TxRecord
		burnTxProofs[i] = proof.TxProof
	}

	joinAttrs := &tokens.JoinFungibleTokenAttributes{
		BurnTransactions:             burnTxs,
		Proofs:                       burnTxProofs,
		Counter:                      targetTokenCounter,
		InvariantPredicateSignatures: nil,
	}

	sub, err := w.prepareTxSubmission(ctx, acc, tokens.PayloadTypeJoinFungibleToken, joinAttrs, targetTokenID, fcrID, w.GetRoundNumber, defaultOwnerProof(acc.AccountNumber()), func(tx *types.TransactionOrder) error {
		signatures, err := preparePredicateSignatures(w.GetAccountManager(), invariantPredicateArgs, tx, joinAttrs)
		if err != nil {
			return err
		}
		joinAttrs.SetInvariantPredicateSignatures(signatures)
		tx.Payload.Attributes, err = types.Cbor.Marshal(joinAttrs)
		return err
	})
	if err != nil {
		return 0, err
	}
	if err = sub.ToBatch(w.tokensClient, w.log).SendTx(ctx, true); err != nil {
		return 0, err
	}
	return sub.Proof.TxRecord.ServerMetadata.ActualFee, nil
}

func (w *Wallet) burnTokensForDC(ctx context.Context, acc *accountKey, tokensToBurn []*sdktypes.TokenUnit, targetTokenCounter uint64, targetTokenID, fcrID types.UnitID, invariantPredicateArgs []*PredicateInput) (uint64, uint64, []*sdktypes.Proof, error) {
	burnBatch := txsubmitter.NewBatch(w.tokensClient, w.log)
	rnFetcher := &cachingRoundNumberFetcher{delegate: w.GetRoundNumber}
	burnBatchAmount := uint64(0)

	for _, token := range tokensToBurn {
		burnBatchAmount += token.Amount
		attrs := newBurnTxAttrs(token, targetTokenCounter, targetTokenID)
		sub, err := w.prepareTxSubmission(ctx, acc, tokens.PayloadTypeBurnFungibleToken, attrs, token.ID, fcrID, rnFetcher.getRoundNumber, defaultOwnerProof(acc.AccountNumber()), func(tx *types.TransactionOrder) error {
			signatures, err := preparePredicateSignatures(w.GetAccountManager(), invariantPredicateArgs, tx, attrs)
			if err != nil {
				return err
			}
			attrs.SetInvariantPredicateSignatures(signatures)
			tx.Payload.Attributes, err = types.Cbor.Marshal(attrs)
			return err
		})
		if err != nil {
			return 0, 0, nil, fmt.Errorf("failed to prepare burn tx: %w", err)
		}
		burnBatch.Add(sub)
	}

	if err := burnBatch.SendTx(ctx, true); err != nil {
		return 0, 0, nil, fmt.Errorf("failed to send burn tx: %w", err)
	}

	proofs := make([]*sdktypes.Proof, 0, len(burnBatch.Submissions()))
	feeSum := uint64(0)
	for _, sub := range burnBatch.Submissions() {
		proofs = append(proofs, sub.Proof)
		feeSum += sub.Proof.TxRecord.ServerMetadata.ActualFee
	}
	return burnBatchAmount, feeSum, proofs, nil
}

func (w *Wallet) getTokensForDC(ctx context.Context, key sdktypes.PubKey, allowedTokenTypes []sdktypes.TokenTypeID) (map[string][]*sdktypes.TokenUnit, error) {
	// find tokens to join
	allTokens, err := w.getTokens(ctx, sdktypes.Fungible, key.Hash())
	if err != nil {
		return nil, err
	}
	// group tokens by type
	var tokensByTypes = make(map[string][]*sdktypes.TokenUnit, len(allowedTokenTypes))
	for _, tokenType := range allowedTokenTypes {
		tokensByTypes[string(tokenType)] = make([]*sdktypes.TokenUnit, 0)
	}
	for _, tok := range allTokens {
		typeID := string(tok.TypeID)
		tokenz, found := tokensByTypes[typeID]
		if !found && len(allowedTokenTypes) > 0 {
			// if filter is set, skip tokens of other types
			continue
		}
		if tok.IsLocked() {
			continue
		}
		tokensByTypes[typeID] = append(tokenz, tok)
	}
	for k, v := range tokensByTypes {
		if len(v) < 2 { // not interested if tokens count is less than two
			delete(tokensByTypes, k)
		}
	}
	return tokensByTypes, nil
}

func (w *Wallet) lockTokenForDC(ctx context.Context, acc *accountKey, fcrID types.UnitID, targetTokenID types.UnitID, targetTokenCounter uint64, invariantPredicateArgs []*PredicateInput) (uint64, error) {
	attr := &tokens.LockTokenAttributes{
		LockStatus: wallet.LockReasonCollectDust,
		Counter:    targetTokenCounter,
	}

	sub, err := w.prepareTxSubmission(ctx, acc, tokens.PayloadTypeLockToken, attr, targetTokenID, fcrID, w.GetRoundNumber, defaultOwnerProof(acc.AccountNumber()), func(tx *types.TransactionOrder) error {
		signatures, err := preparePredicateSignatures(w.GetAccountManager(), invariantPredicateArgs, tx, attr)
		if err != nil {
			return err
		}
		attr.InvariantPredicateSignatures = signatures
		tx.Payload.Attributes, err = types.Cbor.Marshal(attr)
		return err
	})
	if err != nil {
		return 0, err
	}

	if err = sub.ToBatch(w.tokensClient, w.log).SendTx(ctx, true); err != nil {
		return 0, err
	}
	return sub.Proof.TxRecord.ServerMetadata.ActualFee, nil
}
