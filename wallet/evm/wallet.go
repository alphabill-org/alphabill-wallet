package evm

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/txsystem/evm"
	"github.com/alphabill-org/alphabill-go-base/types"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	evmclient "github.com/alphabill-org/alphabill-wallet/wallet/evm/client"
)

const txTimeoutBlockCount = 10

type (
	evmClient interface {
		Client
		Call(ctx context.Context, callAttr *evm.CallEVMRequest) (*evm.ProcessingDetails, error)
		GetTransactionCount(ctx context.Context, ethAddr []byte) (uint64, error)
		GetBalance(ctx context.Context, ethAddr []byte) (string, uint64, error)
		GetFeeCreditBill(ctx context.Context, unitID types.UnitID) (*evmclient.Bill, error)
		GetGasPrice(ctx context.Context) (string, error)
	}

	Wallet struct {
		networkID types.NetworkID
		systemID  types.SystemID
		am        account.Manager
		restCli   evmClient
	}
)

func ConvertBalanceToAlpha(eth *big.Int) uint64 {
	return evmclient.WeiToAlpha(eth)
}

func New(systemID types.SystemID, restUrl string, am account.Manager) (*Wallet, error) {
	if systemID == 0 {
		return nil, fmt.Errorf("system id is unassigned")
	}
	if len(restUrl) == 0 {
		return nil, fmt.Errorf("rest url is empty")
	}
	if am == nil {
		return nil, fmt.Errorf("account manager is nil")
	}
	if !strings.HasPrefix(restUrl, "http://") && !strings.HasPrefix(restUrl, "https://") {
		restUrl = "http://" + restUrl
	}
	addr, err := url.Parse(restUrl)
	if err != nil {
		return nil, err
	}
	return &Wallet{
		systemID: systemID,
		am:       am,
		restCli:  evmclient.New(*addr),
	}, nil
}

func (w *Wallet) Shutdown() {
	w.am.Close()
}

func (w *Wallet) SendEvmTx(ctx context.Context, accountNumber uint64, attrs *evm.TxAttributes) (*evmclient.Result, error) {
	if accountNumber < 1 {
		return nil, fmt.Errorf("invalid account number: %d", accountNumber)
	}
	acc, err := w.am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return nil, fmt.Errorf("account key read failed: %w", err)
	}
	from, err := generateAddress(acc.PubKey)
	if err != nil {
		return nil, fmt.Errorf("from address generation failed: %w", err)
	}
	rnr, err := w.restCli.GetRoundNumber(ctx)
	if err != nil {
		return nil, fmt.Errorf("evm current round number read failed: %w", err)
	}
	if err := w.verifyFeeCreditBalance(ctx, acc, attrs.Gas); err != nil {
		return nil, err
	}
	// verify account exists and get transaction count
	nonce, err := w.restCli.GetTransactionCount(ctx, from.Bytes())
	if err != nil {
		return nil, fmt.Errorf("account %x transaction count read failed: %w", from.Bytes(), err)
	}
	attrs.From = from.Bytes()
	attrs.Nonce = nonce
	if attrs.Value == nil {
		attrs.Value = big.NewInt(0)
	}
	txo, err := sdktypes.NewTransactionOrder(w.networkID, w.systemID, from.Bytes(), evm.TransactionTypeEVMCall, attrs, sdktypes.WithTimeout(rnr.RoundNumber+txTimeoutBlockCount))
	if err != nil {
		return nil, fmt.Errorf("failed to create evm transaction order: %w", err)
	}
	if err = signTx(txo, acc); err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}
	// send transaction and wait for response or timeout
	txPub := NewTxPublisher(w.restCli)
	proof, err := txPub.SendTx(ctx, txo, nil)
	if err != nil {
		return nil, fmt.Errorf("evm transaction failed or account does not have enough fee credit: %w", err)
	}
	if proof == nil || proof.TxRecord == nil {
		return nil, fmt.Errorf("unexpected result")
	}
	var details evm.ProcessingDetails
	if err = proof.TxRecord.UnmarshalProcessingDetails(&details); err != nil {
		return nil, fmt.Errorf("failed to de-serialize evm execution result: %w", err)
	}
	return &evmclient.Result{
		Success:   proof.TxRecord.ServerMetadata.SuccessIndicator == types.TxStatusSuccessful,
		ActualFee: proof.TxRecord.ServerMetadata.GetActualFee(),
		Details:   &details,
	}, nil
}

