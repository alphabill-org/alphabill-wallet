package fees

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/types"

	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/api"
	txbuilder "github.com/alphabill-org/alphabill-wallet/wallet/money/tx_builder"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
	"github.com/alphabill-org/alphabill-wallet/wallet/unitlock"
)

const (
	MinimumFeeAmount             = 4 * txbuilder.MaxFee
	txTimeoutBlockCount          = 10
	transferFCLatestAdditionTime = 65536 // relative timeout after which transferFC unit becomes unusable
)

var (
	ErrMinimumFeeAmount    = errors.New("insufficient fee amount")
	ErrInsufficientBalance = errors.New("insufficient balance for transaction")
	ErrInvalidPartition    = errors.New("pending fee credit process for another partition")
)

type (
	RpcClient interface {
		GetRoundNumber(ctx context.Context) (uint64, error)
		GetBill(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.Bill, error)
		GetFeeCreditRecord(ctx context.Context, unitID types.UnitID, includeStateProof bool) (*api.FeeCreditBill, error)
		GetUnitsByOwnerID(ctx context.Context, ownerID types.Bytes) ([]types.UnitID, error)
		SendTransaction(ctx context.Context, tx *types.TransactionOrder) ([]byte, error)
		GetTransactionProof(ctx context.Context, txHash types.Bytes) (*types.TransactionRecord, *types.TxProof, error)
	}

	// GenerateFcrIDFromPublicKey function to generate fee credit UnitID from shard number nad public key
	GenerateFcrIDFromPublicKey func(shardPart, pubKey []byte) types.UnitID

	FeeManagerDB interface {
		GetAddFeeContext(accountID []byte) (*AddFeeCreditCtx, error)
		SetAddFeeContext(accountID []byte, feeCtx *AddFeeCreditCtx) error
		DeleteAddFeeContext(accountID []byte) error
		GetReclaimFeeContext(accountID []byte) (*ReclaimFeeCreditCtx, error)
		SetReclaimFeeContext(accountID []byte, feeCtx *ReclaimFeeCreditCtx) error
		DeleteReclaimFeeContext(accountID []byte) error
		Close() error
	}

	FeeManager struct {
		am  account.Manager
		db  FeeManagerDB
		log *slog.Logger

		// money partition fields
		moneySystemID         types.SystemID
		moneyClient           RpcClient
		moneyPartitionFcrIDFn GenerateFcrIDFromPublicKey

		// target partition fields
		targetPartitionSystemID types.SystemID
		targetPartitionClient   RpcClient
		targetPartitionFcrIDFn  GenerateFcrIDFromPublicKey
	}

	GetFeeCreditCmd struct {
		AccountIndex uint64
	}

	AddFeeCmd struct {
		AccountIndex   uint64
		Amount         uint64
		DisableLocking bool // if true then lockFC transaction is not sent before adding fee credit
	}

	ReclaimFeeCmd struct {
		AccountIndex   uint64
		DisableLocking bool // if true then lock transaction is not sent before reclaiming fee credit
	}

	LockFeeCreditCmd struct {
		AccountIndex uint64
		LockStatus   uint64
	}

	UnlockFeeCreditCmd struct {
		AccountIndex uint64
	}

	AddFeeCmdResponse struct {
		Proofs []*AddFeeTxProofs
	}

	ReclaimFeeCmdResponse struct {
		Proofs *ReclaimFeeTxProofs
	}

	AddFeeTxProofs struct {
		LockFC     *wallet.Proof
		TransferFC *wallet.Proof
		AddFC      *wallet.Proof
	}

	ReclaimFeeTxProofs struct {
		Lock      *wallet.Proof
		CloseFC   *wallet.Proof
		ReclaimFC *wallet.Proof
	}

	AddFeeCreditCtx struct {
		TargetPartitionID  types.SystemID          `json:"targetPartitionId"`         // target partition id where the fee is being added to
		TargetBillID       []byte                  `json:"targetBillId"`              // transferFC target bill id
		TargetBillBacklink []byte                  `json:"targetBillBacklink"`        // transferFC target bill backlink
		TargetAmount       uint64                  `json:"targetAmount"`              // the amount to add to the fee credit bill
		LockingDisabled    bool                    `json:"lockingDisabled,omitempty"` // user defined flag if we should lock fee credit record when adding fees
		LockFCTx           *types.TransactionOrder `json:"lockFCTx,omitempty"`
		LockFCProof        *wallet.Proof           `json:"lockFCProof,omitempty"`
		TransferFCTx       *types.TransactionOrder `json:"transferFCTx,omitempty"`
		TransferFCProof    *wallet.Proof           `json:"transferFCProof,omitempty"`
		AddFCTx            *types.TransactionOrder `json:"addFCTx,omitempty"`
		AddFCProof         *wallet.Proof           `json:"addFCProof,omitempty"`
	}

	ReclaimFeeCreditCtx struct {
		TargetPartitionID  types.SystemID          `json:"targetPartitionId"`  // target partition id where the fee credit is being reclaimed from
		TargetBillID       []byte                  `json:"targetBillId"`       // closeFC target bill id
		TargetBillBacklink []byte                  `json:"targetBillBacklink"` // closeFC target bill backlink
		LockingDisabled    bool                    `json:"lockingDisabled,omitempty"`
		LockTx             *types.TransactionOrder `json:"lockTx,omitempty"`
		LockTxProof        *wallet.Proof           `json:"lockTxProof,omitempty"`
		CloseFCTx          *types.TransactionOrder `json:"closeFCTx,omitempty"`
		CloseFCProof       *wallet.Proof           `json:"closeFCProof,omitempty"`
		ReclaimFCTx        *types.TransactionOrder `json:"reclaimFCTx,omitempty"`
		ReclaimFCProof     *wallet.Proof           `json:"reclaimFCProof,omitempty"`
	}
)

