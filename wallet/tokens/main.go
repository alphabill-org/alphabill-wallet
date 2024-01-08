package tokens

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"

	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
	"github.com/fxamacker/cbor/v2"
	"go.opentelemetry.io/otel/trace"

	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/fees"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/tx_builder"
	"github.com/alphabill-org/alphabill-wallet/wallet/tokens/backend"
)

const (
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
		systemID   types.SystemID
		am         account.Manager
		backend    TokenBackend
		confirmTx  bool
		feeManager *fees.FeeManager
		log        *slog.Logger
	}

	SubmissionResult struct {
		TokenTypeID   backend.TokenTypeID
		TokenID       backend.TokenID
		AccountNumber uint64
		FeeSum        uint64
	}

	TokenBackend interface {
		GetToken(ctx context.Context, id backend.TokenID) (*backend.TokenUnit, error)
		GetTokens(ctx context.Context, kind backend.Kind, owner wallet.PubKey, offset string, limit int) ([]*backend.TokenUnit, string, error)
		GetTokenTypes(ctx context.Context, kind backend.Kind, creator wallet.PubKey, offset string, limit int) ([]*backend.TokenUnitType, string, error)
		GetTypeHierarchy(ctx context.Context, id backend.TokenTypeID) ([]*backend.TokenUnitType, error)
		GetRoundNumber(ctx context.Context) (*wallet.RoundNumber, error)
		PostTransactions(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error
		GetTxProof(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error)
		GetFeeCreditBill(ctx context.Context, unitID types.UnitID) (*wallet.Bill, error)
		GetInfo(ctx context.Context) (*wallet.InfoResponse, error)
	}

	MoneyDataProvider interface {
		SystemID() []byte
		fees.TxPublisher
	}

	Observability interface {
		TracerProvider() trace.TracerProvider
	}
)

func New(systemID types.SystemID, backendClient TokenBackend, am account.Manager, confirmTx bool, feeManager *fees.FeeManager, log *slog.Logger) (*Wallet, error) {
	return &Wallet{
		systemID:   systemID,
		am:         am,
		backend:    backendClient,
		confirmTx:  confirmTx,
		feeManager: feeManager,
		log:        log,
	}, nil
}

func (w *Wallet) Shutdown() {
	w.am.Close()
	if w.feeManager != nil {
		w.feeManager.Close()
	}
}

func (w *Wallet) GetAccountManager() account.Manager {
	return w.am
}

func (w *Wallet) NewFungibleType(ctx context.Context, accNr uint64, attrs CreateFungibleTokenTypeAttributes, typeId backend.TokenTypeID, subtypePredicateArgs []*PredicateInput) (*SubmissionResult, error) {
	w.log.Info("Creating new fungible token type")
	if typeId == nil {
		var err error
		typeId, err = tokens.NewRandomFungibleTokenTypeID(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to generate fungible token type ID: %w", err)
		}
	}

	if len(typeId) != tokens.UnitIDLength {
		return nil, fmt.Errorf("invalid token type ID: expected hex length is %d characters (%d bytes)",
			tokens.UnitIDLength*2, tokens.UnitIDLength)
	}
	if !typeId.HasType(tokens.FungibleTokenTypeUnitType) {
		return nil, fmt.Errorf("invalid token type ID: expected unit type is 0x%X", tokens.FungibleTokenTypeUnitType)
	}
	if attrs.ParentTypeId != nil && !bytes.Equal(attrs.ParentTypeId, backend.NoParent) {
		parentType, err := w.GetTokenType(ctx, attrs.ParentTypeId)
		if err != nil {
			return nil, fmt.Errorf("failed to get parent type: %w", err)
		}
		if parentType.DecimalPlaces != attrs.DecimalPlaces {
			return nil, fmt.Errorf("parent type requires %d decimal places, got %d", parentType.DecimalPlaces, attrs.DecimalPlaces)
		}
	}
	sub, err := w.newType(ctx, accNr, tokens.PayloadTypeCreateFungibleTokenType, attrs.toCBOR(), typeId, subtypePredicateArgs)
	if err != nil {
		return nil, err
	}
	if sub.Confirmed() {
		return &SubmissionResult{TokenTypeID: sub.UnitID, FeeSum: sub.Proof.TxRecord.ServerMetadata.ActualFee}, nil
	}
	return &SubmissionResult{TokenTypeID: sub.UnitID}, nil
}

