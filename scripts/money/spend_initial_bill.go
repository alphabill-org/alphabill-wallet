package main

import (
	"context"
	"crypto"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/args"
	"github.com/alphabill-org/alphabill-wallet/client"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
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
	partitionID := flag.Uint("partition-id", uint(money.DefaultPartitionID), "the partition identifier")
	networkID := flag.Uint("network-id", uint(types.NetworkLocal), "the network identifier")
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

	// create rpc client
	ctx := context.Background()
	moneyClient, err := client.NewMoneyPartitionClient(ctx, args.BuildRpcUrl(*rpcServerAddr))
	if err != nil {
		log.Fatal(err)
	}
	defer moneyClient.Close()

	pdr, err := moneyClient.PartitionDescription(ctx)
	if err != nil {
		log.Fatal("loading PDR:", err)
	}
	billID, err := pdr.ComposeUnitID(types.ShardID{}, money.BillUnitType, func(b []byte) error {
		binary.BigEndian.PutUint64(b[len(b)-8:], *billIdUint)
		return nil
	})
	if err != nil {
		log.Fatal("composing initial bill ID:", err)
	}

	// calculate fee credit record id
	roundNumber, err := moneyClient.GetRoundNumber(ctx)
	if err != nil {
		log.Fatal(err)
	}
	latestAdditionTime := roundNumber + *timeout
	fcrID, err := money.NewFeeCreditRecordIDFromOwnerPredicate(pdr, types.ShardID{}, templates.AlwaysTrueBytes(), latestAdditionTime)
	if err != nil {
		log.Fatal(err)
	}

	if err = execInitialBill(ctx, moneyClient, types.NetworkID(*networkID), types.PartitionID(*partitionID), billID, fcrID, *billValue, latestAdditionTime, pubKey, *counter); err != nil {
		log.Fatal(err)
	}
}

func execInitialBill(ctx context.Context, moneyClient sdktypes.PartitionClient, networkID types.NetworkID, partitionID types.PartitionID, billID, fcrID types.UnitID, billValue, latestAdditionTime uint64, pubKey []byte, counter uint64) error {
	txFee := uint64(1)
	feeAmount := uint64(2)

	// create transferFC
	transferFC, err := createTransferFC(networkID, partitionID, feeAmount+txFee, billID, fcrID, latestAdditionTime, counter)
	if err != nil {
		return fmt.Errorf("creating transfer FC transaction: %w", err)
	}

	// send transferFC
	log.Println("sending transferFC transaction")
	_, err = moneyClient.SendTransaction(ctx, transferFC)
	if err != nil {
		return fmt.Errorf("processing transfer FC transaction: %w", err)
	}
	// wait for transferFC proof
	transferFCProof, err := waitForConf(ctx, moneyClient, transferFC)
	if err != nil {
		return fmt.Errorf("failed to confirm transferFC transaction: %w", err)
	} else {
		log.Println("confirmed transferFC transaction")
	}

	// create addFC
	addFC, err := createAddFC(networkID, partitionID, fcrID, templates.AlwaysTrueBytes(), transferFCProof, latestAdditionTime, feeAmount)
	if err != nil {
		return fmt.Errorf("creating add FC transaction: %w", err)
	}
	// send addFC
	log.Println("sending addFC transaction")
	_, err = moneyClient.SendTransaction(ctx, addFC)
	if err != nil {
		return fmt.Errorf("processing add FC transaction: %w", err)
	}
	// wait for addFC confirmation
	_, err = waitForConf(ctx, moneyClient, addFC)
	if err != nil {
		return fmt.Errorf("failed to confirm addFC transaction: %w", err)
	} else {
		log.Println("confirmed addFC transaction")
	}

	// create transfer tx
	transferTx, err := createTransferTx(networkID, partitionID, pubKey, billID, billValue-feeAmount-txFee, fcrID, latestAdditionTime, counter+1)
	if err != nil {
		return fmt.Errorf("creating transfer transaction: %w", err)
	}
	// send transfer tx
	log.Println("sending initial bill transfer transaction")
	_, err = moneyClient.SendTransaction(ctx, transferTx)
	if err != nil {
		return fmt.Errorf("processing transfer transaction: %w", err)
	}
	// wait for transfer tx confirmation
	_, err = waitForConf(ctx, moneyClient, transferTx)
	if err != nil {
		return fmt.Errorf("failed to confirm transfer transaction: %w", err)
	} else {
		log.Println("successfully confirmed initial bill transfer transaction")
	}
	return nil
}