// NewFeeManager creates new fee credit manager.
// Parameters:
// - account manager
// - fee manager db
//
// - money partition:
//   - systemID
//   - rpc node client
//   - fee credit record id generation function
//
// - target partition:
//   - systemID
//   - rpc node client
//   - fee credit record id generation function
func NewFeeManager(
	am account.Manager,
	db FeeManagerDB,
	moneySystemID types.SystemID,
	moneyClient RpcClient,
	moneyPartitionFcrIDFn GenerateFcrIDFromPublicKey,
	targetPartitionSystemID types.SystemID,
	targetPartitionClient RpcClient,
	fcrIDFn GenerateFcrIDFromPublicKey,
	log *slog.Logger,
) *FeeManager {
	return &FeeManager{
		am:                      am,
		db:                      db,
		moneySystemID:           moneySystemID,
		moneyClient:             moneyClient,
		moneyPartitionFcrIDFn:   moneyPartitionFcrIDFn,
		targetPartitionSystemID: targetPartitionSystemID,
		targetPartitionClient:   targetPartitionClient,
		targetPartitionFcrIDFn:  fcrIDFn,
		log:                     log,
	}
}

// AddFeeCredit creates fee credit for the given amount. If the wallet does not have a bill large enough for the
// required amount, multiple bills are used until the target amount is reached. In case of partial add
// (the add process was previously left in an incomplete state) only the partial bill is added to fee credit.
// Returns transaction proofs that were used to add credit.
func (w *FeeManager) AddFeeCredit(ctx context.Context, cmd AddFeeCmd) (*AddFeeCmdResponse, error) {
	if err := cmd.isValid(); err != nil {
		return nil, err
	}
	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to load account key: %w", err)
	}

	// if partial reclaim exists, ask user to finish the reclaim process first
	reclaimFeeContext, err := w.db.GetReclaimFeeContext(accountKey.PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load reclaim fee context: %w", err)
	}
	if reclaimFeeContext != nil {
		return nil, errors.New("wallet contains unreclaimed fee credit, run the reclaim command before adding fee credit")
	}
	// if partial add process exists, finish it first
	addFeeCtx, err := w.db.GetAddFeeContext(accountKey.PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load fee manager context: %w", err)
	}
	if addFeeCtx != nil {
		// verify fee ctx exists for current partition
		if addFeeCtx.TargetPartitionID != w.targetPartitionSystemID {
			return nil, fmt.Errorf("%w: pendingProcessSystemID=%s, providedSystemID=%s",
				ErrInvalidPartition, addFeeCtx.TargetPartitionID, w.targetPartitionSystemID)
		}
		// handle the pending fee credit process
		feeTxProofs, err := w.addFeeCredit(ctx, accountKey, addFeeCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to complete pending fee credit addition process: %w", err)
		}
		// delete fee context
		if err := w.db.DeleteAddFeeContext(accountKey.PubKey); err != nil {
			return nil, fmt.Errorf("failed to delete add fee context: %w", err)
		}
		return &AddFeeCmdResponse{Proofs: []*AddFeeTxProofs{feeTxProofs}}, nil
	}

	// if no fee context found, run normal fee process
	fees, err := w.addFees(ctx, accountKey, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to complete fee credit addition process: %w", err)
	}
	return fees, nil
}

// ReclaimFeeCredit reclaims fee credit i.e. reclaims entire fee credit bill balance back to the main balance.
// Reclaimed fee credit is added to the largest bill in wallet.
// Returns transaction proofs that were used to reclaim fee credit.
func (w *FeeManager) ReclaimFeeCredit(ctx context.Context, cmd ReclaimFeeCmd) (*ReclaimFeeCmdResponse, error) {
	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to load account key: %w", err)
	}

	// if partial add process exists, finish it first
	addFeeCtx, err := w.db.GetAddFeeContext(accountKey.PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load fee manager context: %w", err)
	}
	if addFeeCtx != nil {
		return nil, errors.New("wallet contains unadded fee credit, run the add command before reclaiming fee credit")
	}

	reclaimFeeCtx, err := w.db.GetReclaimFeeContext(accountKey.PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load fee context: %w", err)
	}
	if reclaimFeeCtx != nil {
		// verify fee ctx exists for current partition
		if reclaimFeeCtx.TargetPartitionID != w.targetPartitionSystemID {
			return nil, fmt.Errorf("%w: pendingProcessSystemID=%s, providedSystemID=%s",
				ErrInvalidPartition, reclaimFeeCtx.TargetPartitionID, w.targetPartitionSystemID)
		}
		// handle the pending fee credit process
		feeTxProofs, err := w.reclaimFeeCredit(ctx, accountKey, reclaimFeeCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to complete pending fee credit reclaim process: %w", err)
		}
		// delete fee ctx
		if err := w.db.DeleteReclaimFeeContext(accountKey.PubKey); err != nil {
			return nil, fmt.Errorf("failed to delete reclaim fee context: %w", err)
		}
		return &ReclaimFeeCmdResponse{Proofs: feeTxProofs}, nil
	}

	// if no locked bill found, run normal reclaim process, selecting the largest bill as target
	fees, err := w.reclaimFees(ctx, accountKey, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to complete fee credit reclaim process: %w", err)
	}
	return fees, err
}

// GetFeeCredit returns fee credit bill for given account, returns nil if fee credit bill has not been created yet.
func (w *FeeManager) GetFeeCredit(ctx context.Context, cmd GetFeeCreditCmd) (*api.FeeCreditBill, error) {
	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to load account key: %w", err)
	}
	return w.fetchTargetPartitionFCB(ctx, accountKey)
}