func (w *Wallet) NewNonFungibleType(ctx context.Context, accNr uint64, attrs CreateNonFungibleTokenTypeAttributes, typeId backend.TokenTypeID, subtypePredicateArgs []*PredicateInput) (*SubmissionResult, error) {
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

	sub, err := w.newType(ctx, accNr, tokens.PayloadTypeCreateNFTType, attrs.toCBOR(), typeId, subtypePredicateArgs)
	if err != nil {
		return nil, err
	}
	if sub.Confirmed() {
		return &SubmissionResult{TokenTypeID: sub.UnitID, FeeSum: sub.Proof.TxRecord.ServerMetadata.ActualFee}, nil
	}
	return &SubmissionResult{TokenTypeID: sub.UnitID}, nil
}

func (w *Wallet) NewFungibleToken(ctx context.Context, accNr uint64, typeId backend.TokenTypeID, amount uint64, bearerPredicate wallet.Predicate, mintPredicateArgs []*PredicateInput) (*SubmissionResult, error) {
	w.log.Info("Creating new fungible token")
	attrs := &tokens.MintFungibleTokenAttributes{
		Bearer:                           bearerPredicate,
		TypeID:                           typeId,
		Value:                            amount,
		TokenCreationPredicateSignatures: nil,
	}

	var err error
	tokenID, err := tokens.NewRandomFungibleTokenID(nil)
	if err != nil {
		return nil, fmt.Errorf("failed generate fungible token ID: %w", err)
	}
	sub, err := w.newToken(ctx, accNr, tokens.PayloadTypeMintFungibleToken, attrs, tokenID, mintPredicateArgs)
	if err != nil {
		return nil, err
	}
	if sub.Confirmed() {
		return &SubmissionResult{TokenID: sub.UnitID, FeeSum: sub.Proof.TxRecord.ServerMetadata.ActualFee}, nil
	}
	return &SubmissionResult{TokenID: sub.UnitID}, nil
}

func (w *Wallet) NewNFT(ctx context.Context, accNr uint64, attrs MintNonFungibleTokenAttributes, tokenId backend.TokenID, mintPredicateArgs []*PredicateInput) (*SubmissionResult, error) {
	w.log.Info("Creating new NFT")
	if len(attrs.Name) > nameMaxSize {
		return nil, errInvalidNameLength
	}
	if len(attrs.Uri) > uriMaxSize {
		return nil, errInvalidURILength
	}
	if attrs.Uri != "" && !util.IsValidURI(attrs.Uri) {
		return nil, fmt.Errorf("URI '%s' is invalid", attrs.Uri)
	}
	if len(attrs.Data) > dataMaxSize {
		return nil, errInvalidDataLength
	}
	if tokenId == nil {
		var err error
		tokenId, err = tokens.NewRandomNonFungibleTokenID(nil)
		if err != nil {
			return nil, fmt.Errorf("failed generate non-fungible token ID: %w", err)
		}
	}

	sub, err := w.newToken(ctx, accNr, tokens.PayloadTypeMintNFT, attrs.toCBOR(), tokenId, mintPredicateArgs)
	if err != nil {
		return nil, err
	}
	if sub.Confirmed() {
		return &SubmissionResult{TokenID: sub.UnitID, FeeSum: sub.Proof.TxRecord.ServerMetadata.ActualFee}, nil
	}
	return &SubmissionResult{TokenID: sub.UnitID}, nil
}

func (w *Wallet) ListTokenTypes(ctx context.Context, accountNumber uint64, kind backend.Kind) ([]*backend.TokenUnitType, error) {
	keys, err := w.getAccounts(accountNumber)
	if err != nil {
		return nil, err
	}
	allTokenTypes := make([]*backend.TokenUnitType, 0)
	fetchForPubKey := func(pubKey []byte) ([]*backend.TokenUnitType, error) {
		allTokenTypesForKey := make([]*backend.TokenUnitType, 0)
		var types []*backend.TokenUnitType
		offset := ""
		var err error
		for {
			types, offset, err = w.backend.GetTokenTypes(ctx, kind, pubKey, offset, 0)
			if err != nil {
				return nil, err
			}
			allTokenTypesForKey = append(allTokenTypesForKey, types...)
			if offset == "" {
				break
			}
		}
		return allTokenTypesForKey, nil
	}

	for _, key := range keys {
		types, err := fetchForPubKey(key.PubKey)
		if err != nil {
			return nil, err
		}
		allTokenTypes = append(allTokenTypes, types...)
	}

	return allTokenTypes, nil
}