func createTransferFC(networkID types.NetworkID, partitionID types.PartitionID, feeAmount uint64, unitID []byte, targetUnitID []byte, latestAdditionTime, counter uint64) (*types.TransactionOrder, error) {
	attr, err := cbor.Marshal(
		&fc.TransferFeeCreditAttributes{
			Amount:             feeAmount,
			TargetPartitionID:  1,
			TargetRecordID:     targetUnitID,
			LatestAdditionTime: latestAdditionTime,
			Counter:            counter,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transferFC attributes: %w", err)
	}
	tx := &types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			NetworkID:      networkID,
			PartitionID:    partitionID,
			Type:           fc.TransactionTypeTransferFeeCredit,
			UnitID:         unitID,
			Attributes:     attr,
			ClientMetadata: &types.ClientMetadata{Timeout: latestAdditionTime, MaxTransactionFee: 1},
		},
	}
	if err = tx.SetAuthProof(fc.TransferFeeCreditAuthProof{OwnerProof: templates.EmptyArgument()}); err != nil {
		return nil, fmt.Errorf("failed to set auth proof: %w", err)
	}
	return tx, nil
}

func createAddFC(networkID types.NetworkID, partitionID types.PartitionID, unitID []byte, ownerPredicate []byte, transferFCProof *types.TxRecordProof, latestAdditionTime uint64, maxFee uint64) (*types.TransactionOrder, error) {
	attr, err := cbor.Marshal(
		&fc.AddFeeCreditAttributes{
			FeeCreditTransferProof:  transferFCProof,
			FeeCreditOwnerPredicate: ownerPredicate,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transferFC attributes: %w", err)
	}
	tx := &types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			NetworkID:      networkID,
			PartitionID:    partitionID,
			Type:           fc.TransactionTypeAddFeeCredit,
			UnitID:         unitID,
			Attributes:     attr,
			ClientMetadata: &types.ClientMetadata{Timeout: latestAdditionTime, MaxTransactionFee: maxFee},
		},
	}
	if err = tx.SetAuthProof(fc.AddFeeCreditAuthProof{OwnerProof: templates.EmptyArgument()}); err != nil {
		return nil, fmt.Errorf("failed to set auth proof: %w", err)
	}
	return tx, nil
}

func createTransferTx(networkID types.NetworkID, partitionID types.PartitionID, pubKey []byte, unitID []byte, billValue uint64, fcrID []byte, timeout uint64, counter uint64) (*types.TransactionOrder, error) {
	attr, err := cbor.Marshal(
		&money.TransferAttributes{
			NewOwnerPredicate: templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(pubKey)),
			TargetValue:       billValue,
			Counter:           counter,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal transfer attributes: %w", err)
	}
	tx := &types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			NetworkID:   networkID,
			PartitionID: partitionID,
			Type:        money.TransactionTypeTransfer,
			UnitID:      unitID,
			Attributes:  attr,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           timeout,
				MaxTransactionFee: 1,
				FeeCreditRecordID: fcrID,
			},
		},
	}
	if err = tx.SetAuthProof(money.TransferAuthProof{OwnerProof: templates.EmptyArgument()}); err != nil {
		return nil, fmt.Errorf("failed to set auth proof: %w", err)
	}
	return tx, nil
}

func waitForConf(ctx context.Context, c sdktypes.PartitionClient, tx *types.TransactionOrder) (*types.TxRecordProof, error) {
	txHash, err := tx.Hash(crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate transaction hash: %w", err)
	}
	for {
		// fetch round number before proof to ensure that we cannot miss the proof
		roundNumber, err := c.GetRoundNumber(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch target partition round number: %w", err)
		}
		proof, err := c.GetTransactionProof(ctx, txHash)
		if err != nil {
			return nil, err
		}
		if proof != nil {
			return proof, nil
		}
		if roundNumber >= tx.Timeout() {
			return nil, errors.New("transaction timed out")
		}
		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			return nil, errors.New("context canceled")
		}
	}
}
