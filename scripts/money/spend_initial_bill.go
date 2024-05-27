package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/args"
	"github.com/alphabill-org/alphabill-wallet/client/rpc"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/api"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
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
	counter := flag.Uint64("counter", 0, "bill counter")
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

	// calculate fee credit record id
	roundNumber, err := rpcClient.GetRoundNumber(ctx)
	if err != nil {
		log.Fatal(err)
	}
	latestAdditionTime := roundNumber + *timeout
	fcrID := money.NewFeeCreditRecordIDFromOwnerPredicate(nil, templates.AlwaysTrueBytes(), latestAdditionTime)

	if err = execInitialBill(ctx, rpcClient, billID, fcrID, *billValue, latestAdditionTime, pubKey, *counter); err != nil {
		log.Fatal(err)
	}
}

func execInitialBill(ctx context.Context, rpcClient api.RpcClient, billID, fcrID types.UnitID, billValue, latestAdditionTime uint64, pubKey []byte, counter uint64) error {
	txFee := uint64(1)
	feeAmount := uint64(2)

	// create transferFC
	transferFC, err := createTransferFC(feeAmount+txFee, billID, fcrID, latestAdditionTime, counter)
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
	addFC, err := createAddFC(fcrID, templates.AlwaysTrueBytes(), transferFCProof.TxRecord, transferFCProof.TxProof, latestAdditionTime, feeAmount)
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
	transferTx, err := createTransferTx(pubKey, billID, billValue-feeAmount-txFee, fcrID, latestAdditionTime, counter+1)
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

func createTransferFC(feeAmount uint64, unitID []byte, targetUnitID []byte, latestAdditionTime, counter uint64) (*types.TransactionOrder, error) {
	attr, err := cbor.Marshal(
		&fc.TransferFeeCreditAttributes{
			Amount:                 feeAmount,
			TargetSystemIdentifier: 1,
			TargetRecordID:         targetUnitID,
			LatestAdditionTime:     latestAdditionTime,
			Counter:                counter,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transferFC attributes: %w", err)
	}
	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:       1,
			Type:           fc.PayloadTypeTransferFeeCredit,
			UnitID:         unitID,
			Attributes:     attr,
			ClientMetadata: &types.ClientMetadata{Timeout: latestAdditionTime, MaxTransactionFee: 1},
		},
		OwnerProof: nil,
	}
	return tx, nil
}

func createAddFC(unitID []byte, ownerCondition []byte, transferFC *types.TransactionRecord, transferFCProof *types.TxProof, latestAdditionTime uint64, maxFee uint64) (*types.TransactionOrder, error) {
	attr, err := cbor.Marshal(
		&fc.AddFeeCreditAttributes{
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
			Type:           fc.PayloadTypeAddFeeCredit,
			UnitID:         unitID,
			Attributes:     attr,
			ClientMetadata: &types.ClientMetadata{Timeout: latestAdditionTime, MaxTransactionFee: maxFee},
		},
		OwnerProof: nil,
	}, nil
}

func createTransferTx(pubKey []byte, unitID []byte, billValue uint64, fcrID []byte, timeout uint64, counter uint64) (*types.TransactionOrder, error) {
	attr, err := cbor.Marshal(
		&money.TransferAttributes{
			NewBearer:   templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(pubKey)),
			TargetValue: billValue,
			Counter:     counter,
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