// GetTokenType returns non-nil TokenUnitType or error if not found or other issues
func (w *Wallet) GetTokenType(ctx context.Context, typeId backend.TokenTypeID) (*backend.TokenUnitType, error) {
	types, err := w.backend.GetTypeHierarchy(ctx, typeId)
	if err != nil {
		return nil, err
	}
	for i := range types {
		if bytes.Equal(types[i].ID, typeId) {
			return types[i], nil
		}
	}
	return nil, fmt.Errorf("token type %X not found", typeId)
}

// ListTokens specify accountNumber=0 to list tokens from all accounts
func (w *Wallet) ListTokens(ctx context.Context, kind backend.Kind, accountNumber uint64) (map[uint64][]*backend.TokenUnit, error) {
	keys, err := w.getAccounts(accountNumber)
	if err != nil {
		return nil, err
	}

	// account number -> list of its tokens
	allTokensByAccountNumber := make(map[uint64][]*backend.TokenUnit, len(keys))
	for _, key := range keys {
		ts, err := w.getTokens(ctx, kind, key.PubKey)
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

func (w *Wallet) getAccounts(accountNumber uint64) ([]*accountKey, error) {
	if accountNumber > AllAccounts {
		key, err := w.am.GetAccountKey(accountNumber - 1)
		if err != nil {
			return nil, err
		}
		return []*accountKey{{AccountKey: key, idx: accountNumber - 1}}, nil
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

func (w *Wallet) getTokens(ctx context.Context, kind backend.Kind, pubKey wallet.PubKey) ([]*backend.TokenUnit, error) {
	allTokens := make([]*backend.TokenUnit, 0)
	var ts []*backend.TokenUnit
	offset := ""
	var err error
	for {
		ts, offset, err = w.backend.GetTokens(ctx, kind, pubKey, offset, 0)
		if err != nil {
			return nil, err
		}
		allTokens = append(allTokens, ts...)
		if offset == "" {
			break
		}
	}
	return allTokens, nil
}

func (w *Wallet) GetToken(ctx context.Context, owner wallet.PubKey, kind backend.Kind, tokenId backend.TokenID) (*backend.TokenUnit, error) {
	token, err := w.backend.GetToken(ctx, tokenId)
	if err != nil {
		return nil, fmt.Errorf("error fetching token %X: %w", tokenId, err)
	}
	return token, nil
}

func (w *Wallet) TransferNFT(ctx context.Context, accountNumber uint64, tokenId backend.TokenID, receiverPubKey wallet.PubKey, invariantPredicateArgs []*PredicateInput) (*SubmissionResult, error) {
	key, err := w.am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return nil, err
	}
	err = w.ensureFeeCredit(ctx, key, 1)
	if err != nil {
		return nil, err
	}
	token, err := w.GetToken(ctx, key.PubKey, backend.NonFungible, tokenId)
	if err != nil {
		return nil, err
	}
	if token.IsLocked() {
		return nil, errors.New("token is locked")
	}
	attrs := newNonFungibleTransferTxAttrs(token, receiverPubKey)
	sub, err := w.prepareTxSubmission(ctx, tokens.PayloadTypeTransferNFT, attrs, tokenId, key, w.GetRoundNumber, func(tx *types.TransactionOrder) error {
		signatures, err := preparePredicateSignatures(w.am, invariantPredicateArgs, tx, attrs)
		if err != nil {
			return err
		}
		attrs.SetInvariantPredicateSignatures(signatures)
		tx.Payload.Attributes, err = cbor.Marshal(attrs)
		return err
	})
	if err != nil {
		return nil, err
	}
	err = sub.ToBatch(w.backend, key.PubKey, w.log).SendTx(ctx, w.confirmTx)
	if sub.Confirmed() {
		return &SubmissionResult{FeeSum: sub.Proof.TxRecord.ServerMetadata.ActualFee}, err
	}
	return &SubmissionResult{}, err
}

func (w *Wallet) SendFungible(ctx context.Context, accountNumber uint64, typeId backend.TokenTypeID, targetAmount uint64, receiverPubKey []byte, invariantPredicateArgs []*PredicateInput) (*SubmissionResult, error) {
	if targetAmount == 0 {
		return nil, fmt.Errorf("invalid amount: 0")
	}
	if accountNumber < 1 {
		return nil, fmt.Errorf("invalid account number: %d", accountNumber)
	}
	acc, err := w.am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return nil, err
	}
	err = w.ensureFeeCredit(ctx, acc, 1)
	if err != nil {
		return nil, err
	}
	tokensByAcc, err := w.ListTokens(ctx, backend.Fungible, accountNumber)
	if err != nil {
		return nil, err
	}
	tokenz, found := tokensByAcc[accountNumber]
	if !found {
		return nil, fmt.Errorf("account %d has no tokens", accountNumber)
	}
	var matchingTokens []*backend.TokenUnit
	var totalBalance uint64
	// find the best unit candidate for transfer or split, value must be equal or larger than the target amount
	var closestMatch *backend.TokenUnit
	for _, token := range tokenz {
		if token.Kind != backend.Fungible {
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
		sub, err := w.prepareSplitOrTransferTx(ctx, acc, targetAmount, closestMatch, receiverPubKey, invariantPredicateArgs, w.GetRoundNumber)
		if err != nil {
			return nil, err
		}
		err = sub.ToBatch(w.backend, acc.PubKey, w.log).SendTx(ctx, w.confirmTx)
		if sub.Confirmed() {
			return &SubmissionResult{FeeSum: sub.Proof.TxRecord.ServerMetadata.ActualFee}, err
		}
		return &SubmissionResult{}, err
	} else {
		return w.doSendMultiple(ctx, targetAmount, matchingTokens, acc, receiverPubKey, invariantPredicateArgs)
	}
}

func (w *Wallet) UpdateNFTData(ctx context.Context, accountNumber uint64, tokenId []byte, data []byte, updatePredicateArgs []*PredicateInput) (*SubmissionResult, error) {
	if accountNumber < 1 {
		return nil, fmt.Errorf("invalid account number: %d", accountNumber)
	}
	acc, err := w.am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return nil, err
	}
	err = w.ensureFeeCredit(ctx, acc, 1)
	if err != nil {
		return nil, err
	}
	t, err := w.GetToken(ctx, acc.PubKey, backend.NonFungible, tokenId)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, fmt.Errorf("token with id=%X not found under account #%v", tokenId, accountNumber)
	}
	if t.IsLocked() {
		return nil, errors.New("token is locked")
	}

	attrs := &tokens.UpdateNonFungibleTokenAttributes{
		Data:                 data,
		Backlink:             t.TxHash,
		DataUpdateSignatures: nil,
	}

	sub, err := w.prepareTxSubmission(ctx, tokens.PayloadTypeUpdateNFT, attrs, tokenId, acc, w.GetRoundNumber, func(tx *types.TransactionOrder) error {
		signatures, err := preparePredicateSignatures(w.am, updatePredicateArgs, tx, attrs)
		if err != nil {
			return err
		}
		attrs.SetDataUpdateSignatures(signatures)
		tx.Payload.Attributes, err = cbor.Marshal(attrs)
		return err
	})
	if err != nil {
		return nil, err
	}
	err = sub.ToBatch(w.backend, acc.PubKey, w.log).SendTx(ctx, w.confirmTx)
	if sub.Confirmed() {
		return &SubmissionResult{FeeSum: sub.Proof.TxRecord.ServerMetadata.ActualFee}, err
	}
	return &SubmissionResult{}, err
}

// GetFeeCredit returns fee credit bill for given account,
// can return nil if fee credit bill has not been created yet.
func (w *Wallet) GetFeeCredit(ctx context.Context, cmd fees.GetFeeCreditCmd) (*wallet.Bill, error) {
	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}
	return w.GetFeeCreditBill(ctx, tokens.NewFeeCreditRecordID(nil, accountKey.PubKeyHash.Sha256))
}

