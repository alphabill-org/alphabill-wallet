package money

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	testsig "github.com/alphabill-org/alphabill-go-base/testutils/sig"
	sdkmoney "github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	rpcclient "github.com/alphabill-org/alphabill-wallet/client/rpc"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils"
	testfees "github.com/alphabill-org/alphabill-wallet/internal/testutils/fees"
	testobserve "github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/partition"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/fees"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/testutil"
	"github.com/alphabill-org/alphabill-wallet/wallet/txsubmitter"
	"github.com/alphabill-org/alphabill/rpc"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/money"
	ethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/stretchr/testify/require"
)

const (
	fcrLatestAdditionTime = 66536
)

/*
Test scenario:
wallet account 1 sends two bills to wallet accounts 2 and 3
wallet runs dust collection
wallet account 2 and 3 should have only single bill
*/
func TestCollectDustInMultiAccountWallet(t *testing.T) {
	observe := testobserve.Default(t)
	signer, _ := testsig.CreateSignerAndVerifier(t)

	// setup account
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)
	defer am.Close()
	err = GenerateKeys(am, "")
	require.NoError(t, err)
	accKey, err := am.GetAccountKey(0)
	require.NoError(t, err)

	// start server
	genesisConfig := &testutil.MoneyGenesisConfig{
		InitialBillID:    sdkmoney.NewBillID(nil, []byte{1}),
		InitialBillValue: 10000 * 1e8,
		InitialBillOwner: templates.NewP2pkh256BytesFromKey(accKey.PubKey),
	}
	network := startMoneyOnlyAlphabillPartition(t, genesisConfig)
	moneyPart, err := network.GetNodePartition(sdkmoney.DefaultSystemID)
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

	w, err := NewWallet(am, feeManagerDB, moneyClient, observe.Logger())
	require.NoError(t, err)
	defer w.Close()

	_, _, _ = am.AddAccount()
	_, _, _ = am.AddAccount()

	pubKeys, err := am.GetPublicKeys()
	require.NoError(t, err)

	// create fee credit for initial bill transfer
	fcrID := sdkmoney.NewFeeCreditRecordIDFromPublicKeyHash(nil, accKey.PubKeyHash.Sha256, fcrLatestAdditionTime)
	fcrAmount := uint64(1e8)
	_ = testfees.CreateFeeCredit(t, signer, genesisConfig.InitialBillID, fcrID, fcrAmount, fcrLatestAdditionTime, accKey, network)
	initialBillCounter := uint64(1)
	initialBillValue := genesisConfig.InitialBillValue - fcrAmount

	// transfer initial bill to wallet 1
	transferInitialBillTx, err := testutil.CreateInitialBillTransferTx(accKey, genesisConfig.InitialBillID, fcrID, initialBillValue, 10000, initialBillCounter)
	require.NoError(t, err)
	batch := txsubmitter.NewBatch(w.rpcClient, observe.Logger())
	batch.Add(txsubmitter.New(transferInitialBillTx))
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
	mPart, err := testpartition.NewPartition(t, "money node", 1, func(tb types.RootTrustBase) txsystem.TransactionSystem {
		system, err := money.NewTxSystem(
			testobserve.Default(t),
			money.WithSystemIdentifier(sdkmoney.DefaultSystemID),
			money.WithSystemDescriptionRecords(createSDRs()),
			money.WithTrustBase(tb),
			money.WithState(genesisState),
		)
		require.NoError(t, err)
		return system
	}, sdkmoney.DefaultSystemID, genesisState)
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
		SystemIdentifier: sdkmoney.DefaultSystemID,
		T2Timeout:        2500,
		FeeCreditBill: &types.FeeCreditBill{
			UnitID:         sdkmoney.NewBillID(nil, []byte{2}),
			OwnerPredicate: templates.AlwaysTrueBytes(),
		},
	}}
}
