package tokens

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/fees"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/txbuilder"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
	"github.com/alphabill-org/alphabill/predicates"
)

const (
	AllAccounts uint64 = 0

	uriMaxSize  = 4 * 1024
	dataMaxSize = 64 * 1024
	nameMaxSize = 256
)

var (
	errInvalidURILength  = fmt.Errorf("URI exceeds the maximum allowed size of %v bytes", uriMaxSize)
	errInvalidDataLength = fmt.Errorf("data exceeds the maximum allowed size of %v bytes", dataMaxSize)
	errInvalidNameLength = fmt.Errorf("name exceeds the maximum allowed size of %v bytes", nameMaxSize)

	ErrNoFeeCredit           = errors.New("no fee credit in token wallet")
	ErrInsufficientFeeCredit = errors.New("insufficient fee credit balance for transaction(s)")
)

type (
	Wallet struct {
		systemID     types.SystemID
		am           account.Manager
		tokensClient sdktypes.TokensPartitionClient
		confirmTx    bool
		feeManager   *fees.FeeManager
		log          *slog.Logger
	}

	// SubmissionResult dust collection result for single token type.
	SubmissionResult struct {
		Submissions   []*txsubmitter.TxSubmission
		AccountNumber uint64
		FeeSum        uint64
	}
)

func New(systemID types.SystemID, tokensClient sdktypes.TokensPartitionClient, am account.Manager, confirmTx bool, feeManager *fees.FeeManager, log *slog.Logger) (*Wallet, error) {
	return &Wallet{
		systemID:     systemID,
		am:           am,
		tokensClient: tokensClient,
		confirmTx:    confirmTx,
		feeManager:   feeManager,
		log:          log,
	}, nil
}

func (w *Wallet) Close() {
	w.am.Close()
	if w.feeManager != nil {
		w.feeManager.Close()
	}
	if w.tokensClient != nil {
		w.tokensClient.Close()
	}
}

func newSingleResult(sub *txsubmitter.TxSubmission, accNr uint64) *SubmissionResult {
	res := &SubmissionResult{AccountNumber: accNr}
	if sub == nil {
		return res
	}
	res.Submissions = []*txsubmitter.TxSubmission{sub}
	if sub.Confirmed() {
		res.FeeSum = sub.Proof.TxRecord.ServerMetadata.ActualFee
	}
	return res
}

func (r *SubmissionResult) GetProofs() []*sdktypes.Proof {
	proofs := make([]*sdktypes.Proof, len(r.Submissions))
	for i, sub := range r.Submissions {
		proofs[i] = sub.Proof
	}
	return proofs
}

func (r *SubmissionResult) GetUnit() types.UnitID {
	if len(r.Submissions) == 1 {
		return r.Submissions[0].UnitID
	}
	return nil
}

func (w *Wallet) GetAccountManager() account.Manager {
	return w.am
}

func (w *Wallet) NewFungibleType(ctx context.Context, accountNumber uint64, attrs CreateFungibleTokenTypeAttributes, typeID sdktypes.TokenTypeID, subtypePredicateArgs []*PredicateInput) (*SubmissionResult, error) {
	w.log.Info("Creating new fungible token type")
	if typeID == nil {
		var err error
		typeID, err = tokens.NewRandomFungibleTokenTypeID(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to generate fungible token type ID: %w", err)
		}
	}

	if len(typeID) != tokens.UnitIDLength {
		return nil, fmt.Errorf("invalid token type ID: expected hex length is %d characters (%d bytes)",
			tokens.UnitIDLength*2, tokens.UnitIDLength)
	}
	if !typeID.HasType(tokens.FungibleTokenTypeUnitType) {
		return nil, fmt.Errorf("invalid token type ID: expected unit type is 0x%X", tokens.FungibleTokenTypeUnitType)
	}
	if attrs.ParentTypeID != nil && !bytes.Equal(attrs.ParentTypeID, sdktypes.NoParent) {
		parentType, err := w.GetTokenType(ctx, attrs.ParentTypeID)
		if err != nil {
			return nil, fmt.Errorf("failed to get parent type: %w", err)
		}
		if parentType.DecimalPlaces != attrs.DecimalPlaces {
			return nil, fmt.Errorf("parent type requires %d decimal places, got %d", parentType.DecimalPlaces, attrs.DecimalPlaces)
		}
	}
	sub, err := w.newType(ctx, accountNumber, tokens.PayloadTypeCreateFungibleTokenType, attrs.ToCBOR(), typeID, subtypePredicateArgs)
	if err != nil {
		return nil, err
	}

	return newSingleResult(sub, accountNumber), nil
}