// GetFeeCreditBill returns fee credit bill for given unitID
// can return nil if fee credit bill has not been created yet.
func (w *Wallet) GetFeeCreditBill(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
	return w.backend.GetFeeCreditBill(ctx, unitID)
}

func (w *Wallet) GetRoundNumber(ctx context.Context) (*wallet.RoundNumber, error) {
	return w.backend.GetRoundNumber(ctx)
}

func (w *Wallet) AddFeeCredit(ctx context.Context, cmd fees.AddFeeCmd) (*fees.AddFeeCmdResponse, error) {
	return w.feeManager.AddFeeCredit(ctx, cmd)
}

func (w *Wallet) ReclaimFeeCredit(ctx context.Context, cmd fees.ReclaimFeeCmd) (*fees.ReclaimFeeCmdResponse, error) {
	return w.feeManager.ReclaimFeeCredit(ctx, cmd)
}

func (w *Wallet) ensureFeeCredit(ctx context.Context, accountKey *account.AccountKey, txCount int) error {
	fcb, err := w.GetFeeCreditBill(ctx, tokens.NewFeeCreditRecordID(nil, accountKey.PubKeyHash.Sha256))
	if err != nil {
		return err
	}
	if fcb == nil {
		return ErrNoFeeCredit
	}
	maxFee := uint64(txCount) * tx_builder.MaxFee
	if fcb.GetValue() < maxFee {
		return ErrInsufficientFeeCredit
	}
	return nil
}

