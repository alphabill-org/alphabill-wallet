package money

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/hash"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"

	"github.com/alphabill-org/alphabill-wallet/util"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/fees"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/api"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/dc"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/tx_builder"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
)

const (
	txTimeoutBlockCount       = 10
	maxBillsForDustCollection = 100
)

type (
	Wallet struct {
		am            account.Manager
		rpcClient     RpcClient
		feeManager    *fees.FeeManager
		TxPublisher   *TxPublisher
		dustCollector *dc.DustCollector
		log           *slog.Logger
	}

	RpcClient interface {
		GetRoundNumber(ctx context.Context) (uint64, error)
		GetBill(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.Bill, error)
		GetFeeCreditRecord(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.FeeCreditBill, error)
		GetUnitsByOwnerID(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error)
		SendTransaction(ctx context.Context, tx *types.TransactionOrder) ([]byte, error)
		GetTransactionProof(ctx context.Context, txHash types.Bytes) (*types.TransactionRecord, *types.TxProof, error)
		GetBlock(ctx context.Context, roundNumber uint64) (*types.Block, error)
	}

	SendCmd struct {
		Receivers           []ReceiverData
		WaitForConfirmation bool
		AccountIndex        uint64
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

// CreateNewWallet creates a new wallet. To synchronize wallet with a node call Sync.
// Shutdown needs to be called to release resources used by wallet.
// If mnemonic seed is empty then new mnemonic will ge generated, otherwise wallet is restored using given mnemonic.
func CreateNewWallet(am account.Manager, mnemonic string) error {
	return createMoneyWallet(mnemonic, am)
}

func LoadExistingWallet(am account.Manager, feeManagerDB fees.FeeManagerDB, rpcClient RpcClient, log *slog.Logger) (*Wallet, error) {
	moneySystemID := money.DefaultSystemIdentifier
	moneyTxPublisher := NewTxPublisher(rpcClient, log)
	feeManager := fees.NewFeeManager(am, feeManagerDB, moneySystemID, rpcClient, FeeCreditRecordIDFormPublicKey, moneySystemID, rpcClient, FeeCreditRecordIDFormPublicKey, log)
	dustCollector := dc.NewDustCollector(moneySystemID, maxBillsForDustCollection, txTimeoutBlockCount, rpcClient, log)
	return &Wallet{
		am:            am,
		rpcClient:     rpcClient,
		TxPublisher:   moneyTxPublisher,
		feeManager:    feeManager,
		dustCollector: dustCollector,
		log:           log,
	}, nil
}

func (w *Wallet) GetAccountManager() account.Manager {
	return w.am
}

func (w *Wallet) SystemID() types.SystemID {
	// TODO: return the default "AlphaBill Money System ID" for now
	// but this should come from config (base wallet? AB client?)
	return money.DefaultSystemIdentifier
}

// Close terminates connection to alphabill node, closes account manager and cancels any background goroutines.
func (w *Wallet) Close() {
	w.am.Close()
	w.feeManager.Close()
	_ = w.dustCollector.Close()
}

// GetBalance returns the total value of all bills currently held in the wallet, for the given account,
// in Tema denomination. Does not count fee credit bills.
func (w *Wallet) GetBalance(ctx context.Context, cmd GetBalanceCmd) (uint64, error) {
	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return 0, fmt.Errorf("failed to load account key: %w", err)
	}
	ownerID := accountKey.PubKeyHash.Sha256
	bills, err := api.FetchBills(ctx, w.rpcClient, ownerID)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch bills: %w", err)
	}
	var sum uint64
	for _, bill := range bills {
		sum += bill.Value()
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
	return w.rpcClient.GetRoundNumber(ctx)
}

// Send creates, signs and broadcasts transactions, in total for the given amount,
// to the given public key, the public key must be in compressed secp256k1 format.
// Sends one transaction per bill, prioritizing larger bills.
// Waits for initial response from the node, returns error if any transaction was not accepted to the mempool.
// Returns list of tx proofs, if waitForConfirmation=true, otherwise nil.
func (w *Wallet) Send(ctx context.Context, cmd SendCmd) ([]*wallet.Proof, error) {
	if err := cmd.isValid(); err != nil {
		return nil, err
	}

	pubKey, err := w.am.GetPublicKey(cmd.AccountIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to load public key: %w", err)
	}

	roundNumber, err := w.rpcClient.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}

	k, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}

	fcb, err := w.GetFeeCredit(ctx, fees.GetFeeCreditCmd{AccountIndex: cmd.AccountIndex})
	if err != nil {
		return nil, err
	}
	if fcb == nil {
		return nil, errors.New("no fee credit in money wallet")
	}

	bills, err := w.getUnlockedBills(ctx, hash.Sum256(pubKey))
	if err != nil {
		return nil, err
	}
	var balance uint64
	for _, b := range bills {
		balance += b.Value()
	}
	totalAmount := cmd.totalAmount()
	if totalAmount > balance {
		return nil, errors.New("insufficient balance for transaction")
	}
	timeout := roundNumber + txTimeoutBlockCount
	batch := txsubmitter.NewBatch(k.PubKey, w.rpcClient, w.log)

	var txs []*types.TransactionOrder
	if len(cmd.Receivers) > 1 {
		// if more than one receiver then perform transaction as N-way split and require sufficiently large bill
		largestBill := bills[0]
		if largestBill.Value() < totalAmount {
			return nil, fmt.Errorf("sending to multiple addresses is performed using N-way split transaction which "+
				"requires a single sufficiently large bill, wallet needs a bill with at least %s tema value, "+
				"largest bill in wallet currently is %s tema",
				util.AmountToString(totalAmount+1, 8), // +1 because 0 remaining value is not allowed
				util.AmountToString(largestBill.Value(), 8))
		}
		if largestBill.Value() == totalAmount {
			return nil, errors.New("sending to multiple addresses is performed using N-way split transaction " +
				"which requires a single sufficiently large bill and cannot result in a bill with 0 value after the " +
				"transaction")
		}
		// convert send cmd targets to transaction units
		var targetUnits []*money.TargetUnit
		for _, r := range cmd.Receivers {
			targetUnits = append(targetUnits, &money.TargetUnit{
				Amount:         r.Amount,
				OwnerCondition: templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(r.PubKey)),
			})
		}
		remainingValue := largestBill.Value() - totalAmount
		tx, err := tx_builder.NewSplitTx(targetUnits, remainingValue, k, w.SystemID(), largestBill, timeout, fcb.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to create N-way split tx: %w", err)
		}
		txs = append(txs, tx)
	} else {
		// if single receiver then perform up to N transfers (until target amount is reached)
		txs, err = tx_builder.CreateTransactions(cmd.Receivers[0].PubKey, cmd.Receivers[0].Amount, w.SystemID(), bills, k, timeout, fcb.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to create transactions: %w", err)
		}
	}

	for _, tx := range txs {
		batch.Add(&txsubmitter.TxSubmission{
			UnitID:      tx.UnitID(),
			TxHash:      tx.Hash(crypto.SHA256),
			Transaction: tx,
		})
	}

	txsCost := tx_builder.MaxFee * uint64(len(batch.Submissions()))
	if fcb.Balance() < txsCost {
		return nil, errors.New("insufficient fee credit balance for transaction(s)")
	}

	if err = batch.SendTx(ctx, cmd.WaitForConfirmation); err != nil {
		return nil, err
	}

	var proofs []*wallet.Proof
	for _, txSub := range batch.Submissions() {
		proofs = append(proofs, txSub.Proof)
	}
	return proofs, nil
}