func (w *Wallet) NewNonFungibleType(ctx context.Context, accountNumber uint64, attrs CreateNonFungibleTokenTypeAttributes, typeId sdktypes.TokenTypeID, subtypePredicateArgs []*PredicateInput) (*SubmissionResult, error) {
	w.log.Info("Creating new NFT type")
	if typeId == nil {
		var err error
		typeId, err = tokens.NewRandomNonFungibleTokenTypeID(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to generate non-fungible token type ID: %w", err)
		}
	}

	if len(typeId) != tokens.UnitIDLength {
		return nil, fmt.Errorf("invalid token type ID: expected hex length is %d characters (%d bytes)",
			tokens.UnitIDLength*2, tokens.UnitIDLength)
	}
	if !typeId.HasType(tokens.NonFungibleTokenTypeUnitType) {
		return nil, fmt.Errorf("invalid token type ID: expected unit type is 0x%X", tokens.NonFungibleTokenTypeUnitType)
	}

	sub, err := w.newType(ctx, accountNumber, tokens.PayloadTypeCreateNFTType, attrs.ToCBOR(), typeId, subtypePredicateArgs)
	if err != nil {
		return nil, err
	}
	return newSingleResult(sub, accountNumber), nil
}

func (w *Wallet) NewFungibleToken(ctx context.Context, accountNumber uint64, typeID sdktypes.TokenTypeID, amount uint64, bearerPredicate sdktypes.Predicate, mintPredicateArgs []*PredicateInput) (*SubmissionResult, error) {
	w.log.Info("Creating new fungible token")
	attrs := &tokens.MintFungibleTokenAttributes{
		Bearer:                           bearerPredicate,
		TypeID:                           typeID,
		Value:                            amount,
		Nonce:                            0,
		TokenCreationPredicateSignatures: nil,
	}
	sub, err := w.newToken(ctx, accountNumber, tokens.PayloadTypeMintFungibleToken, attrs, mintPredicateArgs, tokens.NewFungibleTokenID)
	if err != nil {
		return nil, err
	}
	return newSingleResult(sub, accountNumber), nil
}

func (w *Wallet) NewNFT(ctx context.Context, accountNumber uint64, attrs *tokens.MintNonFungibleTokenAttributes, mintPredicateArgs []*PredicateInput) (*SubmissionResult, error) {
	w.log.Info("Creating new NFT")
	if len(attrs.Name) > nameMaxSize {
		return nil, errInvalidNameLength
	}
	if len(attrs.URI) > uriMaxSize {
		return nil, errInvalidURILength
	}
	if attrs.URI != "" && !util.IsValidURI(attrs.URI) {
		return nil, fmt.Errorf("URI '%s' is invalid", attrs.URI)
	}
	if len(attrs.Data) > dataMaxSize {
		return nil, errInvalidDataLength
	}

	sub, err := w.newToken(ctx, accountNumber, tokens.PayloadTypeMintNFT, attrs, mintPredicateArgs, tokens.NewNonFungibleTokenID)
	if err != nil {
		return nil, err
	}
	return newSingleResult(sub, accountNumber), nil
}

func (w *Wallet) ListTokenTypes(ctx context.Context, accountNumber uint64, kind sdktypes.Kind) ([]*sdktypes.TokenTypeUnit, error) {
	keys, err := w.getAccounts(accountNumber)
	if err != nil {
		return nil, err
	}
	allTokenTypes := make([]*sdktypes.TokenTypeUnit, 0)
	fetchForPubKey := func(pubKey []byte) ([]*sdktypes.TokenTypeUnit, error) {
		typez, err := w.tokensClient.GetTokenTypes(ctx, kind, pubKey)
		if err != nil {
			return nil, err
		}
		return typez, nil
	}

	for _, key := range keys {
		typez, err := fetchForPubKey(key.PubKey)
		if err != nil {
			return nil, err
		}
		allTokenTypes = append(allTokenTypes, typez...)
	}

	return allTokenTypes, nil
}

