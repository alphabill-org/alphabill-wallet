package main

import (
	"context"
	"crypto"
	"flag"
	"fmt"
	"log"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/args"
	"github.com/alphabill-org/alphabill/hash"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"

	"github.com/alphabill-org/alphabill-wallet/client/rpc"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/api"
)

/*
Example usage
go run scripts/money/spend_initial_bill.go --pubkey 0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3 --rpc-server-address localhost:26866 --bill-id 1 --bill-value 1000000000000000000 --timeout 10
*/
func main() {
	// parse command line parameters
	pubKeyHex := flag.String("pubkey", "", "public key of the new bill owner")
	billIdUint := flag.Uint64("bill-id", 0, "bill id of the spendable bill")
	billValue := flag.Uint64("bill-value", 0, "bill value of the spendable bill")
	timeout := flag.Uint64("timeout", 0, "transaction timeout (block number)")
	rpcServerAddr := flag.String("rpc-server-address", "", "money rpc node url")
	flag.Parse()

	// verify command line parameters
	if *pubKeyHex == "" {
		log.Fatal("pubkey is required")
	}
	if *billIdUint == 0 {
		log.Fatal("bill-id is required")
	}
	if *billValue == 0 {
		log.Fatal("bill-value is required")
	}
	if *timeout == 0 {
		log.Fatal("timeout is required")
	}
	if *rpcServerAddr == "" {
		log.Fatal("rpc-server-address is required")
	}

	// process command line parameters
	pubKey, err := hexutil.Decode(*pubKeyHex)
	if err != nil {
		log.Fatal(err)
	}

	billID := money.NewBillID(nil, util.Uint64ToBytes(*billIdUint))

	// create rpc client
	ctx := context.Background()
	rpcClient, err := rpc.DialContext(ctx, args.BuildRpcUrl(*rpcServerAddr))
	if err != nil {
		log.Fatal(err)
	}
	defer rpcClient.Close()

	err = execInitialBill(ctx, rpcClient, *timeout, billID, *billValue, pubKey)
	if err != nil {
		log.Fatal(err)
	}
}

func execInitialBill(ctx context.Context, rpcClient api.RpcClient, timeout uint64, billID types.UnitID, billValue uint64, pubKey []byte) error {
	roundNumber, err := rpcClient.GetRoundNumber(ctx)
	if err != nil {
		return fmt.Errorf("error getting round number: %w", err)
	}
	absoluteTimeout := roundNumber + timeout

	txFee := uint64(1)
	feeAmount := uint64(2)
	// Make the initial fcrID different from the default
	// sha256(pubKey), so that wallet can later create its own
	// fcrID for the same account with a different owner condition
	fcrID := money.NewFeeCreditRecordID(billID, hash.Sum256(hash.Sum256(pubKey)))

	// create transferFC
	transferFC, err := createTransferFC(feeAmount+txFee, billID, fcrID, roundNumber, absoluteTimeout)
	if err != nil {
		return fmt.Errorf("creating transfer FC transaction: %w", err)
	}
	// send transferFC
	log.Println("sending transferFC transaction")
	_, err = rpcClient.SendTransaction(ctx, transferFC)
	if err != nil {
		return fmt.Errorf("processing transfer FC transaction: %w", err)
	}
	// wait for transferFC proof
	transferFCProof, err := api.WaitForConf(ctx, rpcClient, transferFC)
	if err != nil {
		return fmt.Errorf("failed to confirm transferFC transaction %v", err)
	} else {
		log.Println("confirmed transferFC transaction")
	}

	// create addFC
	addFC, err := createAddFC(fcrID, templates.AlwaysTrueBytes(), transferFCProof.TxRecord, transferFCProof.TxProof, absoluteTimeout, feeAmount)
	if err != nil {
		return fmt.Errorf("creating add FC transaction: %w", err)
	}
	// send addFC
	log.Println("sending addFC transaction")
	_, err = rpcClient.SendTransaction(ctx, addFC)
	if err != nil {
		return fmt.Errorf("processing add FC transaction: %w", err)
	}
	// wait for addFC confirmation
	_, err = api.WaitForConf(ctx, rpcClient, addFC)
	if err != nil {
		return fmt.Errorf("failed to confirm addFC transaction %v", err)
	} else {
		log.Println("confirmed addFC transaction")
	}

	// create transfer tx
	transferTx, err := createTransferTx(pubKey, billID, billValue-feeAmount-txFee, fcrID, absoluteTimeout, transferFC.Hash(crypto.SHA256))
	if err != nil {
		return fmt.Errorf("creating transfer transaction: %w", err)
	}
	// send transfer tx
	log.Println("sending initial bill transfer transaction")
	_, err = rpcClient.SendTransaction(ctx, transferTx)
	if err != nil {
		return fmt.Errorf("processing transfer transaction: %w", err)
	}
	// wait for transfer tx confirmation
	_, err = api.WaitForConf(ctx, rpcClient, transferTx)
	if err != nil {
		return fmt.Errorf("failed to confirm transfer transaction %v", err)
	} else {
		log.Println("successfully confirmed initial bill transfer transaction")
	}
	return nil
}

func createTransferFC(feeAmount uint64, unitID []byte, targetUnitID []byte, t1, t2 uint64) (*types.TransactionOrder, error) {
	attr, err := cbor.Marshal(
		&transactions.TransferFeeCreditAttributes{
			Amount:                 feeAmount,
			TargetSystemIdentifier: 1,
			TargetRecordID:         targetUnitID,
			EarliestAdditionTime:   t1,
			LatestAdditionTime:     t2,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transferFC attributes: %w", err)
	}
	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:       1,
			Type:           transactions.PayloadTypeTransferFeeCredit,
			UnitID:         unitID,
			Attributes:     attr,
			ClientMetadata: &types.ClientMetadata{Timeout: t2, MaxTransactionFee: 1},
		},
		OwnerProof: nil,
	}
	return tx, nil
}

func createAddFC(unitID []byte, ownerCondition []byte, transferFC *types.TransactionRecord, transferFCProof *types.TxProof, timeout uint64, maxFee uint64) (*types.TransactionOrder, error) {
	attr, err := cbor.Marshal(
		&transactions.AddFeeCreditAttributes{
			FeeCreditTransfer:       transferFC,
			FeeCreditTransferProof:  transferFCProof,
			FeeCreditOwnerCondition: ownerCondition,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transferFC attributes: %w", err)
	}
	return &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:       1,
			Type:           transactions.PayloadTypeAddFeeCredit,
			UnitID:         unitID,
			Attributes:     attr,
			ClientMetadata: &types.ClientMetadata{Timeout: timeout, MaxTransactionFee: maxFee},
		},
		OwnerProof: nil,
	}, nil
}

func createTransferTx(pubKey []byte, unitID []byte, billValue uint64, fcrID []byte, timeout uint64, backlink []byte) (*types.TransactionOrder, error) {
	attr, err := cbor.Marshal(
		&money.TransferAttributes{
			NewBearer:   templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(pubKey)),
			TargetValue: billValue,
			Backlink:    backlink,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transferFC attributes: %w", err)
	}
	return &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:   1,
			Type:       money.PayloadTypeTransfer,
			UnitID:     unitID,
			Attributes: attr,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           timeout,
				MaxTransactionFee: 1,
				FeeCreditRecordID: fcrID,
			},
		},
		OwnerProof: nil,
	}, nil
}