func (w *Wallet) LockToken(ctx context.Context, accountNumber uint64, tokenID []byte, ib []*PredicateInput) (*SubmissionResult, error) {
	key, err := w.am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return nil, err
	}
	err = w.ensureFeeCredit(ctx, key, 1)
	if err != nil {
		return nil, err
	}

	token, err := w.GetToken(ctx, key.PubKey, backend.NonFungible, tokenID)
	if err != nil {
		return nil, err
	}
	if token.IsLocked() {
		return nil, errors.New("token is already locked")
	}
	attrs := newLockTxAttrs(token.TxHash, wallet.LockReasonManual)
	sub, err := w.prepareTxSubmission(ctx, tokens.PayloadTypeLockToken, attrs, tokenID, key, w.GetRoundNumber, func(tx *types.TransactionOrder) error {
		signatures, err := preparePredicateSignatures(w.am, ib, tx, attrs)
		if err != nil {
			return err
		}
		attrs.InvariantPredicateSignatures = signatures
		tx.Payload.Attributes, err = cbor.Marshal(attrs)
		return err
	})
	if err != nil {
		return nil, err
	}
	err = sub.ToBatch(w.backend, key.PubKey, w.log).SendTx(ctx, w.confirmTx)
	if sub.Confirmed() {
		return &SubmissionResult{FeeSum: sub.Proof.TxRecord.ServerMetadata.ActualFee}, err
	}
	return &SubmissionResult{}, err
}

func (w *Wallet) UnlockToken(ctx context.Context, accountNumber uint64, tokenID []byte, ib []*PredicateInput) (*SubmissionResult, error) {
	key, err := w.am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return nil, err
	}
	err = w.ensureFeeCredit(ctx, key, 1)
	if err != nil {
		return nil, err
	}
	token, err := w.GetToken(ctx, key.PubKey, backend.Any, tokenID)
	if err != nil {
		return nil, err
	}
	if !token.IsLocked() {
		return nil, errors.New("token is already unlocked")
	}
	attrs := newUnlockTxAttrs(token.TxHash)
	sub, err := w.prepareTxSubmission(ctx, tokens.PayloadTypeUnlockToken, attrs, tokenID, key, w.GetRoundNumber, func(tx *types.TransactionOrder) error {
		signatures, err := preparePredicateSignatures(w.am, ib, tx, attrs)
		if err != nil {
			return err
		}
		attrs.InvariantPredicateSignatures = signatures
		tx.Payload.Attributes, err = cbor.Marshal(attrs)
		return err
	})
	if err != nil {
		return nil, err
	}
	err = sub.ToBatch(w.backend, key.PubKey, w.log).SendTx(ctx, w.confirmTx)
	if sub.Confirmed() {
		return &SubmissionResult{FeeSum: sub.Proof.TxRecord.ServerMetadata.ActualFee}, err
	}
	return &SubmissionResult{}, err
}