// LockFeeCredit locks fee credit bill for given account, returns error if fee credit bill has not been created yet
// or is already locked.
func (w *FeeManager) LockFeeCredit(ctx context.Context, cmd LockFeeCreditCmd) (*wallet.Proof, error) {
	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to load account key: %w", err)
	}
	fcb, err := w.fetchTargetPartitionFCB(ctx, accountKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch fee credit: %w", err)
	}
	if fcb == nil {
		return nil, errors.New("fee credit bill does not exist")
	}
	if fcb.Balance() < 2*txbuilder.MaxFee {
		return nil, errors.New("not enough fee credit in wallet")
	}
	if fcb.IsLocked() {
		return nil, fmt.Errorf("fee credit bill is already locked")
	}
	timeout, err := w.getTargetPartitionTimeout(ctx)
	if err != nil {
		return nil, err
	}
	tx, err := txbuilder.NewLockFCTx(accountKey, w.targetPartitionSystemID, fcb, cmd.LockStatus, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create lockFC transaction: %w", err)
	}
	proof, err := sendTx(ctx, tx, accountKey.PubKey, w.targetPartitionClient, w.log)
	if err != nil {
		return nil, fmt.Errorf("failed to send lockFC transaction: %w", err)
	}
	return proof, nil
}

// UnlockFeeCredit unlocks fee credit bill for given account, returns error if fee credit bill has not been created yet
// or is already unlocked.
func (w *FeeManager) UnlockFeeCredit(ctx context.Context, cmd UnlockFeeCreditCmd) (*wallet.Proof, error) {
	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to load account key: %w", err)
	}
	fcb, err := w.fetchTargetPartitionFCB(ctx, accountKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch fee credit: %w", err)
	}
	if fcb.Balance() == 0 {
		return nil, errors.New("no fee credit in wallet")
	}
	if !fcb.IsLocked() {
		return nil, fmt.Errorf("fee credit bill is already unlocked")
	}
	timeout, err := w.getTargetPartitionTimeout(ctx)
	if err != nil {
		return nil, err
	}
	tx, err := txbuilder.NewUnlockFCTx(accountKey, w.targetPartitionSystemID, fcb, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create unlockFC transaction: %w", err)
	}
	proof, err := sendTx(ctx, tx, accountKey.PubKey, w.targetPartitionClient, w.log)
	if err != nil {
		return nil, fmt.Errorf("failed to send unlockFC transaction: %w", err)
	}
	return proof, nil
}

// Close propagates call to all dependencies
func (w *FeeManager) Close() {
	// TODO close clients?
	_ = w.db.Close()
}

// addFees runs normal fee credit creation process for multiple bills
func (w *FeeManager) addFees(ctx context.Context, accountKey *account.AccountKey, cmd AddFeeCmd) (*AddFeeCmdResponse, error) {
	fcb, err := w.fetchTargetPartitionFCB(ctx, accountKey)
	if err != nil {
		return nil, err
	}
	// verify fee credit bill is not locked
	if fcb.IsLocked() {
		return nil, fmt.Errorf("fee credit bill is locked")
	}

	bills, err := w.fetchBills(ctx, accountKey)
	if err != nil {
		return nil, err
	}

	// verify at least one bill in wallet
	if len(bills) == 0 {
		return nil, errors.New("wallet does not contain any bills")
	}

	// filter locked bills
	bills, _ = filterSlice(bills, func(b *api.Bill) (bool, error) {
		return !b.IsLocked(), nil
	})

	// filter bills of too small value
	bills, _ = filterSlice(bills, func(b *api.Bill) (bool, error) {
		return b.Value() >= MinimumFeeAmount, nil
	})

	// sum bill values i.e. calculate effective balance
	balance := w.sumValues(bills)

	// verify enough balance for all transactions
	var targetAmount = cmd.Amount
	if balance < targetAmount {
		return nil, ErrInsufficientBalance
	}

	// send fee credit transactions
	res := &AddFeeCmdResponse{}
	var totalTransferredAmount uint64
	for _, targetBill := range bills {
		if totalTransferredAmount >= targetAmount {
			break
		}
		// send fee credit transactions
		amount := min(targetBill.Value(), targetAmount-totalTransferredAmount)
		totalTransferredAmount += amount

		feeCtx := &AddFeeCreditCtx{
			TargetPartitionID:  w.targetPartitionSystemID,
			TargetBillID:       targetBill.ID,
			TargetBillBacklink: targetBill.Backlink(),
			TargetAmount:       amount,
			LockingDisabled:    cmd.DisableLocking,
		}
		if err := w.db.SetAddFeeContext(accountKey.PubKey, feeCtx); err != nil {
			return nil, fmt.Errorf("failed to initialise fee context: %w", err)
		}
		proofs, err := w.addFeeCredit(ctx, accountKey, feeCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to add fee credit: %w", err)
		}
		res.Proofs = append(res.Proofs, proofs)
		if err := w.db.DeleteAddFeeContext(accountKey.PubKey); err != nil {
			return nil, fmt.Errorf("failed to delete add fee context: %w", err)
		}
	}
	return res, nil
}

// addFeeCredit runs the add fee credit process for single bill, stores the process status in WriteAheadLog which can be
// used to continue the process later, in case of any errors.
func (w *FeeManager) addFeeCredit(ctx context.Context, accountKey *account.AccountKey, feeCtx *AddFeeCreditCtx) (*AddFeeTxProofs, error) {
	if err := w.sendLockFCTx(ctx, accountKey, feeCtx); err != nil {
		return nil, fmt.Errorf("failed to lockFC: %w", err)
	}
	if err := w.sendTransferFCTx(ctx, accountKey, feeCtx); err != nil {
		return nil, fmt.Errorf("failed to transferFC: %w", err)
	}
	if err := w.sendAddFCTx(ctx, accountKey, feeCtx); err != nil {
		return nil, fmt.Errorf("failed to addFC: %w", err)
	}
	return &AddFeeTxProofs{
		LockFC:     feeCtx.LockFCProof,
		TransferFC: feeCtx.TransferFCProof,
		AddFC:      feeCtx.AddFCProof,
	}, nil
}