// GetTokenType returns non-nil TokenUnitType or error if not found or other issues
func (w *Wallet) GetTokenType(ctx context.Context, typeId sdktypes.TokenTypeID) (*sdktypes.TokenTypeUnit, error) {
	typez, err := w.tokensClient.GetTypeHierarchy(ctx, typeId)
	if err != nil {
		return nil, err
	}
	for i := range typez {
		if bytes.Equal(typez[i].ID, typeId) {
			return typez[i], nil
		}
	}
	return nil, fmt.Errorf("token type %X not found", typeId)
}

// ListTokens specify accountNumber=0 to list tokens from all accounts
func (w *Wallet) ListTokens(ctx context.Context, kind sdktypes.Kind, accountNumber uint64) (map[uint64][]*sdktypes.TokenUnit, error) {
	keys, err := w.getAccounts(accountNumber)
	if err != nil {
		return nil, err
	}

	// account number -> list of its tokens
	allTokensByAccountNumber := make(map[uint64][]*sdktypes.TokenUnit, len(keys))
	for _, key := range keys {
		ts, err := w.getTokens(ctx, kind, key.PubKeyHash.Sha256)
		if err != nil {
			return nil, err
		}
		allTokensByAccountNumber[key.idx+1] = ts
	}

	return allTokensByAccountNumber, nil
}

type accountKey struct {
	*account.AccountKey
	idx uint64
}

func (a *accountKey) AccountNumber() uint64 {
	return a.idx + 1
}

func (w *Wallet) getAccount(accountNumber uint64) (*accountKey, error) {
	if accountNumber < 1 {
		return nil, fmt.Errorf("invalid account number: %d", accountNumber)
	}
	key, err := w.am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return nil, err
	}
	return &accountKey{AccountKey: key, idx: accountNumber - 1}, nil
}

func (w *Wallet) getAccounts(accountNumber uint64) ([]*accountKey, error) {
	if accountNumber > AllAccounts {
		key, err := w.getAccount(accountNumber)
		if err != nil {
			return nil, err
		}
		return []*accountKey{key}, nil
	}
	keys, err := w.am.GetAccountKeys()
	if err != nil {
		return nil, err
	}
	wrappers := make([]*accountKey, len(keys))
	for i := range keys {
		wrappers[i] = &accountKey{AccountKey: keys[i], idx: uint64(i)}
	}
	return wrappers, nil
}

func (w *Wallet) getTokens(ctx context.Context, kind sdktypes.Kind, pubKeyHash sdktypes.PubKeyHash) ([]*sdktypes.TokenUnit, error) {
	return w.tokensClient.GetTokens(ctx, kind, pubKeyHash)
}

func (w *Wallet) GetToken(ctx context.Context, tokenID sdktypes.TokenID) (*sdktypes.TokenUnit, error) {
	token, err := w.tokensClient.GetToken(ctx, tokenID)
	if err != nil {
		return nil, fmt.Errorf("error fetching token %X: %w", tokenID, err)
	}
	return token, nil
}

