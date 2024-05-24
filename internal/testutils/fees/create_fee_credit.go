package testfees

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill-wallet/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill-wallet/internal/testutils/partition"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/txbuilder"
	testfc "github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

// CreateFeeCredit creates fee credit to be able to spend initial bill
func CreateFeeCredit(t *testing.T, signer abcrypto.Signer, initialBillID, fcrID types.UnitID, fcrAmount uint64, accountKey *account.AccountKey, network *testpartition.AlphabillNetwork) *types.TransactionOrder {
	// send transferFC
	transferFC := testfc.NewTransferFC(t, signer,
		testfc.NewTransferFCAttr(t, signer,
			testfc.WithCounter(0),
			testfc.WithAmount(fcrAmount),
			testfc.WithTargetRecordID(fcrID),
		),
		testtransaction.WithUnitID(initialBillID),
		testtransaction.WithPayloadType(fc.PayloadTypeTransferFeeCredit),
	)
	transferFC, err := txbuilder.SignPayload(transferFC.Payload, accountKey)
	require.NoError(t, err)

	moneyPartition, err := network.GetNodePartition(1)
	require.NoError(t, err)
	require.NoError(t, moneyPartition.SubmitTx(transferFC))

	transferFCRecord, transferFCProof, err := testpartition.WaitTxProof(t, moneyPartition, transferFC)
	require.NoError(t, err, "transfer fee credit tx failed")
	// send addFC
	addFC := testfc.NewAddFC(t, network.RootPartition.Nodes[0].RootSigner,
		testfc.NewAddFCAttr(t, network.RootPartition.Nodes[0].RootSigner,
			testfc.WithTransferFCRecord(transferFCRecord),
			testfc.WithTransferFCProof(transferFCProof),
			testfc.WithFCOwnerCondition(templates.NewP2pkh256BytesFromKey(accountKey.PubKey)),
		),
		testtransaction.WithUnitID(fcrID),
		testtransaction.WithPayloadType(fc.PayloadTypeAddFeeCredit),
	)
	addFC, err = txbuilder.SignPayload(addFC.Payload, accountKey)
	require.NoError(t, err)
	require.NoError(t, moneyPartition.SubmitTx(addFC))
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPartition, addFC), test.WaitDuration, test.WaitTick)
	return transferFCRecord.TransactionOrder
}