func (w *FeeManager) sendLockFCTx(ctx context.Context, accountKey *account.AccountKey, feeCtx *AddFeeCreditCtx) error {
	if feeCtx.LockingDisabled {
		return nil
	}
	// fee credit already locked
	if feeCtx.LockFCProof != nil {
		return nil
	}
	// if lockFC tx already exists wait for confirmation
	// if confirmed => store proof
	// if not confirmed => create new transaction
	if feeCtx.LockFCTx != nil {
		proof, err := api.WaitForConf(ctx, w.targetPartitionClient, feeCtx.LockFCTx)
		if err != nil {
			return fmt.Errorf("failed to wait for confirmation: %w", err)
		}
		if proof != nil {
			w.log.InfoContext(ctx, fmt.Sprintf("lockFC tx '%s' confirmed", feeCtx.LockFCTx.Hash(crypto.SHA256)))
			feeCtx.LockFCProof = proof
			if err := w.db.SetAddFeeContext(accountKey.PubKey, feeCtx); err != nil {
				return fmt.Errorf("failed to store lockFC proof: %w", err)
			}
			return nil
		}
	}
	fcb, err := w.fetchTargetPartitionFCB(ctx, accountKey)
	if err != nil {
		return fmt.Errorf("failed to fetch fee credit bill: %w", err)
	}
	// cannot lock fee credit bill if it does not exist
	if fcb == nil {
		return nil
	}
	// do not lock 0 value fee credit bill
	if fcb.Balance() == 0 {
		return nil
	}
	// verify fee credit bill is not locked
	if fcb.IsLocked() {
		return errors.New("fee credit bill is locked")
	}

	// fetch round number for timeout
	targetPartitionTimeout, err := w.getTargetPartitionTimeout(ctx)
	if err != nil {
		return err
	}

	// create lockFC
	w.log.InfoContext(ctx, "sending lock fee credit transaction")
	tx, err := txbuilder.NewLockFCTx(
		accountKey,
		w.targetPartitionSystemID,
		fcb,
		unitlock.LockReasonAddFees,
		targetPartitionTimeout,
	)
	if err != nil {
		return fmt.Errorf("failed to create lockFC transaction: %w", err)
	}

	// store lockFC write-ahead log
	feeCtx.LockFCTx = tx
	if err := w.db.SetAddFeeContext(accountKey.PubKey, feeCtx); err != nil {
		return fmt.Errorf("failed to store lockFC write-ahead log: %w", err)
	}

	// send lockFC transaction
	proof, err := sendTx(ctx, tx, accountKey.PubKey, w.targetPartitionClient, w.log)
	if err != nil {
		return fmt.Errorf("failed to send lockFC transaction: %w", err)
	}

	// store lockFC proof
	feeCtx.LockFCProof = proof
	if err = w.db.SetAddFeeContext(accountKey.PubKey, feeCtx); err != nil {
		return fmt.Errorf("failed to store lockFC proof: %w", err)
	}
	return nil
}

func (w *FeeManager) sendTransferFCTx(ctx context.Context, accountKey *account.AccountKey, feeCtx *AddFeeCreditCtx) error {
	// transferFC already sent
	if feeCtx.TransferFCProof != nil {
		return nil
	}
	// if transferFC tx already exists wait for confirmation =>
	//   if confirmed => store proof
	//   if not confirmed => verify target bill and create new transaction, or return error
	if feeCtx.TransferFCTx != nil {
		proof, err := api.WaitForConf(ctx, w.moneyClient, feeCtx.TransferFCTx)
		if err != nil {
			return fmt.Errorf("failed to wait for confirmation: %w", err)
		}
		if proof != nil {
			feeCtx.TransferFCProof = proof
			if err := w.db.SetAddFeeContext(accountKey.PubKey, feeCtx); err != nil {
				return fmt.Errorf("failed to store transferFC proof: %w", err)
			}
			return nil
		}

		// if transferFC failed then verify the source bill is still valid,
		// if not valid then return error to user and delete fee context and remote lock
		sourceBill, err := w.fetchBillByIdAndHash(ctx, accountKey, feeCtx.TargetBillID, feeCtx.TargetBillBacklink)
		if err != nil {
			return err
		}
		if sourceBill == nil {
			w.log.WarnContext(ctx, "transferFC target unit no longer usable, unlocking fee credit unit")
			// unlock remote locked fee credit record if exists
			if feeCtx.LockFCProof != nil {
				_, err := w.unlockFeeCreditRecord(ctx, accountKey)
				if err != nil {
					return fmt.Errorf("failed to unlock remote fee credit record: %w", err)
				}
			}
			// delete ctx
			if err := w.db.DeleteAddFeeContext(accountKey.PubKey); err != nil {
				return fmt.Errorf("failed to delete add fee context: %w", err)
			}
			// return error to user
			return fmt.Errorf("transferFC target unit is no longer valid")
		}
	}

	// fetch timeouts
	moneyTimeout, err := w.getMoneyPartitionTimeout(ctx)
	if err != nil {
		return err
	}
	roundNumber, err := w.moneyClient.GetRoundNumber(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch target partition round number: %w", err)
	}
	earliestAdditionTime := roundNumber
	latestAdditionTime := roundNumber + transferFCLatestAdditionTime

	// create transferFC transaction
	w.log.InfoContext(ctx, "sending transfer fee credit transaction")
	fcrID := w.targetPartitionFcrIDFn(nil, accountKey.PubKey)
	fcb, err := w.fetchTargetPartitionFCB(ctx, accountKey)
	if err != nil {
		return fmt.Errorf("faild to fetch fee credit bill: %w", err)
	}
	tx, err := txbuilder.NewTransferFCTx(
		feeCtx.TargetAmount,
		fcrID,
		fcb.Backlink(),
		accountKey,
		w.moneySystemID,
		w.targetPartitionSystemID,
		feeCtx.TargetBillID,
		feeCtx.TargetBillBacklink,
		moneyTimeout,
		earliestAdditionTime,
		latestAdditionTime,
	)
	if err != nil {
		return fmt.Errorf("failed to create transferFC transaction: %w", err)
	}

	// store transferFC transaction write-ahead log
	feeCtx.TransferFCTx = tx
	if err := w.db.SetAddFeeContext(accountKey.PubKey, feeCtx); err != nil {
		return fmt.Errorf("failed to store transferFC write-ahead log: %w", err)
	}

	// send transferFC transaction to money partition
	proof, err := sendTx(ctx, tx, accountKey.PubKey, w.moneyClient, w.log)
	if err != nil {
		return fmt.Errorf("failed to send transferFC transaction: %w", err)
	}

	// store transferFC transaction proof
	feeCtx.TransferFCProof = proof
	if err := w.db.SetAddFeeContext(accountKey.PubKey, feeCtx); err != nil {
		return fmt.Errorf("failed to store transferFC proof: %w", err)
	}
	return nil
}

