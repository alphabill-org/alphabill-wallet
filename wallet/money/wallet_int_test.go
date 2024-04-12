package money

import (
	"context"
	"crypto"
	"net"
	"net/http"
	"testing"
	"time"

	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"

	rpcclient "github.com/alphabill-org/alphabill-wallet/client/rpc"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils"
	testfees "github.com/alphabill-org/alphabill-wallet/internal/testutils/fees"
	testobserve "github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/partition"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/fees"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/testutil"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
)

var (
	fcrID     = money.NewFeeCreditRecordID(nil, []byte{1})
	fcrAmount = uint64(1e8)
)

/*
Test scenario:
wallet account 1 sends two bills to wallet accounts 2 and 3
wallet runs dust collection
wallet account 2 and 3 should have only single bill
*/
func TestCollectDustInMultiAccountWallet(t *testing.T) {
	observe := testobserve.Default(t)

	// setup account
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)
	defer am.Close()
	err = CreateNewWallet(am, "")
	require.NoError(t, err)
	accKey, err := am.GetAccountKey(0)
	require.NoError(t, err)

	// start server
	genesisConfig := &testutil.MoneyGenesisConfig{
		InitialBillID:    money.NewBillID(nil, []byte{1}),
		InitialBillValue: 10000 * 1e8,
		InitialBillOwner: templates.NewP2pkh256BytesFromKey(accKey.PubKey),
	}
	network := startMoneyOnlyAlphabillPartition(t, genesisConfig)
	moneyPart, err := network.GetNodePartition(money.DefaultSystemIdentifier)
	require.NoError(t, err)
	addr := initRPCServer(t, moneyPart.Nodes[0])

	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)

	moneyClient, err := rpcclient.DialContext(ctx, "http://"+addr+"/rpc")
	require.NoError(t, err)
	defer moneyClient.Close()

	feeManagerDB, err := fees.NewFeeManagerDB(dir)
	require.NoError(t, err)
	defer feeManagerDB.Close()

	w, err := LoadExistingWallet(am, feeManagerDB, moneyClient, observe.Logger())
	require.NoError(t, err)
	defer w.Close()

	_, _, _ = am.AddAccount()
	_, _, _ = am.AddAccount()

	pubKeys, err := am.GetPublicKeys()
	require.NoError(t, err)

	// create fee credit for initial bill transfer
	_ = testfees.CreateFeeCredit(t, genesisConfig.InitialBillID, fcrID, fcrAmount, accKey.PrivKey, accKey.PubKey, network)
	initialBillCounter := uint64(1)
	initialBillValue := genesisConfig.InitialBillValue - fcrAmount

	// transfer initial bill to wallet 1
	transferInitialBillTx, err := testutil.CreateInitialBillTransferTx(accKey, genesisConfig.InitialBillID, fcrID, initialBillValue, 10000, initialBillCounter)
	require.NoError(t, err)
	batch := txsubmitter.NewBatch(w.rpcClient, observe.Logger())
	batch.Add(&txsubmitter.TxSubmission{
		UnitID:      transferInitialBillTx.UnitID(),
		TxHash:      transferInitialBillTx.Hash(crypto.SHA256),
		Transaction: transferInitialBillTx,
	})
	err = batch.SendTx(ctx, false)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPart, transferInitialBillTx), test.WaitDuration, test.WaitTick)

	// verify initial bill tx is received by wallet
	require.Eventually(t, func() bool {
		balance, _ := w.GetBalance(ctx, GetBalanceCmd{})
		return balance == initialBillValue
	}, test.WaitDuration, time.Second)

	// add fee credit to account 1
	_, err = w.AddFeeCredit(ctx, fees.AddFeeCmd{
		Amount:       1e8,
		AccountIndex: 0,
	})
	require.NoError(t, err)

	// send two bills to account number 2 and 3
	sendTo(t, w, []ReceiverData{
		{Amount: 10 * 1e8, PubKey: pubKeys[1]},
		{Amount: 10 * 1e8, PubKey: pubKeys[1]},
		{Amount: 10 * 1e8, PubKey: pubKeys[2]},
		{Amount: 10 * 1e8, PubKey: pubKeys[2]},
	}, 0)

	// add fee credit to account 2
	_, err = w.AddFeeCredit(ctx, fees.AddFeeCmd{
		Amount:       1e8,
		AccountIndex: 1,
	})
	require.NoError(t, err)

	// add fee credit to account 3
	_, err = w.AddFeeCredit(ctx, fees.AddFeeCmd{
		Amount:       1e8,
		AccountIndex: 2,
	})
	require.NoError(t, err)

	// start dust collection
	_, err = w.CollectDust(ctx, 0)
	require.NoError(t, err)
}

func sendTo(t *testing.T, w *Wallet, receivers []ReceiverData, fromAccount uint64) {
	proof, err := w.Send(context.Background(), SendCmd{
		Receivers:           receivers,
		AccountIndex:        fromAccount,
		WaitForConfirmation: true,
	})
	require.NoError(t, err)
	require.NotNil(t, proof)
}

func startMoneyOnlyAlphabillPartition(t *testing.T, genesisConfig *testutil.MoneyGenesisConfig) *testpartition.AlphabillNetwork {
	genesisConfig.DCMoneySupplyValue = 10000 * 1e8
	genesisConfig.SDRs = createSDRs()
	genesisState := testutil.MoneyGenesisState(t, genesisConfig)
	mPart, err := testpartition.NewPartition(t, "money node", 1, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		system, err := money.NewTxSystem(
			testobserve.Default(t),
			money.WithSystemIdentifier(money.DefaultSystemIdentifier),
			money.WithSystemDescriptionRecords(createSDRs()),
			money.WithTrustBase(tb),
			money.WithState(genesisState),
		)
		require.NoError(t, err)
		return system
	}, money.DefaultSystemIdentifier, genesisState)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{mPart})
	require.NoError(t, err)
	require.NoError(t, abNet.Start(t))
	t.Cleanup(func() { abNet.WaitClose(t) })
	return abNet
}

func initRPCServer(t *testing.T, partitionNode *testpartition.PartitionNode) string {
	node := partitionNode.Node
	server := ethrpc.NewServer()
	t.Cleanup(server.Stop)

	stateAPI := rpc.NewStateAPI(node, partitionNode.OwnerIndexer)
	err := server.RegisterName("state", stateAPI)
	require.NoError(t, err)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = listener.Close()
	})

	httpServer := &http.Server{
		Addr:    listener.Addr().String(),
		Handler: server,
	}

	go httpServer.Serve(listener)
	t.Cleanup(func() {
		_ = httpServer.Close()
	})

	return listener.Addr().String()
}

func createSDRs() []*types.SystemDescriptionRecord {
	return []*types.SystemDescriptionRecord{{
		SystemIdentifier: money.DefaultSystemIdentifier,
		T2Timeout:        2500,
		FeeCreditBill: &types.FeeCreditBill{
			UnitID:         money.NewBillID(nil, []byte{2}),
			OwnerPredicate: templates.AlwaysTrueBytes(),
		},
	}}
}
