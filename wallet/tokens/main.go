package tokens

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"

	"github.com/alphabill-org/alphabill-go-base/predicates"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/nop"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/alphabill-org/alphabill-go-base/util"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/fees"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
)

const (
	AllAccounts uint64 = 0
	uriMaxSize         = 4 * 1024
	dataMaxSize        = 64 * 1024
	nameMaxSize        = 256
)

var (
	ErrNoFeeCredit           = errors.New("no fee credit in token wallet")
	ErrInsufficientFeeCredit = errors.New("insufficient fee credit balance for transaction(s)")
	errInvalidURILength      = fmt.Errorf("URI exceeds the maximum allowed size of %v bytes", uriMaxSize)
	errInvalidDataLength     = fmt.Errorf("data exceeds the maximum allowed size of %v bytes", dataMaxSize)
	errInvalidNameLength     = fmt.Errorf("name exceeds the maximum allowed size of %v bytes", nameMaxSize)
)

type (
	Wallet struct {
		pdr          *types.PartitionDescriptionRecord
		am           account.Manager
		tokensClient sdktypes.TokensPartitionClient
		confirmTx    bool
		feeManager   *fees.FeeManager
		maxFee       uint64
		log          *slog.Logger
	}

	// SubmissionResult dust collection result for single token type.
	SubmissionResult struct {
		Submissions   []*txsubmitter.TxSubmission
		AccountNumber uint64
		FeeSum        uint64
	}

	Token interface {
		GetID() sdktypes.TokenID
		GetOwnerPredicate() sdktypes.Predicate
		GetStateLockTx() hex.Bytes
		Lock(stateLock *types.StateLock, txOptions ...sdktypes.Option) (*types.TransactionOrder, error)
		Unlock(txOptions ...sdktypes.Option) (*types.TransactionOrder, error)
	}
)