func (w *FeeManager) sendAddFCTx(ctx context.Context, accountKey *account.AccountKey, feeCtx *AddFeeCreditCtx) error {
	// check if addFC already sent
	if feeCtx.AddFCProof != nil {
		return nil
	}
	// if addFC tx already exists wait for confirmation =>
	// if confirmed => store proof
	// if not confirmed =>
	//   check if transferFC proof is still usable =>
	//     if yes => create new addFC with existing transferFC proof
	//     if not => unlock remote fee credit record and delete fee context
	if feeCtx.AddFCTx != nil {
		proof, err := api.WaitForConf(ctx, w.targetPartitionClient, feeCtx.AddFCTx)
		if err != nil {
			return fmt.Errorf("failed to wait for confirmation: %w", err)
		}
		if proof != nil {
			feeCtx.AddFCProof = proof
			if err := w.db.SetAddFeeContext(accountKey.PubKey, feeCtx); err != nil {
				return fmt.Errorf("failed to store addFC proof: %w", err)
			}
			return nil
		}
		transferFCAttr := &transactions.TransferFeeCreditAttributes{}
		if err := feeCtx.TransferFCProof.TxRecord.TransactionOrder.UnmarshalAttributes(transferFCAttr); err != nil {
			return fmt.Errorf("failed to unmarshal transferFC attributes: %w", err)
		}
		roundNumber, err := w.targetPartitionClient.GetRoundNumber(ctx)
		if err != nil {
			return fmt.Errorf("failed to fetch target partition round number: %w", err)
		}
		if roundNumber >= transferFCAttr.LatestAdditionTime {
			_, err := w.unlockFeeCreditRecord(ctx, accountKey)
			if err != nil {
				return fmt.Errorf("failed to unlock remote fee credit record: %w", err)
			}
			if err := w.db.DeleteAddFeeContext(accountKey.PubKey); err != nil {
				return fmt.Errorf("failed to delete add fee context: %w", err)
			}
			return errors.New("addFC timed out and transferFC latestAdditionTime exceeded, the target bill is no longer usable")
		}
		w.log.InfoContext(ctx, "addFC timed out, but transferFC still usable")
	}

	// fetch round number for timeout
	timeout, err := w.getTargetPartitionTimeout(ctx)
	if err != nil {
		return err
	}

	// create addFC transaction
	fcrID := w.targetPartitionFcrIDFn(nil, accountKey.PubKey)
	addFCTx, err := txbuilder.NewAddFCTx(fcrID, feeCtx.TransferFCProof, accountKey, w.targetPartitionSystemID, timeout)
	if err != nil {
		return fmt.Errorf("failed to create addFC transaction: %w", err)
	}

	// store addFC write-ahead log
	feeCtx.AddFCTx = addFCTx
	err = w.db.SetAddFeeContext(accountKey.PubKey, feeCtx)
	if err != nil {
		return fmt.Errorf("failed to store addFC write-ahead log: %w", err)
	}

	// send addFC transaction
	w.log.InfoContext(ctx, "sending add fee credit transaction")
	proof, err := sendTx(ctx, addFCTx, accountKey.PubKey, w.targetPartitionClient, w.log)
	if err != nil {
		return fmt.Errorf("failed to send addFC transaction: %w", err)
	}

	// store addFC proof
	feeCtx.AddFCProof = proof
	if err := w.db.SetAddFeeContext(accountKey.PubKey, feeCtx); err != nil {
		return fmt.Errorf("failed to store addFC proof: %w", err)
	}
	return nil
}

