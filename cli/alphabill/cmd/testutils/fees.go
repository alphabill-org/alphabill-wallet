package testutils

import (
	"testing"

	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"

	testpartition "github.com/alphabill-org/alphabill-wallet/internal/testutils/partition"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	txbuilder "github.com/alphabill-org/alphabill-wallet/wallet/money/tx_builder"
)

func AddFeeCredit(t *testing.T, amount uint64, systemID types.SystemID, accountKey *account.AccountKey, unitID, unitBacklink []byte, fcrID, fcrBacklink []byte, node *testpartition.NodePartition) {
	// create transferFC tx
	transferFCTx, err := txbuilder.NewTransferFCTx(amount, fcrID, fcrBacklink, accountKey, systemID, systemID, unitID, unitBacklink, 10000, 0, 10000)
	require.NoError(t, err)

	// submit transferFC tx
	err = node.SubmitTx(transferFCTx)
	require.NoError(t, err)

	// confirm transferFC tx
	transferFCRecord, transferFCProof, err := testpartition.WaitTxProof(t, node, transferFCTx)
	require.NoError(t, err, "transfer fee credit tx failed")

	// create addFC tx
	addFCTx, err := txbuilder.NewAddFCTx(fcrID, &wallet.Proof{TxProof: transferFCProof, TxRecord: transferFCRecord}, accountKey, systemID, 10000)
	require.NoError(t, err)

	// submit addFC tx
	err = node.SubmitTx(addFCTx)
	require.NoError(t, err)

	// confirm addFC tx
	_, _, err = testpartition.WaitTxProof(t, node, addFCTx)
	require.NoError(t, err, "add fee credit tx failed")
}