func (w *Wallet) TransferNFT(ctx context.Context, accountNumber uint64, tokenID sdktypes.TokenID, receiverPubKey sdktypes.PubKey, invariantPredicateArgs []*PredicateInput, ownerProof *PredicateInput) (*SubmissionResult, error) {
	key, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}
	fcrID, err := w.ensureFeeCredit(ctx, key.AccountKey, 1)
	if err != nil {
		return nil, err
	}
	token, err := w.GetToken(ctx, tokenID)
	if err != nil {
		return nil, err
	}
	if err = ensureTokenOwnership(key, token, ownerProof); err != nil {
		return nil, err
	}
	if token.IsLocked() {
		return nil, errors.New("token is locked")
	}
	attrs := newNonFungibleTransferTxAttrs(token, receiverPubKey)
	sub, err := w.prepareTxSubmission(ctx, key, tokens.PayloadTypeTransferNFT, attrs, tokenID, fcrID, w.GetRoundNumber, ownerProof, func(tx *types.TransactionOrder) error {
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
	err = sub.ToBatch(w.tokensClient, w.log).SendTx(ctx, w.confirmTx)
	return newSingleResult(sub, accountNumber), err
}

func (w *Wallet) SendFungible(ctx context.Context, accountNumber uint64, typeId sdktypes.TokenTypeID, targetAmount uint64, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput, ownerProof *PredicateInput) (*SubmissionResult, error) {
	if targetAmount == 0 {
		return nil, fmt.Errorf("invalid amount: 0")
	}
	if accountNumber < 1 {
		return nil, fmt.Errorf("invalid account number: %d", accountNumber)
	}
	acc, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}
	fcrID, err := w.ensureFeeCredit(ctx, acc.AccountKey, 1)
	if err != nil {
		return nil, err
	}
	tokensByAcc, err := w.ListTokens(ctx, sdktypes.Fungible, accountNumber)
	if err != nil {
		return nil, err
	}
	tokenz, found := tokensByAcc[accountNumber]
	if !found {
		return nil, fmt.Errorf("account %d has no tokens", accountNumber)
	}
	var matchingTokens []*sdktypes.TokenUnit
	var totalBalance uint64
	// find the best unit candidate for transfer or split, value must be equal or larger than the target amount
	var closestMatch *sdktypes.TokenUnit
	for _, token := range tokenz {
		if token.Kind != sdktypes.Fungible {
			return nil, fmt.Errorf("expected fungible token, got %v, token %X", token.Kind.String(), token.ID)
		}
		if typeId.Eq(token.TypeID) {
			if token.IsLocked() {
				continue
			}
			matchingTokens = append(matchingTokens, token)
			var overflow bool
			totalBalance, overflow, _ = util.AddUint64(totalBalance, token.Amount)
			if overflow {
				// capping the total balance to maxUint64 should be enough to perform the transfer
				totalBalance = math.MaxUint64
			}
			if closestMatch == nil {
				closestMatch = token
			} else {
				prevDiff := closestMatch.Amount - targetAmount
				currDiff := token.Amount - targetAmount
				// this should work with overflow nicely
				if prevDiff > currDiff {
					closestMatch = token
				}
			}
		}
	}
	if targetAmount > totalBalance {
		return nil, fmt.Errorf("insufficient tokens of type %s: got %v, need %v", typeId, totalBalance, targetAmount)
	}
	// optimization: first try to make a single operation instead of iterating through all tokens in doSendMultiple
	if closestMatch.Amount >= targetAmount {
		sub, err := w.prepareSplitOrTransferTx(ctx, acc, targetAmount, closestMatch, fcrID, receiverPubKey, invariantPredicateArgs, w.GetRoundNumber, ownerProof)
		if err != nil {
			return nil, err
		}
		err = sub.ToBatch(w.tokensClient, w.log).SendTx(ctx, w.confirmTx)
		return newSingleResult(sub, accountNumber), err
	} else {
		return w.doSendMultiple(ctx, targetAmount, matchingTokens, acc, fcrID, receiverPubKey, invariantPredicateArgs, ownerProof)
	}
}

