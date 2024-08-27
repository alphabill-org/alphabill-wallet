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

func (w *Wallet) CollectDust(ctx context.Context, accountNumber uint64, allowedTokenTypes []sdktypes.TokenTypeID, ownerPredicateInput *PredicateInput, typeOwnerPredicateInputs []*PredicateInput) (map[uint64][]*SubmissionResult, error) {
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
			subResult, err := w.collectDust(ctx, key, tokenz, ownerPredicateInput, typeOwnerPredicateInputs)
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

func (w *Wallet) collectDust(ctx context.Context, acc *accountKey, tokens []*sdktypes.FungibleToken, ownerPredicateInput *PredicateInput, typeOwnerPredicateInputs []*PredicateInput) (*SubmissionResult, error) {
	batchCount := ((len(tokens) - 1) / maxBurnBatchSize) + 1
	txCount := len(tokens) + batchCount*2 // +lock fee and join fee for every batch
	fcrID, err := w.ensureFeeCredit(ctx, acc.AccountKey, txCount)
	if err != nil {
		return nil, err
	}
	// first token to be joined into
	targetToken := tokens[0]
	totalAmountJoined := targetToken.Amount
	burnTokens := tokens[1:]
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
		lockFee, err = w.lockTokenForDC(ctx, acc, fcrID, targetToken, ownerPredicateInput)
		if err != nil {
			return nil, fmt.Errorf("failed to lock target token: %w", err)
		}

		targetToken.Counter += 1
		burnBatchAmount, burnFee, proofs, err := w.burnTokensForDC(ctx, acc, burnBatch, targetToken, fcrID, ownerPredicateInput, typeOwnerPredicateInputs)
		if err != nil {
			return nil, err
		}

		// if there's more to burn, update counter to continue
		var joinFee uint64
		joinFee, err = w.joinTokenForDC(ctx, acc, proofs, targetToken, fcrID, ownerPredicateInput, typeOwnerPredicateInputs)
		if err != nil {
			return nil, err
		}
		targetToken.Counter += 1

		totalAmountJoined += burnBatchAmount
		totalFees += lockFee + burnFee + joinFee
	}
	return &SubmissionResult{FeeSum: totalFees}, nil
}

func (w *Wallet) joinTokenForDC(ctx context.Context, acc *accountKey, burnProofs []*sdktypes.Proof, targetToken *sdktypes.FungibleToken, fcrID types.UnitID, ownerPredicateInput *PredicateInput, typeOwnerPredicateInputs []*PredicateInput) (uint64, error) {
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
	roundNumber, err := w.GetRoundNumber(ctx)
	if err != nil {
		return 0, err
	}

	tx, err := targetToken.Join(burnTxs, burnTxProofs,
		sdktypes.WithTimeout(roundNumber+txTimeoutRoundCount),
		sdktypes.WithFeeCreditRecordID(fcrID),
		sdktypes.WithMaxFee(w.maxFee),
	)
	if err != nil {
		return 0, err
	}

	payloadBytes, err := tx.PayloadBytes()
	if err != nil {
		return 0, err
	}
	typeOwnerPredicateSignatures, err := newPredicateSignatures(payloadBytes, typeOwnerPredicateInputs)
	if err != nil {
		return 0, err
	}
	ownerPredicateSignature, err := ownerPredicateInput.PredicateSignature(payloadBytes)
	if err != nil {
		return 0, err
	}
	err = tx.SetAuthProof(tokens.JoinFungibleTokenAuthProof{
		OwnerPredicateSignature:           ownerPredicateSignature,
		TokenTypeOwnerPredicateSignatures: typeOwnerPredicateSignatures,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to set auth proof: %w", err)
	}
	tx.FeeProof, err = sdktypes.NewP2pkhFeeSignatureFromKey(tx, acc.PrivKey)
	if err != nil {
		return 0, fmt.Errorf("failed to sign tx fee proof: %w", err)
	}

	sub := txsubmitter.New(tx)
	if err = sub.ToBatch(w.tokensClient, w.log).SendTx(ctx, true); err != nil {
		return 0, err
	}
	return sub.Proof.TxRecord.ServerMetadata.ActualFee, nil
}

func (w *Wallet) burnTokensForDC(ctx context.Context, acc *accountKey, tokensToBurn []*sdktypes.FungibleToken, targetToken *sdktypes.FungibleToken, fcrID types.UnitID, ownerPredicateInput *PredicateInput, typeOwnerPredicateInputs []*PredicateInput) (uint64, uint64, []*sdktypes.Proof, error) {
	burnBatch := txsubmitter.NewBatch(w.tokensClient, w.log)
	burnBatchAmount := uint64(0)

	roundNumber, err := w.GetRoundNumber(ctx)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("failed to get round number: %w", err)
	}
	for _, token := range tokensToBurn {
		burnBatchAmount += token.Amount
		tx, err := token.Burn(targetToken.ID, targetToken.Counter,
			sdktypes.WithTimeout(roundNumber+txTimeoutRoundCount),
			sdktypes.WithFeeCreditRecordID(fcrID),
			sdktypes.WithMaxFee(w.maxFee),
		)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("failed to prepare burn tx: %w", err)
		}

		payloadBytes, err := tx.PayloadBytes()
		if err != nil {
			return 0, 0, nil, err
		}
		typeOwnerPredicateSignatures, err := newPredicateSignatures(payloadBytes, typeOwnerPredicateInputs)
		if err != nil {
			return 0, 0, nil, err
		}
		ownerPredicateSignature, err := ownerPredicateInput.PredicateSignature(payloadBytes)
		if err != nil {
			return 0, 0, nil, err
		}
		err = tx.SetAuthProof(tokens.BurnFungibleTokenAuthProof{
			OwnerPredicateSignature:           ownerPredicateSignature,
			TokenTypeOwnerPredicateSignatures: typeOwnerPredicateSignatures,
		})
		if err != nil {
			return 0, 0, nil, fmt.Errorf("failed to set auth proof: %w", err)
		}
		tx.FeeProof, err = sdktypes.NewP2pkhFeeSignatureFromKey(tx, acc.PrivKey)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("failed to sign tx fee proof: %w", err)
		}

		burnBatch.Add(txsubmitter.New(tx))
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

