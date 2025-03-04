package money

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/logger"
	testmoney "github.com/alphabill-org/alphabill-wallet/internal/testutils/money"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/fees"
)

const (
	testMnemonic    = "dinosaur simple verify deliver bless ridge monkey design venue six problem lucky"
	testPubKey0Hex  = "03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"
	testPubKey0Hash = "f52022bb450407d92f13bf1c53128a676bcf304818e9f41a5ef4ebeae9c0d6b0"
	testPubKey1Hex  = "02d36c574db299904b285aaeb57eb7b1fa145c43af90bec3c635c4174c224587b6"
	testPubKey2Hex  = "02f6cbeacfd97ebc9b657081eb8b6c9ed3a588646d618ddbd03e198290af94c9d2"
	maxFee          = 10
)

func TestExistingWalletCanBeLoaded(t *testing.T) {
	homedir := t.TempDir()
	am, err := account.NewManager(homedir, "", true)
	require.NoError(t, err)
	rpcClient := testmoney.NewRpcClientMock()
	feeManagerDB, err := fees.NewFeeManagerDB(homedir)
	require.NoError(t, err)
	_, err = NewWallet(context.Background(), am, feeManagerDB, rpcClient, maxFee, logger.New(t))
	require.NoError(t, err)
}

func TestWallet_GetPublicKey(t *testing.T) {
	rpcClient := testmoney.NewRpcClientMock()
	w := createTestWallet(t, rpcClient)
	pubKey, err := w.am.GetPublicKey(0)
	require.NoError(t, err)
	require.EqualValues(t, "0x"+testPubKey0Hex, hexutil.Encode(pubKey))
}

func TestWallet_GetPublicKeys(t *testing.T) {
	rpcClient := testmoney.NewRpcClientMock()
	w := createTestWallet(t, rpcClient)
	_, _, _ = w.am.AddAccount()

	pubKeys, err := w.am.GetPublicKeys()
	require.NoError(t, err)
	require.Len(t, pubKeys, 2)
	require.EqualValues(t, "0x"+testPubKey0Hex, hexutil.Encode(pubKeys[0]))
	require.EqualValues(t, "0x"+testPubKey1Hex, hexutil.Encode(pubKeys[1]))
}

func TestWallet_AddKey(t *testing.T) {
	rpcClient := testmoney.NewRpcClientMock()
	w := createTestWallet(t, rpcClient)

	accIdx, accPubKey, err := w.am.AddAccount()
	require.NoError(t, err)
	require.EqualValues(t, 1, accIdx)
	require.EqualValues(t, "0x"+testPubKey1Hex, hexutil.Encode(accPubKey))
	accIdx, _ = w.am.GetMaxAccountIndex()
	require.EqualValues(t, 1, accIdx)

	accIdx, accPubKey, err = w.am.AddAccount()
	require.NoError(t, err)
	require.EqualValues(t, 2, accIdx)
	require.EqualValues(t, "0x"+testPubKey2Hex, hexutil.Encode(accPubKey))
	accIdx, _ = w.am.GetMaxAccountIndex()
	require.EqualValues(t, 2, accIdx)
}

func TestWallet_GetBalance(t *testing.T) {
	rpcClient := testmoney.NewRpcClientMock(
		testmoney.WithOwnerBill(testmoney.NewBill(t, 10, 1)),
	)
	w := createTestWallet(t, rpcClient)
	balance, err := w.GetBalance(context.Background(), GetBalanceCmd{})
	require.NoError(t, err)
	require.EqualValues(t, 10, balance)
}

func TestWallet_GetBalances(t *testing.T) {
	rpcClient := testmoney.NewRpcClientMock(
		testmoney.WithOwnerBill(testmoney.NewBill(t, 10, 1)),
	)
	w := createTestWallet(t, rpcClient)
	_, _, err := w.am.AddAccount()
	require.NoError(t, err)

	balances, sum, err := w.GetBalances(context.Background(), GetBalanceCmd{})
	require.NoError(t, err)
	require.EqualValues(t, 10, balances[0])
	require.EqualValues(t, 10, balances[1])
	require.EqualValues(t, 20, sum)
}

func createTestWallet(t *testing.T, moneyClient sdktypes.MoneyPartitionClient) *Wallet {
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)

	err = GenerateKeys(am, testMnemonic)
	require.NoError(t, err)

	feeManagerDB, err := fees.NewFeeManagerDB(dir)
	require.NoError(t, err)

	w, err := NewWallet(context.Background(), am, feeManagerDB, moneyClient, maxFee, logger.New(t))
	require.NoError(t, err)

	return w
}