func New(tokensClient sdktypes.TokensPartitionClient, am account.Manager, confirmTx bool, feeManager *fees.FeeManager, maxFee uint64, log *slog.Logger) (*Wallet, error) {
	pdr, err := tokensClient.PartitionDescription(context.Background())
	if err != nil {
		return nil, fmt.Errorf("loading partition description: %w", err)
	}
	if pdr.PartitionTypeID != tokens.PartitionTypeID {
		return nil, fmt.Errorf("invalid rpc url: expected tokens partition (%d) node reports partition type %d", tokens.PartitionTypeID, pdr.PartitionTypeID)
	}

	return &Wallet{
		pdr:          pdr,
		am:           am,
		tokensClient: tokensClient,
		confirmTx:    confirmTx,
		feeManager:   feeManager,
		maxFee:       maxFee,
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

func (r *SubmissionResult) GetProofs() []*types.TxRecordProof {
	proofs := make([]*types.TxRecordProof, len(r.Submissions))
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

func (w *Wallet) NetworkID() types.NetworkID {
	return w.pdr.NetworkID
}

func (w *Wallet) PartitionID() types.PartitionID {
	return w.pdr.PartitionID
}

func (w *Wallet) NewFungibleType(ctx context.Context, accountNumber uint64, ft *sdktypes.FungibleTokenType, subtypePredicateInputs []*PredicateInput) (*SubmissionResult, error) {
	w.log.Info("Creating new FT type")

	if len(ft.ID) != 0 {
		if idLen := int(w.pdr.UnitIDLen+w.pdr.TypeIDLen) / 8; idLen != len(ft.ID) {
			return nil, fmt.Errorf("invalid token type ID: expected hex length is %d characters (%d bytes)", idLen*2, idLen)
		}
		if ft.ID.TypeMustBe(tokens.FungibleTokenTypeUnitType, w.pdr) != nil {
			return nil, fmt.Errorf("invalid token type ID: expected unit type is 0x%X", tokens.FungibleTokenTypeUnitType)
		}
	}

	if ft.ParentTypeID != nil && !bytes.Equal(ft.ParentTypeID, sdktypes.NoParent) {
		parentType, err := w.GetFungibleTokenType(ctx, ft.ParentTypeID)
		if err != nil {
			return nil, fmt.Errorf("failed to get parent type: %w", err)
		}
		if parentType.DecimalPlaces != ft.DecimalPlaces {
			return nil, fmt.Errorf("parent type requires %d decimal places, got %d", parentType.DecimalPlaces, ft.DecimalPlaces)
		}
	}

	acc, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}
	fcrID, err := w.ensureFeeCredit(ctx, acc.AccountKey, 1)
	if err != nil {
		return nil, err
	}
	roundNumber, err := w.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}

	ft.NetworkID = w.pdr.NetworkID
	ft.PartitionID = w.pdr.PartitionID
	tx, err := ft.Define(
		sdktypes.WithTimeout(roundNumber+txTimeoutRoundCount),
		sdktypes.WithFeeCreditRecordID(fcrID),
		sdktypes.WithMaxFee(w.maxFee),
	)
	if err != nil {
		return nil, err
	}
	if len(ft.ID) == 0 {
		if err = tokens.GenerateUnitID(tx, w.pdr); err != nil {
			return nil, fmt.Errorf("failed to generate fungible token type ID: %w", err)
		}
		ft.ID = tx.UnitID
	}

	sigBytes, err := tx.AuthProofSigBytes()
	if err != nil {
		return nil, err
	}
	subTypeCreationProofs, err := newProofs(sigBytes, subtypePredicateInputs)
	if err != nil {
		return nil, err
	}
	err = tx.SetAuthProof(tokens.DefineFungibleTokenAuthProof{
		SubTypeCreationProofs: subTypeCreationProofs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to set auth proof: %w", err)
	}
	tx.FeeProof, err = sdktypes.NewP2pkhFeeSignatureFromKey(tx, acc.PrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign tx fee proof: %w", err)
	}

	return w.submitTx(ctx, tx, accountNumber)
}

func (w *Wallet) NewNonFungibleType(ctx context.Context, accountNumber uint64, nft *sdktypes.NonFungibleTokenType, subtypePredicateInputs []*PredicateInput) (*SubmissionResult, error) {
	w.log.Info("Creating new NFT type")

	if len(nft.ID) != 0 {
		if idLen := int(w.pdr.UnitIDLen+w.pdr.TypeIDLen) / 8; idLen != len(nft.ID) {
			return nil, fmt.Errorf("invalid token type ID: expected hex length is %d characters (%d bytes)", idLen*2, idLen)
		}
		if nft.ID.TypeMustBe(tokens.NonFungibleTokenTypeUnitType, w.pdr) != nil {
			return nil, fmt.Errorf("invalid token type ID: expected unit type is %#x", tokens.NonFungibleTokenTypeUnitType)
		}
	}

	acc, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}
	fcrID, err := w.ensureFeeCredit(ctx, acc.AccountKey, 1)
	if err != nil {
		return nil, err
	}
	roundNumber, err := w.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}

	nft.NetworkID = w.pdr.NetworkID
	nft.PartitionID = w.pdr.PartitionID
	tx, err := nft.Define(
		sdktypes.WithTimeout(roundNumber+txTimeoutRoundCount),
		sdktypes.WithFeeCreditRecordID(fcrID),
		sdktypes.WithMaxFee(w.maxFee),
	)
	if err != nil {
		return nil, err
	}
	if len(tx.UnitID) == 0 {
		if err = tokens.GenerateUnitID(tx, w.pdr); err != nil {
			return nil, fmt.Errorf("failed to generate non-fungible token type ID: %w", err)
		}
		nft.ID = tx.UnitID
	}

	sigBytes, err := tx.AuthProofSigBytes()
	if err != nil {
		return nil, err
	}
	subTypeProofs, err := newProofs(sigBytes, subtypePredicateInputs)
	if err != nil {
		return nil, err
	}
	err = tx.SetAuthProof(tokens.DefineNonFungibleTokenAuthProof{
		SubTypeCreationProofs: subTypeProofs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to set auth proof: %w", err)
	}
	tx.FeeProof, err = sdktypes.NewP2pkhFeeSignatureFromKey(tx, acc.PrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign tx fee proof: %w", err)
	}

	return w.submitTx(ctx, tx, accountNumber)
}

func (w *Wallet) NewFungibleToken(ctx context.Context, accountNumber uint64, ft *sdktypes.FungibleToken, mintPredicateInput *PredicateInput) (*SubmissionResult, error) {
	w.log.Info("Minting new fungible token")

	acc, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}
	fcrID, err := w.ensureFeeCredit(ctx, acc.AccountKey, 1)
	if err != nil {
		return nil, err
	}
	roundNumber, err := w.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := ft.Mint(
		w.pdr,
		sdktypes.WithTimeout(roundNumber+txTimeoutRoundCount),
		sdktypes.WithFeeCreditRecordID(fcrID),
		sdktypes.WithMaxFee(w.maxFee),
	)
	if err != nil {
		return nil, err
	}

	sigBytes, err := tx.AuthProofSigBytes()
	if err != nil {
		return nil, err
	}
	tokenMintingProof, err := mintPredicateInput.Proof(sigBytes)
	if err != nil {
		return nil, err
	}
	err = tx.SetAuthProof(tokens.MintFungibleTokenAuthProof{TokenMintingProof: tokenMintingProof})
	if err != nil {
		return nil, fmt.Errorf("failed to set auth proof: %w", err)
	}
	tx.FeeProof, err = sdktypes.NewP2pkhFeeSignatureFromKey(tx, acc.PrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign tx fee proof: %w", err)
	}

	return w.submitTx(ctx, tx, accountNumber)
}

