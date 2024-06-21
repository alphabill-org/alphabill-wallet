package tokens

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"sort"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/txbuilder"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
)

const (
	txTimeoutRoundCount = 10
)

type (
	txPreprocessor     func(tx *types.TransactionOrder) error
	roundNumberFetcher func(ctx context.Context) (uint64, error)

	cachingRoundNumberFetcher struct {
		roundNumber uint64
		delegate    roundNumberFetcher
	}
)

func (f *cachingRoundNumberFetcher) getRoundNumber(ctx context.Context) (uint64, error) {
	if f.roundNumber == 0 {
		var err error
		f.roundNumber, err = f.delegate(ctx)
		if err != nil {
			return 0, err
		}
	}
	return f.roundNumber, nil
}

func (w *Wallet) newType(ctx context.Context, accountNumber uint64, payloadType string, attrs AttrWithSubTypeCreationInputs, typeId sdktypes.TokenTypeID, subtypePredicateArgs []*PredicateInput) (*txsubmitter.TxSubmission, error) {
	acc, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}
	fcrID, err := w.ensureFeeCredit(ctx, acc.AccountKey, 1)
	if err != nil {
		return nil, err
	}
	sub, err := w.prepareTxSubmission(ctx, acc, payloadType, attrs, typeId, fcrID, w.GetRoundNumber, defaultOwnerProof(accountNumber), func(tx *types.TransactionOrder) error {
		signatures, err := preparePredicateSignatures(w.am, subtypePredicateArgs, tx, attrs)
		if err != nil {
			return err
		}
		attrs.SetSubTypeCreationPredicateSignatures(signatures)
		tx.Payload.Attributes, err = types.Cbor.Marshal(attrs)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err = sub.ToBatch(w.tokensClient, w.log).SendTx(ctx, w.confirmTx); err != nil {
		return nil, err
	}
	return sub, nil
}

/*
preparePredicateSignatures signs the transaction with the account key if the account number is greater than 0, otherwise it uses the provided argument as the signature.
*/
func preparePredicateSignatures(am account.Manager, args []*PredicateInput, tx *types.TransactionOrder, attrs types.SigBytesProvider) ([][]byte, error) {
	signatures := make([][]byte, 0, len(args))
	for _, input := range args {
		sig, err := preparePredicateSignature(am, input, tx, attrs)
		if err != nil {
			return nil, err
		}
		signatures = append(signatures, sig)
	}
	return signatures, nil
}

func preparePredicateSignature(am account.Manager, input *PredicateInput, tx *types.TransactionOrder, bp types.SigBytesProvider) ([]byte, error) {
	if input == nil {
		return nil, errors.New("nil predicate input")
	}
	if input.AccountNumber > 0 {
		ac, err := am.GetAccountKey(input.AccountNumber - 1)
		if err != nil {
			return nil, err
		}
		sig, err := signTx(ac, tx, bp)
		if err != nil {
			return nil, err
		}
		return sig, nil
	} else {
		return input.Argument, nil
	}
}

func (w *Wallet) newToken(ctx context.Context, accountNumber uint64, payloadType string, attrs MintAttr, mintPredicateArgs []*PredicateInput, idGenFunc func(shardPart []byte, unitPart []byte) types.UnitID) (*txsubmitter.TxSubmission, error) {
	key, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}
	fcrID, err := w.ensureFeeCredit(ctx, key.AccountKey, 1)
	if err != nil {
		return nil, err
	}
	sub, err := w.prepareTxSubmission(ctx, key, payloadType, attrs, nil, fcrID, w.GetRoundNumber, defaultOwnerProof(accountNumber), func(tx *types.TransactionOrder) error {
		// generate token identifier, needs to be generated before signatures
		unitPart, err := tokens.HashForNewTokenID(attrs, tx.Payload.ClientMetadata, crypto.SHA256)
		if err != nil {
			return err
		}
		tx.Payload.UnitID = idGenFunc(attrs.GetTypeID(), unitPart)

		signatures, err := preparePredicateSignatures(w.am, mintPredicateArgs, tx, attrs)
		if err != nil {
			return err
		}
		attrs.SetTokenCreationPredicateSignatures(signatures)
		tx.Payload.Attributes, err = types.Cbor.Marshal(attrs)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err = sub.ToBatch(w.tokensClient, w.log).SendTx(ctx, w.confirmTx); err != nil {
		return nil, err
	}
	return sub, nil
}

func (w *Wallet) prepareTxSubmission(ctx context.Context, acc *accountKey, payloadType string, attrs types.SigBytesProvider, unitID types.UnitID, fcrID types.UnitID, rn roundNumberFetcher, ownerProof *PredicateInput, txps txPreprocessor) (*txsubmitter.TxSubmission, error) {
	if unitID != nil {
		w.log.InfoContext(ctx, fmt.Sprintf("Preparing to send token tx, UnitID=%s", unitID))
	} else {
		w.log.InfoContext(ctx, fmt.Sprintf("Preparing to send %s token tx", payloadType))
	}

	roundNumber, err := rn(ctx)
	if err != nil {
		return nil, err
	}
	tx := createTx(w.systemID, payloadType, unitID, roundNumber+txTimeoutRoundCount, fcrID)
	if txps != nil {
		// set fields before tx is signed
		err = txps(tx)
		if err != nil {
			return nil, err
		}
	}

	am := w.GetAccountManager()
	tx.OwnerProof, err = preparePredicateSignature(am, ownerProof, tx, attrs)
	if err != nil {
		return nil, fmt.Errorf("failed to make owner proof: %w", err)
	}

	// TODO: AB-1016 remove when fixed
	tx.FeeProof, err = signTx(acc.AccountKey, tx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to make tx fee proof: %w", err)
	}

	return txsubmitter.New(tx), nil
}

func signTx(ac *account.AccountKey, tx *types.TransactionOrder, bp types.SigBytesProvider) (sdktypes.Predicate, error) {
	if ac == nil {
		return nil, nil
	}
	signer, err := abcrypto.NewInMemorySecp256K1SignerFromKey(ac.PrivKey)
	if err != nil {
		return nil, err
	}
	var bytes []byte
	if bp != nil {
		bytes, err = tx.Payload.BytesWithAttributeSigBytes(bp) // TODO: AB-1016
	} else {
		bytes, err = tx.Payload.Bytes()
	}
	if err != nil {
		return nil, err
	}
	sig, err := signer.SignBytes(bytes)
	if err != nil {
		return nil, err
	}
	return templates.NewP2pkh256SignatureBytes(sig, ac.PubKey), nil
}

func newFungibleTransferTxAttrs(token *sdktypes.TokenUnit, receiverPubKey []byte) *tokens.TransferFungibleTokenAttributes {
	return &tokens.TransferFungibleTokenAttributes{
		NewBearer:                    BearerPredicateFromPubKey(receiverPubKey),
		Value:                        token.Amount,
		Nonce:                        token.Nonce,
		Counter:                      token.Counter,
		TypeID:                       token.TypeID,
		InvariantPredicateSignatures: nil,
	}
}

func newNonFungibleTransferTxAttrs(token *sdktypes.TokenUnit, receiverPubKey []byte) *tokens.TransferNonFungibleTokenAttributes {
	return &tokens.TransferNonFungibleTokenAttributes{
		NewBearer:                    BearerPredicateFromPubKey(receiverPubKey),
		Nonce:                        token.Nonce,
		Counter:                      token.Counter,
		TypeID:                       token.TypeID,
		InvariantPredicateSignatures: nil,
	}
}

func newLockTxAttrs(counter uint64, lockStatus uint64) *tokens.LockTokenAttributes {
	return &tokens.LockTokenAttributes{
		LockStatus:                   lockStatus,
		Counter:                      counter,
		InvariantPredicateSignatures: nil,
	}
}

func newUnlockTxAttrs(counter uint64) *tokens.UnlockTokenAttributes {
	return &tokens.UnlockTokenAttributes{
		Counter:                      counter,
		InvariantPredicateSignatures: nil,
	}
}

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

func newSplitTxAttrs(token *sdktypes.TokenUnit, amount uint64, receiverPubKey []byte) *tokens.SplitFungibleTokenAttributes {
	return &tokens.SplitFungibleTokenAttributes{
		NewBearer:                    BearerPredicateFromPubKey(receiverPubKey),
		TargetValue:                  amount,
		Nonce:                        nil,
		Counter:                      token.Counter,
		TypeID:                       token.TypeID,
		RemainingValue:               token.Amount - amount,
		InvariantPredicateSignatures: [][]byte{nil},
	}
}

func newBurnTxAttrs(token *sdktypes.TokenUnit, targetTokenCounter uint64, targetTokenID types.UnitID) *tokens.BurnFungibleTokenAttributes {
	return &tokens.BurnFungibleTokenAttributes{
		TypeID:                       token.TypeID,
		Value:                        token.Amount,
		TargetTokenID:                targetTokenID,
		TargetTokenCounter:           targetTokenCounter,
		Counter:                      token.Counter,
		InvariantPredicateSignatures: nil,
	}
}

// assumes there's sufficient balance for the given amount, sends transactions immediately
func (w *Wallet) doSendMultiple(ctx context.Context, amount uint64, tokens []*sdktypes.TokenUnit, acc *accountKey, fcrID, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput, ownerProof *PredicateInput) (*SubmissionResult, error) {
	var accumulatedSum uint64
	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].Amount > tokens[j].Amount
	})

	batch := txsubmitter.NewBatch(w.tokensClient, w.log)
	rnFetcher := &cachingRoundNumberFetcher{delegate: w.GetRoundNumber}

	for _, t := range tokens {
		remainingAmount := amount - accumulatedSum
		sub, err := w.prepareSplitOrTransferTx(ctx, acc, remainingAmount, t, fcrID, receiverPubKey, invariantPredicateArgs, rnFetcher.getRoundNumber, ownerProof)
		if err != nil {
			return nil, err
		}
		batch.Add(sub)
		accumulatedSum += t.Amount
		if accumulatedSum >= amount {
			break
		}
	}
	err := batch.SendTx(ctx, w.confirmTx)
	feeSum := uint64(0)
	for _, sub := range batch.Submissions() {
		if sub.Confirmed() {
			feeSum += sub.Proof.TxRecord.ServerMetadata.ActualFee
		}
	}
	return &SubmissionResult{Submissions: batch.Submissions(), FeeSum: feeSum, AccountNumber: acc.AccountNumber()}, err
}

