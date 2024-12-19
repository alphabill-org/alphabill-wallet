package money

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/util"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/fees"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/dc"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/txbuilder"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
)

const (
	txTimeoutBlockCount       = 10
	maxBillsForDustCollection = 100
)

type (
	Wallet struct {
		pdr           *types.PartitionDescriptionRecord
		am            account.Manager
		moneyClient   sdktypes.MoneyPartitionClient
		feeManager    *fees.FeeManager
		dustCollector *dc.DustCollector
		maxFee        uint64
		log           *slog.Logger
	}

	SendCmd struct {
		Receivers           []ReceiverData
		WaitForConfirmation bool
		AccountIndex        uint64
		ReferenceNumber     []byte
		MaxFee              uint64
	}

	ReceiverData struct {
		PubKey []byte
		Amount uint64
	}

	GetBalanceCmd struct {
		AccountIndex uint64

		// TODO deprecated: if transferDC is sent then the owner of bill becomes dust collector,
		// wallet needs to locally keep track of unswapped bills
		CountDCBills bool
	}

	DustCollectionResult struct {
		AccountIndex         uint64
		DustCollectionResult *dc.DustCollectionResult // NB! can be nil
	}
)

// GenerateKeys generates the first account key and stores it in the account manager along with the mnemonic seed,
// does nothing if the account manager already contains keys.
// If the mnemonic seed is empty then a random mnemonic will be used.
func GenerateKeys(am account.Manager, mnemonic string) error {
	return createMoneyWallet(mnemonic, am)
}

// NewWallet creates a new money wallet from specified parameters. The account manager must contain pre-generated keys.
func NewWallet(ctx context.Context, am account.Manager, feeManagerDB fees.FeeManagerDB, moneyClient sdktypes.MoneyPartitionClient, maxFee uint64, log *slog.Logger) (*Wallet, error) {
	pdr, err := moneyClient.PartitionDescription(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading partition description: %w", err)
	}
	if pdr.PartitionTypeID != money.PartitionTypeID {
		return nil, fmt.Errorf("invalid rpc url: expected money partition (%d) node reports partition type %d", money.PartitionTypeID, pdr.PartitionTypeID)
	}
	fcrGen := func(shard types.ShardID, pubKey []byte, latestAdditionTime uint64) (types.UnitID, error) {
		return money.NewFeeCreditRecordIDFromPublicKey(pdr, shard, pubKey, latestAdditionTime)
	}

	feeManager := fees.NewFeeManager(pdr.NetworkID, am, feeManagerDB,
		pdr.PartitionID, moneyClient, fcrGen,
		pdr.PartitionID, moneyClient, fcrGen,
		maxFee, log,
	)
	dustCollector := dc.NewDustCollector(maxBillsForDustCollection, txTimeoutBlockCount, moneyClient, maxFee, log)
	return &Wallet{
		pdr:           pdr,
		am:            am,
		moneyClient:   moneyClient,
		feeManager:    feeManager,
		dustCollector: dustCollector,
		maxFee:        maxFee,
		log:           log,
	}, nil
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

// Close terminates connection to alphabill node, closes account manager and cancels any background goroutines.
func (w *Wallet) Close() {
	w.am.Close()
	w.feeManager.Close()
	_ = w.dustCollector.Close()
	w.moneyClient.Close()
}

// GetBalance returns the total value of all bills currently held in the wallet, for the given account,
// in Tema denomination. Does not count fee credit bills.
func (w *Wallet) GetBalance(ctx context.Context, cmd GetBalanceCmd) (uint64, error) {
	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return 0, fmt.Errorf("failed to load account key: %w", err)
	}
	ownerID := accountKey.PubKeyHash.Sha256
	bills, err := w.moneyClient.GetBills(ctx, ownerID)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch bills: %w", err)
	}
	var sum uint64
	for _, bill := range bills {
		sum += bill.Value
	}
	return sum, nil
}

// GetBalances returns the total value of all bills currently held in the wallet, for all accounts,
// in Tema denomination. Does not count fee credit bills.
func (w *Wallet) GetBalances(ctx context.Context, cmd GetBalanceCmd) ([]uint64, uint64, error) {
	accountKeys, err := w.am.GetAccountKeys()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to load account keys: %w", err)
	}
	accountTotals := make([]uint64, len(accountKeys))
	var total uint64
	for accountIndex := range accountKeys {
		balance, err := w.GetBalance(ctx, GetBalanceCmd{AccountIndex: uint64(accountIndex), CountDCBills: cmd.CountDCBills})
		if err != nil {
			return nil, 0, err
		}
		total += balance
		accountTotals[accountIndex] = balance
	}
	return accountTotals, total, err
}

// GetRoundNumber returns the latest round number in node.
func (w *Wallet) GetRoundNumber(ctx context.Context) (uint64, error) {
	return w.moneyClient.GetRoundNumber(ctx)
}