func (w *Wallet) getTokensForDC(ctx context.Context, key sdktypes.PubKey, allowedTokenTypes []sdktypes.TokenTypeID) (map[string][]*sdktypes.FungibleToken, error) {
	// find tokens to join
	allTokens, err := w.tokensClient.GetFungibleTokens(ctx, key.Hash())
	if err != nil {
		return nil, err
	}
	// group tokens by type
	var tokensByTypes = make(map[string][]*sdktypes.FungibleToken, len(allowedTokenTypes))
	for _, tokenType := range allowedTokenTypes {
		tokensByTypes[string(tokenType)] = make([]*sdktypes.FungibleToken, 0)
	}
	for _, tok := range allTokens {
		typeID := string(tok.TypeID)
		tokenz, found := tokensByTypes[typeID]
		if !found && len(allowedTokenTypes) > 0 {
			// if filter is set, skip tokens of other types
			continue
		}
		if tok.LockStatus != 0 {
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

func (w *Wallet) lockTokenForDC(ctx context.Context, acc *accountKey, fcrID types.UnitID, targetToken Token, ownerPredicateInput *PredicateInput) (uint64, error) {
	roundNumber, err := w.GetRoundNumber(ctx)
	if err != nil {
		return 0, err
	}
	tx, err := targetToken.Lock(wallet.LockReasonCollectDust,
		sdktypes.WithTimeout(roundNumber+txTimeoutRoundCount),
		sdktypes.WithFeeCreditRecordID(fcrID),
		sdktypes.WithMaxFee(w.maxFee),
	)
	if err != nil {
		return 0, err
	}

	payloadBytes, err := tx.PayloadBytes()
	if err != nil {
		return 0, err
	}
	ownerPredicateSignature, err := ownerPredicateInput.PredicateSignature(payloadBytes)
	if err != nil {
		return 0, err
	}
	err = tx.SetAuthProof(tokens.LockTokenAuthProof{
		OwnerPredicateSignature: ownerPredicateSignature,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to set auth proof: %w", err)
	}
	tx.FeeProof, err = sdktypes.NewP2pkhFeeSignatureFromKey(tx, acc.PrivKey)
	if err != nil {
		return 0, fmt.Errorf("failed to sign tx fee proof: %w", err)
	}

	sub := txsubmitter.New(tx)
	if err = sub.ToBatch(w.tokensClient, w.log).SendTx(ctx, true); err != nil {
		return 0, err
	}
	return sub.Proof.TxRecord.ServerMetadata.ActualFee, nil
}