func (w *Wallet) UpdateNFTData(ctx context.Context, accountNumber uint64, tokenID sdktypes.TokenID, data []byte, updatePredicateArgs []*PredicateInput) (*SubmissionResult, error) {
	acc, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}
	fcrID, err := w.ensureFeeCredit(ctx, acc.AccountKey, 1)
	if err != nil {
		return nil, err
	}
	t, err := w.GetToken(ctx, tokenID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("token with id=%X not found under account #%v", tokenID, accountNumber)
	}
	if t.IsLocked() {
		return nil, errors.New("token is locked")
	}

	attrs := &tokens.UpdateNonFungibleTokenAttributes{
		Data:                 data,
		Counter:              t.Counter,
		DataUpdateSignatures: nil,
	}

	sub, err := w.prepareTxSubmission(ctx, acc, tokens.PayloadTypeUpdateNFT, attrs, tokenID, fcrID, w.GetRoundNumber, defaultOwnerProof(accountNumber), func(tx *types.TransactionOrder) error {
		signatures, err := preparePredicateSignatures(w.am, updatePredicateArgs, tx, attrs)
		if err != nil {
			return err
		}
		attrs.SetDataUpdateSignatures(signatures)
		tx.Payload.Attributes, err = types.Cbor.Marshal(attrs)
		return err
	})
	if err != nil {
		return nil, err
	}
	err = sub.ToBatch(w.tokensClient, w.log).SendTx(ctx, w.confirmTx)
	return newSingleResult(sub, accountNumber), err
}

// SendFungibleByID sends fungible tokens by given unit ID, if amount matches, does the transfer, otherwise splits the token
func (w *Wallet) SendFungibleByID(ctx context.Context, accountNumber uint64, tokenID sdktypes.TokenID, targetAmount uint64, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput) (*SubmissionResult, error) {
	acc, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}
	fcrID, err := w.ensureFeeCredit(ctx, acc.AccountKey, 1)
	if err != nil {
		return nil, err
	}
	token, err := w.GetToken(ctx, tokenID)
	if err != nil {
		return nil, fmt.Errorf("failed to get token %X: %w", tokenID, err)
	}
	if err = ensureTokenOwnership(acc, token, defaultOwnerProof(accountNumber)); err != nil {
		return nil, err
	}
	if targetAmount > token.Amount {
		return nil, fmt.Errorf("insufficient FT value: got %v, need %v", token.Amount, targetAmount)
	}
	sub, err := w.prepareSplitOrTransferTx(ctx, acc, targetAmount, token, fcrID, receiverPubKey, invariantPredicateArgs, w.GetRoundNumber, defaultOwnerProof(accountNumber))
	if err != nil {
		return nil, err
	}
	err = sub.ToBatch(w.tokensClient, w.log).SendTx(ctx, w.confirmTx)
	return newSingleResult(sub, accountNumber), err
}

func (w *Wallet) BurnTokens(ctx context.Context, accountNumber uint64, tokensToBurn []*sdktypes.TokenUnit, invariantPredicateArgs []*PredicateInput) (uint64, uint64, []*sdktypes.Proof, error) {
	acc, err := w.getAccount(accountNumber)
	if err != nil {
		return 0, 0, nil, err
	}
	fcrID, err := w.ensureFeeCredit(ctx, acc.AccountKey, 1)
	if err != nil {
		return 0, 0, nil, err
	}
	return w.burnTokensForDC(ctx, acc, tokensToBurn, 0, nil, fcrID, invariantPredicateArgs)
}

func (w *Wallet) GetRoundNumber(ctx context.Context) (uint64, error) {
	return w.tokensClient.GetRoundNumber(ctx)
}

// GetFeeCredit returns fee credit record for the given account,
// can return nil if fee credit record has not been created yet.
// Deprecated: faucet still uses, will be removed
func (w *Wallet) GetFeeCredit(ctx context.Context, cmd fees.GetFeeCreditCmd) (*sdktypes.FeeCreditRecord, error) {
	ac, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}
	return w.tokensClient.GetFeeCreditRecordByOwnerID(ctx, ac.PubKeyHash.Sha256)
}

func (w *Wallet) AddFeeCredit(ctx context.Context, cmd fees.AddFeeCmd) (*fees.AddFeeCmdResponse, error) {
	return w.feeManager.AddFeeCredit(ctx, cmd)
}

func (w *Wallet) ReclaimFeeCredit(ctx context.Context, cmd fees.ReclaimFeeCmd) (*fees.ReclaimFeeCmdResponse, error) {
	return w.feeManager.ReclaimFeeCredit(ctx, cmd)
}