// reclaimFees closes and reclaims entire fee credit bill balance back to the main balance, largest bill is used as the
// target bill, stores status in WriteAheadLog which can be used to continue the process later, in case of any errors.
func (w *FeeManager) reclaimFees(ctx context.Context, accountKey *account.AccountKey, cmd ReclaimFeeCmd) (*ReclaimFeeCmdResponse, error) {
	// fetch fee credit bill
	fcb, err := w.fetchTargetPartitionFCB(ctx, accountKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch fee credit bill: %w", err)
	}
	if fcb.IsLocked() {
		return nil, errors.New("fee credit bill is locked")
	}
	if fcb.Balance() < MinimumFeeAmount {
		return nil, ErrMinimumFeeAmount
	}

	// select largest bill as the target
	bills, err := w.fetchBills(ctx, accountKey)
	if err != nil {
		return nil, err
	}
	bills, _ = filterSlice(bills, func(b *api.Bill) (bool, error) {
		return !b.IsLocked(), nil
	})
	if len(bills) == 0 {
		return nil, errors.New("wallet must have a source bill to which to add reclaimed fee credits")
	}
	targetBill := bills[0]

	// create fee ctx to track reclaim process
	feeCtx := &ReclaimFeeCreditCtx{
		TargetPartitionID:  w.targetPartitionSystemID,
		TargetBillID:       targetBill.ID,
		TargetBillBacklink: targetBill.Backlink(),
		LockingDisabled:    cmd.DisableLocking,
	}
	if err := w.db.SetReclaimFeeContext(accountKey.PubKey, feeCtx); err != nil {
		return nil, fmt.Errorf("failed to store reclaim fee context: %w", err)
	}
	feeTxProofs, err := w.reclaimFeeCredit(ctx, accountKey, feeCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to reclaim fee credit: %w", err)
	}
	if err := w.db.DeleteReclaimFeeContext(accountKey.PubKey); err != nil {
		return nil, fmt.Errorf("failed to delete reclaim fee context: %w", err)
	}
	return &ReclaimFeeCmdResponse{Proofs: feeTxProofs}, nil
}

// reclaimFeeCredit runs the reclaim fee credit process for single bill, stores the process status in WriteAheadLog
// which can be used to continue the process later, in case of any errors.
func (w *FeeManager) reclaimFeeCredit(ctx context.Context, accountKey *account.AccountKey, feeCtx *ReclaimFeeCreditCtx) (*ReclaimFeeTxProofs, error) {
	if err := w.sendLockTx(ctx, accountKey, feeCtx); err != nil {
		return nil, fmt.Errorf("failed to lock: %w", err)
	}
	if err := w.sendCloseFCTx(ctx, accountKey, feeCtx); err != nil {
		return nil, fmt.Errorf("failed to closeFC: %w", err)
	}
	if err := w.sendReclaimFCTx(ctx, accountKey, feeCtx); err != nil {
		return nil, fmt.Errorf("failed to reclaimFC: %w", err)
	}
	return &ReclaimFeeTxProofs{
		Lock:      feeCtx.LockTxProof,
		CloseFC:   feeCtx.CloseFCProof,
		ReclaimFC: feeCtx.ReclaimFCProof,
	}, nil
}

func (w *FeeManager) sendLockTx(ctx context.Context, accountKey *account.AccountKey, feeCtx *ReclaimFeeCreditCtx) error {
	if feeCtx.LockingDisabled {
		return nil
	}
	// target bill already locked
	if feeCtx.LockTxProof != nil {
		return nil
	}
	// if lock tx already exists then wait for confirmation => if confirmed store proof else create new transaction
	if feeCtx.LockTx != nil {
		proof, err := api.WaitForConf(ctx, w.moneyClient, feeCtx.LockTx)
		if err != nil {
			return fmt.Errorf("failed to wait for confirmation: %w", err)
		}
		if proof != nil {
			w.log.InfoContext(ctx, fmt.Sprintf("lock tx '%s' confirmed", feeCtx.LockTx.Hash(crypto.SHA256)))
			feeCtx.LockTxProof = proof
			feeCtx.TargetBillBacklink = proof.TxRecord.TransactionOrder.Hash(crypto.SHA256)
			if err := w.db.SetReclaimFeeContext(accountKey.PubKey, feeCtx); err != nil {
				return fmt.Errorf("failed to store lock tx proof: %w", err)
			}
			return nil
		}
	}

	moneyFCB, err := w.fetchMoneyPartitionFCB(ctx, accountKey)
	if err != nil {
		return fmt.Errorf("failed to fetch money fee credit bill: %w", err)
	}
	// do not lock target bill if there's not enough fee credit on money partition
	if moneyFCB.Balance() == 0 {
		return nil
	}

	// create lock tx
	timeout, err := w.getMoneyPartitionTimeout(ctx)
	if err != nil {
		return err
	}
	tx, err := txbuilder.NewLockTx(
		accountKey,
		w.targetPartitionSystemID,
		feeCtx.TargetBillID,
		feeCtx.TargetBillBacklink,
		unitlock.LockReasonReclaimFees,
		timeout,
	)
	if err != nil {
		return fmt.Errorf("failed to create lock transaction: %w", err)
	}

	// store lock transaction write-ahead log
	feeCtx.LockTx = tx
	if err := w.db.SetReclaimFeeContext(accountKey.PubKey, feeCtx); err != nil {
		return fmt.Errorf("failed to store lock tx write-ahead log: %w", err)
	}

	// send lock transaction
	w.log.InfoContext(ctx, "sending lock transaction")
	proof, err := sendTx(ctx, tx, accountKey.PubKey, w.moneyClient, w.log)
	if err != nil {
		return fmt.Errorf("failed to send lock transaction: %w", err)
	}

	// store lock transaction proof in fee context
	feeCtx.LockTxProof = proof
	feeCtx.TargetBillBacklink = proof.TxRecord.TransactionOrder.Hash(crypto.SHA256)
	if err = w.db.SetReclaimFeeContext(accountKey.PubKey, feeCtx); err != nil {
		return fmt.Errorf("failed to store lock transaction fee context: %w", err)
	}
	return nil
}