// Send creates, signs and broadcasts transactions, in total for the given amount,
// to the given public key, the public key must be in compressed secp256k1 format.
// Sends one transaction per bill, prioritizing larger bills.
// Waits for initial response from the node, returns error if any transaction was not accepted to the mempool.
// Returns list of tx proofs, if waitForConfirmation=true, otherwise nil.
func (w *Wallet) Send(ctx context.Context, cmd SendCmd) ([]*types.TxRecordProof, error) {
	if err := cmd.isValid(); err != nil {
		return nil, err
	}

	pubKey, err := w.am.GetPublicKey(cmd.AccountIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to load public key: %w", err)
	}

	roundNumber, err := w.moneyClient.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}

	k, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}

	fcr, err := w.moneyClient.GetFeeCreditRecordByOwnerID(ctx, k.PubKeyHash.Sha256)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch fee credit record: %w", err)
	}
	if fcr == nil {
		return nil, errors.New("fee credit record not found")
	}

	bills, err := w.getUnlockedBills(ctx, hash.Sum256(pubKey))
	if err != nil {
		return nil, err
	}
	var balance uint64
	for _, b := range bills {
		balance += b.Value
	}
	totalAmount := cmd.totalAmount()
	if totalAmount > balance {
		return nil, errors.New("insufficient balance for transaction")
	}
	timeout := roundNumber + txTimeoutBlockCount
	batch := txsubmitter.NewBatch(w.moneyClient, w.log)

	txSigner, err := sdktypes.NewMoneyTxSignerFromKey(k.PrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create money tx signer: %w", err)
	}

	var txs []*types.TransactionOrder
	if len(cmd.Receivers) > 1 {
		// if more than one receiver then perform transaction as N-way split and require sufficiently large bill
		largestBill := bills[0]
		if largestBill.Value < totalAmount {
			return nil, fmt.Errorf("sending to multiple addresses is performed using N-way split transaction which "+
				"requires a single sufficiently large bill, wallet needs a bill with at least %s tema value, "+
				"largest bill in wallet currently is %s tema",
				util.AmountToString(totalAmount+1, 8), // +1 because 0 remaining value is not allowed
				util.AmountToString(largestBill.Value, 8))
		}
		if largestBill.Value == totalAmount {
			return nil, errors.New("sending to multiple addresses is performed using N-way split transaction " +
				"which requires a single sufficiently large bill and cannot result in a bill with 0 value after the " +
				"transaction")
		}
		// convert send cmd targets to transaction units
		var targetUnits []*money.TargetUnit
		for _, r := range cmd.Receivers {
			targetUnits = append(targetUnits, &money.TargetUnit{
				Amount:         r.Amount,
				OwnerPredicate: templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(r.PubKey)),
			})
		}
		tx, err := largestBill.Split(targetUnits,
			sdktypes.WithTimeout(timeout),
			sdktypes.WithFeeCreditRecordID(fcr.ID),
			sdktypes.WithMaxFee(cmd.MaxFee),
			sdktypes.WithReferenceNumber(cmd.ReferenceNumber),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create N-way split tx: %w", err)
		}
		if err = txSigner.SignTx(tx); err != nil {
			return nil, fmt.Errorf("failed to sign tx: %w", err)
		}
		txs = append(txs, tx)
	} else {
		// if single receiver then perform up to N transfers (until target amount is reached)
		txs, err = txbuilder.CreateTransactions(cmd.Receivers[0].PubKey, cmd.Receivers[0].Amount, bills, txSigner, timeout, fcr.ID, cmd.ReferenceNumber, cmd.MaxFee)
		if err != nil {
			return nil, fmt.Errorf("failed to create transactions: %w", err)
		}
	}

	for _, tx := range txs {
		sub, err := txsubmitter.New(tx)
		if err != nil {
			return nil, fmt.Errorf("failed to create tx submission: %w", err)
		}
		batch.Add(sub)
	}

	txsCost := cmd.MaxFee * uint64(len(batch.Submissions()))
	if fcr.Balance < txsCost {
		return nil, errors.New("insufficient fee credit balance for transaction(s)")
	}

	if err = batch.SendTx(ctx, cmd.WaitForConfirmation); err != nil {
		return nil, err
	}

	var proofs []*types.TxRecordProof
	for _, txSub := range batch.Submissions() {
		proofs = append(proofs, txSub.Proof)
	}
	return proofs, nil
}

// GetFeeCredit returns fee credit record for the given account,
// can return nil if fee credit record has not been created yet.
// Deprecated: faucet still uses, will be removed
func (w *Wallet) GetFeeCredit(ctx context.Context, cmd fees.GetFeeCreditCmd) (*sdktypes.FeeCreditRecord, error) {
	ac, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}
	return w.moneyClient.GetFeeCreditRecordByOwnerID(ctx, ac.PubKeyHash.Sha256)
}