func (w *Wallet) ensureFeeCredit(ctx context.Context, accountKey *account.AccountKey, txCount int) ([]byte, error) {
	fcb, err := w.tokensClient.GetFeeCreditRecordByOwnerID(ctx, accountKey.PubKeyHash.Sha256)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch fee credit bill: %w", err)
	}
	if fcb == nil {
		return nil, ErrNoFeeCredit
	}
	maxFee := uint64(txCount) * txbuilder.MaxFee
	if fcb.Balance() < maxFee {
		return nil, ErrInsufficientFeeCredit
	}
	return fcb.ID, nil
}

func (w *Wallet) LockToken(ctx context.Context, accountNumber uint64, tokenID []byte, ib []*PredicateInput, ownerProof *PredicateInput) (*SubmissionResult, error) {
	key, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}
	fcrID, err := w.ensureFeeCredit(ctx, key.AccountKey, 1)
	if err != nil {
		return nil, err
	}

	token, err := w.GetToken(ctx, tokenID)
	if err != nil {
		return nil, err
	}
	if err = ensureTokenOwnership(key, token, ownerProof); err != nil {
		return nil, err
	}
	if token.IsLocked() {
		return nil, errors.New("token is already locked")
	}
	attrs := newLockTxAttrs(token.Counter, wallet.LockReasonManual)
	sub, err := w.prepareTxSubmission(ctx, key, tokens.PayloadTypeLockToken, attrs, tokenID, fcrID, w.GetRoundNumber, ownerProof, func(tx *types.TransactionOrder) error {
		signatures, err := preparePredicateSignatures(w.am, ib, tx, attrs)
		if err != nil {
			return err
		}
		attrs.InvariantPredicateSignatures = signatures
		tx.Payload.Attributes, err = types.Cbor.Marshal(attrs)
		return err
	})
	if err != nil {
		return nil, err
	}
	err = sub.ToBatch(w.tokensClient, w.log).SendTx(ctx, w.confirmTx)
	return newSingleResult(sub, accountNumber), err
}

func (w *Wallet) UnlockToken(ctx context.Context, accountNumber uint64, tokenID []byte, ib []*PredicateInput, ownerProof *PredicateInput) (*SubmissionResult, error) {
	key, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}
	fcrID, err := w.ensureFeeCredit(ctx, key.AccountKey, 1)
	if err != nil {
		return nil, err
	}
	token, err := w.GetToken(ctx, tokenID)
	if err != nil {
		return nil, err
	}
	if err = ensureTokenOwnership(key, token, ownerProof); err != nil {
		return nil, err
	}
	if !token.IsLocked() {
		return nil, errors.New("token is already unlocked")
	}
	attrs := newUnlockTxAttrs(token.Counter)
	sub, err := w.prepareTxSubmission(ctx, key, tokens.PayloadTypeUnlockToken, attrs, tokenID, fcrID, w.GetRoundNumber, ownerProof, func(tx *types.TransactionOrder) error {
		signatures, err := preparePredicateSignatures(w.am, ib, tx, attrs)
		if err != nil {
			return err
		}
		attrs.InvariantPredicateSignatures = signatures
		tx.Payload.Attributes, err = types.Cbor.Marshal(attrs)
		return err
	})
	if err != nil {
		return nil, err
	}
	err = sub.ToBatch(w.tokensClient, w.log).SendTx(ctx, w.confirmTx)
	return newSingleResult(sub, accountNumber), err
}

func ensureTokenOwnership(acc *accountKey, unit *sdktypes.TokenUnit, ownerProof *PredicateInput) error {
	if bytes.Equal(unit.Owner, templates.NewP2pkh256BytesFromKey(acc.PubKey)) {
		return nil
	}
	predicate, err := predicates.ExtractPredicate(unit.Owner)
	if err != nil {
		return err
	}
	if !templates.IsP2pkhTemplate(predicate) && ownerProof != nil && ownerProof.AccountNumber == 0 && ownerProof.Argument != nil {
		// this must be a "custom predicate" with provided owner proof
		return nil
	}
	return fmt.Errorf("token '%s' does not belong to account #%d", unit.ID, acc.AccountNumber())
}

func defaultOwnerProof(accNr uint64) *PredicateInput {
	return &PredicateInput{AccountNumber: accNr}
}