func (w *FeeManager) sendCloseFCTx(ctx context.Context, accountKey *account.AccountKey, feeCtx *ReclaimFeeCreditCtx) error {
	// check if closeFC already sent
	if feeCtx.CloseFCProof != nil {
		return nil
	}
	// if closeFC tx already exists wait for confirmation =>
	// if confirmed => store proof
	// if not confirmed => create new transaction
	if feeCtx.CloseFCTx != nil {
		proof, err := api.WaitForConf(ctx, w.targetPartitionClient, feeCtx.CloseFCTx)
		if err != nil {
			return fmt.Errorf("failed to wait for confirmation: %w", err)
		}
		if proof != nil {
			feeCtx.CloseFCProof = proof
			if err := w.db.SetReclaimFeeContext(accountKey.PubKey, feeCtx); err != nil {
				return fmt.Errorf("failed to store closeFC proof: %w", err)
			}
			return nil
		}
	}

	// fetch fee credit bill
	fcb, err := w.fetchTargetPartitionFCB(ctx, accountKey)
	if err != nil {
		return fmt.Errorf("failed to fetch fee credit bill: %w", err)
	}

	// fetch target partition timeout
	targetPartitionTimeout, err := w.getTargetPartitionTimeout(ctx)
	if err != nil {
		return err
	}

	// create closeFC transaction
	tx, err := txbuilder.NewCloseFCTx(
		w.targetPartitionSystemID, fcb.ID, targetPartitionTimeout, fcb.Balance(),
		feeCtx.TargetBillID, feeCtx.TargetBillBacklink, accountKey)
	if err != nil {
		return fmt.Errorf("failed to create closeFC transaction: %w", err)
	}

	// store closeFC write-ahead log
	feeCtx.CloseFCTx = tx
	if err := w.db.SetReclaimFeeContext(accountKey.PubKey, feeCtx); err != nil {
		return fmt.Errorf("failed to store closeFC write-ahead log: %w", err)
	}

	// send closeFC transaction to target partition
	w.log.InfoContext(ctx, "sending close fee credit transaction")
	proof, err := sendTx(ctx, tx, accountKey.PubKey, w.targetPartitionClient, w.log)
	if err != nil {
		return fmt.Errorf("failed to send closeFC transaction: %w", err)
	}

	// store closeFC transaction proof
	feeCtx.CloseFCProof = proof
	if err := w.db.SetReclaimFeeContext(accountKey.PubKey, feeCtx); err != nil {
		return fmt.Errorf("failed to store closeFC proof: %w", err)
	}
	return nil
}

func (w *FeeManager) sendReclaimFCTx(ctx context.Context, accountKey *account.AccountKey, feeCtx *ReclaimFeeCreditCtx) error {
	// check if reclaimFC already sent
	if feeCtx.ReclaimFCProof != nil {
		return nil
	}
	// if reclaimFC tx already exists wait for confirmation =>
	// if confirmed => store proof
	// if not confirmed =>
	//   check if closeFC proof is still usable =>
	//     if yes => create new reclaimFC with existing closeFC proof
	//     if not => unlock target bill and delete fee context
	if feeCtx.ReclaimFCTx != nil {
		proof, err := api.WaitForConf(ctx, w.moneyClient, feeCtx.ReclaimFCTx)
		if err != nil {
			return fmt.Errorf("failed to wait for confirmation: %w", err)
		}
		if proof != nil {
			feeCtx.ReclaimFCProof = proof
			if err := w.db.SetReclaimFeeContext(accountKey.PubKey, feeCtx); err != nil {
				return fmt.Errorf("failed to store reclaimFC proof: %w", err)
			}
			return nil
		}
		actualTargetBill, err := w.fetchBillByIdAndHash(ctx, accountKey, feeCtx.TargetBillID, feeCtx.TargetBillBacklink)
		if err != nil {
			return err
		}
		if actualTargetBill == nil {
			_, err := w.unlockBill(ctx, accountKey, feeCtx.TargetBillID)
			if err != nil {
				return fmt.Errorf("failed to unlock target bill: %w", err)
			}
			if err := w.db.DeleteReclaimFeeContext(accountKey.PubKey); err != nil {
				return fmt.Errorf("failed to delete reclaim fee context: %w", err)
			}
			return errors.New("reclaimFC target bill is no longer usable")
		}
		w.log.InfoContext(ctx, "reclaimFC timed out, but closeFC is still valid, sending new reclaimFC transaction")
	}

	moneyTimeout, err := w.getMoneyPartitionTimeout(ctx)
	if err != nil {
		return err
	}

	reclaimFC, err := txbuilder.NewReclaimFCTx(w.moneySystemID, feeCtx.TargetBillID, moneyTimeout, feeCtx.CloseFCProof, feeCtx.TargetBillBacklink, accountKey)
	if err != nil {
		return fmt.Errorf("failed to create reclaimFC transaction: %w", err)
	}

	// store reclaimFC write-ahead log
	feeCtx.ReclaimFCTx = reclaimFC
	if err := w.db.SetReclaimFeeContext(accountKey.PubKey, feeCtx); err != nil {
		return fmt.Errorf("failed to store reclaimFC write-ahead log: %w", err)
	}

	// send reclaimFC transaction
	w.log.InfoContext(ctx, "sending reclaim fee credit transaction")
	proof, err := sendTx(ctx, reclaimFC, accountKey.PubKey, w.moneyClient, w.log)
	if err != nil {
		return fmt.Errorf("failed to send reclaimFC transaction: %w", err)
	}

	// store reclaimFC proof
	feeCtx.ReclaimFCProof = proof
	if err := w.db.SetReclaimFeeContext(accountKey.PubKey, feeCtx); err != nil {
		return fmt.Errorf("failed to store reclaimFC proof: %w", err)
	}
	return nil
}