// AddFeeCredit creates fee credit for the given amount.
// Wallet must have a bill large enough for the required amount plus fees.
// Returns transferFC and addFC transaction proofs.
func (w *Wallet) AddFeeCredit(ctx context.Context, cmd fees.AddFeeCmd) (*fees.AddFeeCmdResponse, error) {
	return w.feeManager.AddFeeCredit(ctx, cmd)
}

// ReclaimFeeCredit reclaims fee credit.
// Reclaimed fee credit is added to the largest bill in wallet.
// Returns closeFC and reclaimFC transaction proofs.
func (w *Wallet) ReclaimFeeCredit(ctx context.Context, cmd fees.ReclaimFeeCmd) (*fees.ReclaimFeeCmdResponse, error) {
	return w.feeManager.ReclaimFeeCredit(ctx, cmd)
}

// CollectDust starts the dust collector process for the requested accounts in the wallet.
// Dust collection process joins up to N units into existing target unit, prioritizing small units first.
// The largest unit in wallet is selected as the target unit.
// If accountNumber is equal to 0 then dust collection is run for all accounts, returns list of swap tx proofs
// together with account numbers, the proof can be nil if swap tx was not sent e.g. if there's not enough bills to swap.
// If accountNumber is greater than 0 then dust collection is run only for the specific account, returns single swap tx
// proof, the proof can be nil e.g. if there's not enough bills to swap.
func (w *Wallet) CollectDust(ctx context.Context, accountNumber uint64) ([]*DustCollectionResult, error) {
	var res []*DustCollectionResult
	if accountNumber == 0 {
		for _, acc := range w.am.GetAll() {
			accKey, err := w.am.GetAccountKey(acc.AccountIndex)
			if err != nil {
				return nil, fmt.Errorf("failed to load account key: %w", err)
			}
			dcResult, err := w.dustCollector.CollectDust(ctx, accKey)
			if err != nil {
				return nil, fmt.Errorf("dust collection failed for account number %d: %w", acc.AccountIndex+1, err)
			}
			res = append(res, &DustCollectionResult{AccountIndex: acc.AccountIndex, DustCollectionResult: dcResult})
		}
	} else {
		accKey, err := w.am.GetAccountKey(accountNumber - 1)
		if err != nil {
			return nil, fmt.Errorf("failed to load account key: %w", err)
		}
		dcResult, err := w.dustCollector.CollectDust(ctx, accKey)
		if err != nil {
			return nil, fmt.Errorf("dust collection failed for account number %d: %w", accountNumber, err)
		}
		res = append(res, &DustCollectionResult{AccountIndex: accountNumber - 1, DustCollectionResult: dcResult})
	}
	return res, nil
}

func (w *Wallet) getUnlockedBills(ctx context.Context, ownerID []byte) ([]*sdktypes.Bill, error) {
	var unlockedBills []*sdktypes.Bill
	bills, err := w.moneyClient.GetBills(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bills: %w", err)
	}
	// sort bills by value largest first
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Value > bills[j].Value
	})
	// filter locked bills
	for _, b := range bills {
		if b.LockStatus == 0 {
			unlockedBills = append(unlockedBills, b)
		}
	}
	return unlockedBills, nil
}

func (c *SendCmd) isValid() error {
	if len(c.Receivers) == 0 {
		return errors.New("receivers is empty")
	}
	for _, r := range c.Receivers {
		if len(r.PubKey) != abcrypto.CompressedSecp256K1PublicKeySize {
			return fmt.Errorf("invalid public key: public key must be in compressed secp256k1 format: "+
				"got %d bytes, expected %d bytes for public key 0x%x", len(r.PubKey), abcrypto.CompressedSecp256K1PublicKeySize, r.PubKey)
		}
		if r.Amount == 0 {
			return errors.New("invalid amount: amount must be greater than zero")
		}
	}
	return nil
}

func (c *SendCmd) totalAmount() uint64 {
	var sum uint64
	for _, r := range c.Receivers {
		sum += r.Amount
	}
	return sum
}

func createMoneyWallet(mnemonic string, am account.Manager) error {
	// load accounts from account manager
	accountKeys, err := am.GetAccountKeys()
	if err != nil {
		return fmt.Errorf("failed to check does account have any keys: %w", err)
	}
	// create keys in account manager if not exists
	if len(accountKeys) == 0 {
		// creating keys also adds the first account
		if err = am.CreateKeys(mnemonic); err != nil {
			return fmt.Errorf("failed to create keys for the account: %w", err)
		}
		// reload accounts after adding the first account
		accountKeys, err = am.GetAccountKeys()
		if err != nil {
			return fmt.Errorf("failed to read account keys: %w", err)
		}
		if len(accountKeys) == 0 {
			return errors.New("failed to create key for the first account")
		}
	}
	return nil
}