func (w *Wallet) prepareSplitOrTransferTx(ctx context.Context, acc *accountKey, amount uint64, token *sdktypes.TokenUnit, fcrID, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput, rn roundNumberFetcher, ownerProof *PredicateInput) (*txsubmitter.TxSubmission, error) {
	var attrs AttrWithInvariantPredicateInputs
	var payloadType string
	if amount >= token.Amount {
		attrs = newFungibleTransferTxAttrs(token, receiverPubKey)
		payloadType = tokens.PayloadTypeTransferFungibleToken
	} else {
		attrs = newSplitTxAttrs(token, amount, receiverPubKey)
		payloadType = tokens.PayloadTypeSplitFungibleToken
	}
	sub, err := w.prepareTxSubmission(ctx, acc, payloadType, attrs, token.ID, fcrID, rn, ownerProof, func(tx *types.TransactionOrder) error {
		signatures, err := preparePredicateSignatures(w.am, invariantPredicateArgs, tx, attrs)
		if err != nil {
			return err
		}
		attrs.SetInvariantPredicateSignatures(signatures)
		tx.Payload.Attributes, err = types.Cbor.Marshal(attrs)
		return err
	})
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func createTx(systemID types.SystemID, payloadType string, unitId []byte, timeout uint64, fcrID []byte) *types.TransactionOrder {
	return &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID: systemID,
			Type:     payloadType,
			UnitID:   unitId,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           timeout,
				MaxTransactionFee: txbuilder.MaxFee,
				FeeCreditRecordID: fcrID,
			},
		},
		// OwnerProof is added after whole transaction is built
	}
}