func (w *FeeManager) getMoneyPartitionTimeout(ctx context.Context) (uint64, error) {
	roundNumber, err := w.moneyClient.GetRoundNumber(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch money partition round number: %w", err)
	}
	return roundNumber + txTimeoutBlockCount, nil
}

func (w *FeeManager) getTargetPartitionTimeout(ctx context.Context) (uint64, error) {
	roundNumber, err := w.targetPartitionClient.GetRoundNumber(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch target partition round number: %w", err)
	}
	return roundNumber + txTimeoutBlockCount, nil
}

// fetchBills fetches bills from backend and sorts them by value (descending, largest first)
func (w *FeeManager) fetchBills(ctx context.Context, k *account.AccountKey) ([]*api.Bill, error) {
	bills, err := api.FetchBills(ctx, w.moneyClient, k.PubKeyHash.Sha256)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bills: %w", err)
	}
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Value() > bills[j].Value()
	})
	return bills, nil
}

func (w *FeeManager) fetchBillByIdAndHash(ctx context.Context, accountKey *account.AccountKey, unitID []byte, txHash []byte) (*api.Bill, error) {
	bills, err := api.FetchBills(ctx, w.moneyClient, accountKey.PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bills: %w", err)
	}
	for _, b := range bills {
		if bytes.Equal(b.ID, unitID) && bytes.Equal(b.Backlink(), txHash) {
			return b, nil
		}
	}
	return nil, nil
}

func (w *FeeManager) sumValues(bills []*api.Bill) uint64 {
	var sum uint64
	for _, b := range bills {
		sum += b.Value()
	}
	return sum
}

func (w *FeeManager) fetchTargetPartitionFCB(ctx context.Context, accountKey *account.AccountKey) (*api.FeeCreditBill, error) {
	fcrID := w.targetPartitionFcrIDFn(nil, accountKey.PubKey)
	return api.FetchFeeCreditBill(ctx, w.targetPartitionClient, fcrID)
}

func (w *FeeManager) fetchMoneyPartitionFCB(ctx context.Context, accountKey *account.AccountKey) (*api.FeeCreditBill, error) {
	fcrID := w.moneyPartitionFcrIDFn(nil, accountKey.PubKey)
	return api.FetchFeeCreditBill(ctx, w.moneyClient, fcrID)
}

func (w *FeeManager) unlockFeeCreditRecord(ctx context.Context, accountKey *account.AccountKey) (*wallet.Proof, error) {
	fcb, err := w.fetchTargetPartitionFCB(ctx, accountKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch fee credit record: %w", err)
	}
	if !fcb.IsLocked() {
		return nil, nil
	}
	timeout, err := w.getTargetPartitionTimeout(ctx)
	if err != nil {
		return nil, err
	}
	tx, err := txbuilder.NewUnlockFCTx(accountKey, w.targetPartitionSystemID, fcb, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create unlockFC transaction: %w", err)
	}
	proof, err := sendTx(ctx, tx, accountKey.PubKey, w.targetPartitionClient, w.log)
	if err != nil {
		return nil, fmt.Errorf("failed to send unlockFC tx: %w", err)
	}
	return proof, nil
}

func (w *FeeManager) unlockBill(ctx context.Context, accountKey *account.AccountKey, unitID types.UnitID) (*wallet.Proof, error) {
	bills, err := w.fetchBills(ctx, accountKey)
	if err != nil {
		return nil, err
	}
	for _, b := range bills {
		if bytes.Equal(b.ID, unitID) {
			if b.IsLocked() {
				timeout, err := w.getMoneyPartitionTimeout(ctx)
				if err != nil {
					return nil, err
				}
				unlockTx, err := txbuilder.NewUnlockTx(accountKey, w.moneySystemID, b, timeout)
				if err != nil {
					return nil, fmt.Errorf("failed to create unlock tx: %w", err)
				}
				proof, err := sendTx(ctx, unlockTx, accountKey.PubKey, w.moneyClient, w.log)
				if err != nil {
					return nil, fmt.Errorf("failed to send unlock tx: %w", err)
				}
				return proof, nil
			}
			return nil, nil
		}
	}
	return nil, nil
}

func (c AddFeeCmd) isValid() error {
	if c.Amount < MinimumFeeAmount {
		return ErrMinimumFeeAmount
	}
	return nil
}

// filterSlice generic function for filtering a slice
func filterSlice[T any](src []*T, filterFn func(*T) (bool, error)) ([]*T, error) {
	var res []*T
	for _, b := range src {
		ok, err := filterFn(b)
		if err != nil {
			return nil, err
		}
		if ok {
			res = append(res, b)
		}
	}
	return res, nil
}

func (p *AddFeeTxProofs) GetFees() uint64 {
	if p == nil {
		return 0
	}
	return p.LockFC.GetActualFee() + p.TransferFC.GetActualFee() + p.AddFC.GetActualFee()
}

func (p *ReclaimFeeTxProofs) GetFees() uint64 {
	if p == nil {
		return 0
	}
	return p.Lock.GetActualFee() + p.CloseFC.GetActualFee() + p.ReclaimFC.GetActualFee()
}

func sendTx(ctx context.Context, tx *types.TransactionOrder, senderPubKey wallet.PubKey, c RpcClient, log *slog.Logger) (*wallet.Proof, error) {
	batch := txsubmitter.NewBatch(senderPubKey, c, log)
	txSubmission := &txsubmitter.TxSubmission{
		UnitID:      tx.UnitID(),
		TxHash:      tx.Hash(crypto.SHA256),
		Transaction: tx,
	}
	batch.Add(txSubmission)
	if err := batch.SendTx(ctx, true); err != nil {
		return nil, err
	}
	return txSubmission.Proof, nil
}