// SendTx sends tx and waits for confirmation, returns tx proof
func (w *Wallet) SendTx(ctx context.Context, tx *types.TransactionOrder, senderPubKey []byte) (*wallet.Proof, error) {
	return w.TxPublisher.SendTx(ctx, tx, senderPubKey)
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

// GetFeeCredit returns fee credit bill for given account,
// can return nil if fee credit bill has not been created yet.
func (w *Wallet) GetFeeCredit(ctx context.Context, cmd fees.GetFeeCreditCmd) (*api.FeeCreditBill, error) {
	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to load account key: %w", err)
	}
	return w.GetFeeCreditBill(ctx, money.NewFeeCreditRecordID(nil, accountKey.PubKeyHash.Sha256))
}

// GetFeeCreditBill returns fee credit bill for given unitID,
// returns nil if fee credit bill has not been created yet.
func (w *Wallet) GetFeeCreditBill(ctx context.Context, unitID types.UnitID) (*api.FeeCreditBill, error) {
	return api.FetchFeeCreditBill(ctx, w.rpcClient, unitID)
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

func (w *Wallet) getUnlockedBills(ctx context.Context, ownerID []byte) ([]*api.Bill, error) {
	var unlockedBills []*api.Bill
	bills, err := api.FetchBills(ctx, w.rpcClient, ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bills: %w", err)
	}
	// sort bills by value largest first
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Value() > bills[j].Value()
	})
	// filter locked bills
	for _, b := range bills {
		if !b.IsLocked() {
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