func (w *Wallet) NewNFT(ctx context.Context, accountNumber uint64, nft *sdktypes.NonFungibleToken, mintPredicateInput *PredicateInput) (*SubmissionResult, error) {
	w.log.Info("Minting new NFT")

	if len(nft.Name) > nameMaxSize {
		return nil, errInvalidNameLength
	}
	if len(nft.URI) > uriMaxSize {
		return nil, errInvalidURILength
	}
	if nft.URI != "" && !util.IsValidURI(nft.URI) {
		return nil, fmt.Errorf("URI '%s' is invalid", nft.URI)
	}
	if len(nft.Data) > dataMaxSize {
		return nil, errInvalidDataLength
	}

	acc, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}
	fcrID, err := w.ensureFeeCredit(ctx, acc.AccountKey, 1)
	if err != nil {
		return nil, err
	}
	roundNumber, err := w.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := nft.Mint(
		w.pdr,
		sdktypes.WithTimeout(roundNumber+txTimeoutRoundCount),
		sdktypes.WithFeeCreditRecordID(fcrID),
		sdktypes.WithMaxFee(w.maxFee),
	)
	if err != nil {
		return nil, err
	}

	sigBytes, err := tx.AuthProofSigBytes()
	if err != nil {
		return nil, err
	}
	tokenMintingProof, err := mintPredicateInput.Proof(sigBytes)
	if err != nil {
		return nil, err
	}
	err = tx.SetAuthProof(tokens.MintNonFungibleTokenAuthProof{TokenMintingProof: tokenMintingProof})
	if err != nil {
		return nil, fmt.Errorf("failed to set auth proof: %w", err)
	}
	tx.FeeProof, err = sdktypes.NewP2pkhFeeSignatureFromKey(tx, acc.PrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign tx fee proof: %w", err)
	}

	return w.submitTx(ctx, tx, accountNumber)
}

