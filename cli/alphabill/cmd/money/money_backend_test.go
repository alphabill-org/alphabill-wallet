package money

import (
	"context"
	"crypto"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/predicates/templates"
	moneytx "github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	types2 "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/http"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/net"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/partition"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/backend"
)

var (
	defaultInitialBillID = moneytx.NewBillID(nil, []byte{1})
)

func TestMoneyBackendCLI(t *testing.T) {
	// create ab network
	initialBill := &moneytx.InitialBill{
		ID:    defaultInitialBillID,
		Value: 1e18,
		Owner: templates.AlwaysTrueBytes(),
	}
	moneyPartition := testutils.CreateMoneyPartition(t, initialBill, 1)
	abNet := testutils.StartAlphabill(t, []*testpartition.NodePartition{moneyPartition})
	testutils.StartPartitionRPCServers(t, moneyPartition)
	alphabillNodeAddr := moneyPartition.Nodes[0].AddrGRPC

	// transfer initial bill to wallet pubkey
	pk := "0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"
	pkBytes, _ := PubKeyHexToBytes(pk)
	initialBillValue := testutils.SpendInitialBillWithFeeCredits(t, abNet, initialBill, pkBytes)

	// start wallet-backend service
	homedir := testutils.SetupTestHomeDir(t, "money-backend-test")
	port, err := net.GetFreePort()
	require.NoError(t, err)
	serverAddr := fmt.Sprintf("localhost:%d", port)
	go func() {
		cmd := NewMoneyBackendCmd(&types2.BaseConfiguration{HomeDir: homedir, Observe: observability.Default(t)})
		args := fmt.Sprintf("start --%s %s --%s %s", serverAddrCmdName, serverAddr, alphabillNodeURLCmdName, alphabillNodeAddr)
		cmd.SetArgs(strings.Split(args, " "))
		ctx, cancelFunc := context.WithCancel(context.Background())
		t.Cleanup(cancelFunc)
		err = cmd.ExecuteContext(ctx)
		require.ErrorIs(t, err, context.Canceled)
	}()

	// wait for wallet-backend to index the transaction by verifying balance
	require.Eventually(t, func() bool {
		// verify balance
		res := &backend.BalanceResponse{}
		httpRes, _ := testhttp.DoGetJson(fmt.Sprintf("http://%s/api/v1/balance?pubkey=%s", serverAddr, pk), res)
		return httpRes != nil && httpRes.StatusCode == 200 && res.Balance == initialBillValue
	}, test.WaitDuration, test.WaitTick)

	// verify /list-bills
	resListBills := &backend.ListBillsResponse{}
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://%s/api/v1/list-bills?pubkey=%s", serverAddr, pk), resListBills)
	require.NoError(t, err)
	require.EqualValues(t, 200, httpRes.StatusCode)
	require.Len(t, resListBills.Bills, 1)
	b := resListBills.Bills[0]
	require.Equal(t, initialBillValue, b.Value)
	require.Equal(t, initialBill.ID, types.UnitID(b.Id))
	require.NotNil(t, b.TxHash)

	// verify proof
	resBlockProof := &wallet.Proof{}
	httpRes, err = testhttp.DoGetCbor(fmt.Sprintf("http://%s/api/v1/units/0x%s/transactions/0x%x/proof", serverAddr, initialBill.ID, b.TxHash), resBlockProof)
	require.NoError(t, err)
	require.EqualValues(t, 200, httpRes.StatusCode)
	require.Equal(t, resBlockProof.TxRecord.TransactionOrder.Hash(crypto.SHA256), b.TxHash)
}

func TestMoneyBackendConfig_DbFileParentDirsAreCreated(t *testing.T) {
	expectedFilePath := filepath.Join(t.TempDir(), "non-existent-dir", "my.db")
	c := &moneyBackendConfig{DbFile: expectedFilePath}
	_, err := c.GetDbFile()
	require.NoError(t, err)
	require.True(t, util.FileExists(filepath.Dir(expectedFilePath)))
}

func PubKeyHexToBytes(s string) ([]byte, bool) {
	if len(s) != 68 {
		return nil, false
	}
	pubKeyBytes, err := hexutil.Decode(s)
	if err != nil {
		return nil, false
	}
	return pubKeyBytes, true
}