func (w *Wallet) EvmCall(ctx context.Context, accountNumber uint64, attrs *evm.CallEVMRequest) (*evmclient.Result, error) {
	if accountNumber < 1 {
		return nil, fmt.Errorf("invalid account number: %d", accountNumber)
	}
	acc, err := w.am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return nil, fmt.Errorf("account key read failed: %w", err)
	}
	from, err := generateAddress(acc.PubKey)
	if err != nil {
		return nil, fmt.Errorf("generating address: %w", err)
	}
	attrs.From = from.Bytes()
	details, err := w.restCli.Call(ctx, attrs)
	if err != nil {
		return nil, err
	}
	return &evmclient.Result{
		Success:   len(details.ErrorDetails) == 0,
		ActualFee: 0,
		Details:   details,
	}, nil
}

func (w *Wallet) GetBalance(ctx context.Context, accountNumber uint64) (*big.Int, error) {
	if accountNumber < 1 {
		return nil, fmt.Errorf("invalid account number: %d", accountNumber)
	}
	acc, err := w.am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return nil, fmt.Errorf("account key read failed: %w", err)
	}
	from, err := generateAddress(acc.PubKey)
	if err != nil {
		return nil, fmt.Errorf("generating address: %w", err)
	}
	balanceStr, _, err := w.restCli.GetBalance(ctx, from.Bytes())
	balance, ok := new(big.Int).SetString(balanceStr, 10)
	if !ok {
		return nil, fmt.Errorf("balance string %s to base 10 conversion failed: %w", balanceStr, err)
	}
	return balance, nil
}

// make sure wallet has enough fee credit to perform transaction
func (w *Wallet) verifyFeeCreditBalance(ctx context.Context, acc *account.AccountKey, maxGas uint64) error {
	from, err := generateAddress(acc.PubKey)
	if err != nil {
		return fmt.Errorf("generating address: %w", err)
	}
	balanceStr, _, err := w.restCli.GetBalance(ctx, from.Bytes())
	if err != nil {
		if errors.Is(err, evmclient.ErrNotFound) {
			return fmt.Errorf("no fee credit in evm wallet")
		}
		return err
	}
	balance, ok := new(big.Int).SetString(balanceStr, 10)
	if !ok {
		return fmt.Errorf("balance %s to base 10 conversion failed: %w", balanceStr, err)
	}
	gasPriceStr, err := w.restCli.GetGasPrice(ctx)
	if err != nil {
		return err
	}
	gasPrice, ok := new(big.Int).SetString(gasPriceStr, 10)
	if !ok {
		return fmt.Errorf("gas price string %s to base 10 conversion failed: %w", gasPriceStr, err)
	}
	if balance.Cmp(new(big.Int).Mul(gasPrice, new(big.Int).SetUint64(maxGas))) == -1 {
		return fmt.Errorf("insufficient fee credit balance for transaction")
	}
	return nil
}

func signTx(tx *types.TransactionOrder, ac *account.AccountKey) error {
	signer, err := crypto.NewInMemorySecp256K1SignerFromKey(ac.PrivKey)
	if err != nil {
		return err
	}
	ownerProof, err := sdktypes.NewP2pkhAuthProofSignature(tx, signer)
	if err != nil {
		return err
	}
	if err = tx.SetAuthProof(evm.TxAuthProof{OwnerProof: ownerProof}); err != nil {
		return fmt.Errorf("failed to set auth proof: %w", err)
	}
	return nil
}