func (w *Wallet) ListFungibleTokenTypes(ctx context.Context, accountNumber uint64) ([]*sdktypes.FungibleTokenType, error) {
	keys, err := w.getAccounts(accountNumber)
	if err != nil {
		return nil, err
	}
	allTokenTypes := make([]*sdktypes.FungibleTokenType, 0)
	fetchForPubKey := func(pubKey []byte) ([]*sdktypes.FungibleTokenType, error) {
		typez, err := w.tokensClient.GetFungibleTokenTypes(ctx, pubKey)
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

func (w *Wallet) ListNonFungibleTokenTypes(ctx context.Context, accountNumber uint64) ([]*sdktypes.NonFungibleTokenType, error) {
	keys, err := w.getAccounts(accountNumber)
	if err != nil {
		return nil, err
	}
	allTokenTypes := make([]*sdktypes.NonFungibleTokenType, 0)
	fetchForPubKey := func(pubKey []byte) ([]*sdktypes.NonFungibleTokenType, error) {
		typez, err := w.tokensClient.GetNonFungibleTokenTypes(ctx, pubKey)
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

// GetFungibleTokenType returns FungibleTokenType or nil if not found
func (w *Wallet) GetFungibleTokenType(ctx context.Context, typeId sdktypes.TokenTypeID) (*sdktypes.FungibleTokenType, error) {
	typez, err := w.tokensClient.GetFungibleTokenTypeHierarchy(ctx, typeId)
	if err != nil {
		return nil, err
	}
	for i := range typez {
		if bytes.Equal(typez[i].ID, typeId) {
			return typez[i], nil
		}
	}
	return nil, nil
}

// GetNonFungibleTokenType returns NonFungibleTokenType or nil if not found
func (w *Wallet) GetNonFungibleTokenType(ctx context.Context, typeId sdktypes.TokenTypeID) (*sdktypes.NonFungibleTokenType, error) {
	typez, err := w.tokensClient.GetNonFungibleTokenTypeHierarchy(ctx, typeId)
	if err != nil {
		return nil, err
	}
	for i := range typez {
		if bytes.Equal(typez[i].ID, typeId) {
			return typez[i], nil
		}
	}
	return nil, nil
}

// ListFungibleTokens returns all fungible tokens for the given accountNumber
func (w *Wallet) ListFungibleTokens(ctx context.Context, accountNumber uint64) ([]*sdktypes.FungibleToken, error) {
	key, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}

	return w.tokensClient.GetFungibleTokens(ctx, key.PubKeyHash.Sha256)
}

// ListNonFungibleTokens returns all non-fungible tokens for the given accountNumber
func (w *Wallet) ListNonFungibleTokens(ctx context.Context, accountNumber uint64) ([]*sdktypes.NonFungibleToken, error) {
	key, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}

	return w.tokensClient.GetNonFungibleTokens(ctx, key.PubKeyHash.Sha256)
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
		wrappers[i] = &accountKey{AccountKey: keys[i], idx: uint64(i)} /* #nosec G115 its unlikely that i exceeds uint64 */
	}
	return wrappers, nil
}

func (w *Wallet) GetFungibleToken(ctx context.Context, tokenID sdktypes.TokenID) (*sdktypes.FungibleToken, error) {
	token, err := w.tokensClient.GetFungibleToken(ctx, tokenID)
	if err != nil {
		return nil, fmt.Errorf("error fetching token %s: %w", tokenID, err)
	}
	if token == nil {
		return nil, fmt.Errorf("token not found: %s", tokenID)
	}
	return token, nil
}

func (w *Wallet) GetNonFungibleToken(ctx context.Context, tokenID sdktypes.TokenID) (*sdktypes.NonFungibleToken, error) {
	token, err := w.tokensClient.GetNonFungibleToken(ctx, tokenID)
	if err != nil {
		return nil, fmt.Errorf("error fetching token %s: %w", tokenID, err)
	}
	if token == nil {
		return nil, fmt.Errorf("token not found: %s", tokenID)
	}
	return token, nil
}

func (w *Wallet) TransferNFT(ctx context.Context, accountNumber uint64, tokenID sdktypes.TokenID, receiverPubKey sdktypes.PubKey, typeOwnerPredicateInputs []*PredicateInput, ownerPredicateInput *PredicateInput) (*SubmissionResult, error) {
	acc, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}
	fcrID, err := w.ensureFeeCredit(ctx, acc.AccountKey, 1)
	if err != nil {
		return nil, err
	}
	token, err := w.GetNonFungibleToken(ctx, tokenID)
	if err != nil {
		return nil, err
	}
	if err = ensureTokenOwnership(acc, token, ownerPredicateInput); err != nil {
		return nil, err
	}
	if token.GetStateLockTx() != nil {
		return nil, errors.New("token is locked")
	}
	roundNumber, err := w.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := token.Transfer(OwnerPredicateFromPubKey(receiverPubKey),
		sdktypes.WithTimeout(roundNumber+txTimeoutRoundCount),
		sdktypes.WithFeeCreditRecordID(fcrID),
		sdktypes.WithMaxFee(w.maxFee),
	)
	if err != nil {
		return nil, err
	}

	sigBytes, err := tx.AuthProofSigBytes()
	if err != nil {
		return nil, err
	}
	typeOwnerProofs, err := newProofs(sigBytes, typeOwnerPredicateInputs)
	if err != nil {
		return nil, err
	}
	ownerProof, err := ownerPredicateInput.Proof(sigBytes)
	if err != nil {
		return nil, err
	}
	err = tx.SetAuthProof(tokens.TransferNonFungibleTokenAuthProof{
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

	return w.submitTx(ctx, tx, accountNumber)
}

func (w *Wallet) SendFungible(ctx context.Context, accountNumber uint64, typeId sdktypes.TokenTypeID, targetAmount uint64, receiverPubKey []byte, ownerPredicateInput *PredicateInput, typeOwnerPredicateInputs []*PredicateInput) (*SubmissionResult, error) {
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
	tokenz, err := w.ListFungibleTokens(ctx, accountNumber)
	if err != nil {
		return nil, err
	}
	if len(tokenz) == 0 {
		return nil, fmt.Errorf("account %d has no tokens", accountNumber)
	}
	var matchingTokens []*sdktypes.FungibleToken
	var totalBalance uint64
	// find the best unit candidate for transfer or split, value must be equal or larger than the target amount
	var closestMatch *sdktypes.FungibleToken
	for _, token := range tokenz {
		if !typeId.Eq(token.TypeID) {
			continue
		}
		if token.StateLockTx != nil {
			continue
		}
		matchingTokens = append(matchingTokens, token)
		var ok bool
		if totalBalance, ok = util.SafeAdd(totalBalance, token.Amount); !ok {
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
	if targetAmount > totalBalance {
		return nil, fmt.Errorf("insufficient tokens of type %s: got %v, need %v", typeId, totalBalance, targetAmount)
	}
	// optimization: first try to make a single operation instead of iterating through all tokens in doSendMultiple
	if closestMatch.Amount >= targetAmount {
		roundNumber, err := w.GetRoundNumber(ctx)
		if err != nil {
			return nil, err
		}
		sub, err := w.prepareSplitOrTransferTx(acc, targetAmount, closestMatch, fcrID, receiverPubKey, roundNumber+txTimeoutRoundCount, ownerPredicateInput, typeOwnerPredicateInputs)
		if err != nil {
			return nil, err
		}
		err = sub.ToBatch(w.tokensClient, w.log).SendTx(ctx, w.confirmTx)
		return newSingleResult(sub, accountNumber), err
	} else {
		return w.doSendMultiple(ctx, targetAmount, matchingTokens, acc, fcrID, receiverPubKey, ownerPredicateInput, typeOwnerPredicateInputs)
	}
}

func (w *Wallet) UpdateNFTData(ctx context.Context, accountNumber uint64, tokenID sdktypes.TokenID, data []byte, tokenDataUpdatePredicateInput *PredicateInput, tokenTypeDataUpdatePredicateInputs []*PredicateInput) (*SubmissionResult, error) {
	acc, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}
	fcrID, err := w.ensureFeeCredit(ctx, acc.AccountKey, 1)
	if err != nil {
		return nil, err
	}
	t, err := w.GetNonFungibleToken(ctx, tokenID)
	if err != nil {
		return nil, err
	}
	if t.GetStateLockTx() != nil {
		return nil, errors.New("token is locked")
	}
	roundNumber, err := w.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := t.Update(data,
		sdktypes.WithTimeout(roundNumber+txTimeoutRoundCount),
		sdktypes.WithFeeCreditRecordID(fcrID),
		sdktypes.WithMaxFee(w.maxFee),
	)
	if err != nil {
		return nil, err
	}

	sigBytes, err := tx.AuthProofSigBytes()
	if err != nil {
		return nil, err
	}
	tokenDataUpdateProof, err := tokenDataUpdatePredicateInput.Proof(sigBytes)
	if err != nil {
		return nil, err
	}
	tokenTypeDataUpdateProofs, err := newProofs(sigBytes, tokenTypeDataUpdatePredicateInputs)
	if err != nil {
		return nil, err
	}
	err = tx.SetAuthProof(tokens.UpdateNonFungibleTokenAuthProof{
		TokenDataUpdateProof:      tokenDataUpdateProof,
		TokenTypeDataUpdateProofs: tokenTypeDataUpdateProofs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to set auth proof: %w", err)
	}
	tx.FeeProof, err = sdktypes.NewP2pkhFeeSignatureFromKey(tx, acc.PrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign tx fee proof: %w", err)
	}

	return w.submitTx(ctx, tx, accountNumber)
}

// SendFungibleByID sends fungible tokens by given unit ID, if amount matches, does the transfer, otherwise splits the token
func (w *Wallet) SendFungibleByID(ctx context.Context, accountNumber uint64, tokenID sdktypes.TokenID, targetAmount uint64, receiverPubKey []byte, typeOwnerPredicateInputs []*PredicateInput) (*SubmissionResult, error) {
	acc, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}
	fcrID, err := w.ensureFeeCredit(ctx, acc.AccountKey, 1)
	if err != nil {
		return nil, err
	}
	token, err := w.GetFungibleToken(ctx, tokenID)
	if err != nil {
		return nil, fmt.Errorf("failed to get token with id=%s: %w", tokenID, err)
	}
	if err = ensureTokenOwnership(acc, token, defaultProof(acc.AccountKey)); err != nil {
		return nil, err
	}
	if targetAmount > token.Amount {
		return nil, fmt.Errorf("insufficient FT value: got %v, need %v", token.Amount, targetAmount)
	}
	roundNumber, err := w.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}

	sub, err := w.prepareSplitOrTransferTx(acc, targetAmount, token, fcrID, receiverPubKey, roundNumber+txTimeoutRoundCount, defaultProof(acc.AccountKey), typeOwnerPredicateInputs)
	if err != nil {
		return nil, err
	}
	err = sub.ToBatch(w.tokensClient, w.log).SendTx(ctx, w.confirmTx)
	return newSingleResult(sub, accountNumber), err
}

func (w *Wallet) GetRoundNumber(ctx context.Context) (uint64, error) {
	roundInfo, err := w.tokensClient.GetRoundInfo(ctx)
	if err != nil {
		return 0, err
	}
	return roundInfo.RoundNumber, nil
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

func (w *Wallet) ensureFeeCredit(ctx context.Context, accountKey *account.AccountKey, txCount uint64) ([]byte, error) {
	fcr, err := w.tokensClient.GetFeeCreditRecordByOwnerID(ctx, accountKey.PubKeyHash.Sha256)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch fee credit record: %w", err)
	}
	if fcr == nil {
		return nil, ErrNoFeeCredit
	}
	maxFee := txCount * w.maxFee
	if fcr.Balance < maxFee {
		return nil, ErrInsufficientFeeCredit
	}
	return fcr.ID, nil
}

func (w *Wallet) LockToken(ctx context.Context, accountNumber uint64, tokenID types.UnitID, ownerPredicateInput *PredicateInput) (*SubmissionResult, error) {
	key, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}
	fcrID, err := w.ensureFeeCredit(ctx, key.AccountKey, 1)
	if err != nil {
		return nil, err
	}
	acc, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}

	tid, err := w.pdr.ExtractUnitType(tokenID)
	if err != nil {
		return nil, fmt.Errorf("extracting token type: %w", err)
	}
	var token Token
	switch tid {
	case tokens.FungibleTokenUnitType:
		token, err = w.GetFungibleToken(ctx, tokenID)
	case tokens.NonFungibleTokenUnitType:
		token, err = w.GetNonFungibleToken(ctx, tokenID)
	default:
		return nil, errors.New("invalid token ID")
	}
	if err != nil {
		return nil, err
	}

	if err = ensureTokenOwnership(key, token, ownerPredicateInput); err != nil {
		return nil, fmt.Errorf("failed to ensure token ownership: %w", err)
	}
	if token.GetStateLockTx() != nil {
		return nil, errors.New("token is already locked")
	}
	roundNumber, err := w.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := token.Lock(wallet.NewP2PKHStateLock(key.PubKeyHash.Sha256),
		sdktypes.WithTimeout(roundNumber+txTimeoutRoundCount),
		sdktypes.WithFeeCreditRecordID(fcrID),
		sdktypes.WithMaxFee(w.maxFee),
	)
	if err != nil {
		return nil, err
	}

	sigBytes, err := tx.AuthProofSigBytes()
	if err != nil {
		return nil, err
	}
	ownerProof, err := ownerPredicateInput.Proof(sigBytes)
	if err != nil {
		return nil, err
	}
	err = tx.SetAuthProof(nop.AuthProof{
		OwnerProof: ownerProof,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to set auth proof: %w", err)
	}
	tx.FeeProof, err = sdktypes.NewP2pkhFeeSignatureFromKey(tx, acc.PrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign tx fee proof: %w", err)
	}

	return w.submitTx(ctx, tx, accountNumber)
}

func (w *Wallet) UnlockToken(ctx context.Context, accountNumber uint64, tokenID sdktypes.TokenID, ownerPredicateInput *PredicateInput) (*SubmissionResult, error) {
	key, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}
	fcrID, err := w.ensureFeeCredit(ctx, key.AccountKey, 1)
	if err != nil {
		return nil, err
	}
	acc, err := w.getAccount(accountNumber)
	if err != nil {
		return nil, err
	}

	tid, err := w.pdr.ExtractUnitType(tokenID)
	if err != nil {
		return nil, fmt.Errorf("extracting token type: %w", err)
	}
	var token Token
	switch tid {
	case tokens.FungibleTokenUnitType:
		token, err = w.GetFungibleToken(ctx, tokenID)
	case tokens.NonFungibleTokenUnitType:
		token, err = w.GetNonFungibleToken(ctx, tokenID)
	default:
		return nil, errors.New("invalid token ID")
	}
	if err != nil {
		return nil, err
	}

	if err = ensureTokenOwnership(key, token, ownerPredicateInput); err != nil {
		return nil, err
	}
	if token.GetStateLockTx() == nil {
		return nil, errors.New("token is already unlocked")
	}
	roundNumber, err := w.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}

	tx, err := token.Unlock(
		sdktypes.WithTimeout(roundNumber+txTimeoutRoundCount),
		sdktypes.WithFeeCreditRecordID(fcrID),
		sdktypes.WithMaxFee(w.maxFee),
	)
	if err != nil {
		return nil, err
	}
	// add state unlock proof
	unlockProof, err := sdktypes.NewP2pkhStateLockSignatureFromKey(tx, acc.PrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create state unlock proof: %w", err)
	}
	tx.AddStateUnlockCommitProof(unlockProof)

	sigBytes, err := tx.AuthProofSigBytes()
	if err != nil {
		return nil, err
	}
	ownerProof, err := ownerPredicateInput.Proof(sigBytes)
	if err != nil {
		return nil, err
	}
	err = tx.SetAuthProof(nop.AuthProof{
		OwnerProof: ownerProof,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to set auth proof: %w", err)
	}
	tx.FeeProof, err = sdktypes.NewP2pkhFeeSignatureFromKey(tx, acc.PrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign tx fee proof: %w", err)
	}

	return w.submitTx(ctx, tx, accountNumber)
}

func (w *Wallet) submitTx(ctx context.Context, tx *types.TransactionOrder, accountNumber uint64) (*SubmissionResult, error) {
	sub, err := txsubmitter.New(tx)
	if err != nil {
		return nil, err
	}
	if err := sub.ToBatch(w.tokensClient, w.log).SendTx(ctx, w.confirmTx); err != nil {
		return nil, err
	}
	return newSingleResult(sub, accountNumber), nil
}

func ensureTokenOwnership(acc *accountKey, token Token, ownerProof *PredicateInput) error {
	if bytes.Equal(token.GetOwnerPredicate(), templates.NewP2pkh256BytesFromKey(acc.PubKey)) {
		return nil
	}
	predicate, err := extractPredicate(token.GetOwnerPredicate())
	if err != nil {
		return err
	}
	isP2pkhPredicate := templates.VerifyP2pkhPredicate(predicate) == nil
	if !isP2pkhPredicate && ownerProof != nil && ownerProof.AccountKey == nil && ownerProof.Argument != nil {
		// this must be a "custom predicate" with provided owner proof
		return nil
	}
	return fmt.Errorf("token '%s' does not belong to account #%d", token.GetID(), acc.AccountNumber())
}

func defaultProof(accountKey *account.AccountKey) *PredicateInput {
	return &PredicateInput{AccountKey: accountKey}
}

func extractPredicate(predicateBytes []byte) (*predicates.Predicate, error) {
	predicate := &predicates.Predicate{}
	if err := types.Cbor.Unmarshal(predicateBytes, predicate); err != nil {
		return nil, err
	}
	return predicate, nil
}

func newProofs(sigBytes []byte, predicateInputs []*PredicateInput) ([][]byte, error) {
	var predicateSigs [][]byte
	for _, predicateInput := range predicateInputs {
		predicateSig, err := predicateInput.Proof(sigBytes)
		if err != nil {
			return nil, err
		}
		predicateSigs = append(predicateSigs, predicateSig)
	}
	return predicateSigs, nil
}
