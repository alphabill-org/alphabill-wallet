package testfees

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/predicates/templates"
	testfc "github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"

	test "github.com/alphabill-org/alphabill-wallet/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill-wallet/internal/testutils/partition"
)

// CreateFeeCredit creates fee credit to be able to spend initial bill
func CreateFeeCredit(t *testing.T, initialBillID, fcrID types.UnitID, fcrAmount uint64, privKey []byte, pubKey []byte, network *testpartition.AlphabillNetwork) *types.TransactionOrder {
	// send transferFC
	transferFC := testfc.NewTransferFC(t,
		testfc.NewTransferFCAttr(
			testfc.WithBacklink(nil),
			testfc.WithAmount(fcrAmount),
			testfc.WithTargetRecordID(fcrID),
		),
		testtransaction.WithUnitId(initialBillID),
		testtransaction.WithPayloadType(transactions.PayloadTypeTransferFeeCredit),
	)

	signer, _ := abcrypto.NewInMemorySecp256K1SignerFromKey(privKey)
	sigBytes, err := transferFC.PayloadBytes()
	require.NoError(t, err)
	sig, _ := signer.SignBytes(sigBytes)
	transferFC.OwnerProof = templates.NewP2pkh256SignatureBytes(sig, pubKey)

	moneyPartition, err := network.GetNodePartition(1)
	require.NoError(t, err)
	require.NoError(t, moneyPartition.SubmitTx(transferFC))

	transferFCRecord, transferFCProof, err := testpartition.WaitTxProof(t, moneyPartition, transferFC)
	require.NoError(t, err, "transfer fee credit tx failed")
	// send addFC
	addFC := testfc.NewAddFC(t, network.RootPartition.Nodes[0].RootSigner,
		testfc.NewAddFCAttr(t, network.RootPartition.Nodes[0].RootSigner,
			testfc.WithTransferFCTx(transferFCRecord),
			testfc.WithTransferFCProof(transferFCProof),
			testfc.WithFCOwnerCondition(templates.NewP2pkh256BytesFromKey(pubKey)),
		),
		testtransaction.WithUnitId(fcrID),
		testtransaction.WithOwnerProof(templates.NewP2pkh256BytesFromKey(pubKey)),
		testtransaction.WithPayloadType(transactions.PayloadTypeAddFeeCredit),
	)
	require.NoError(t, moneyPartition.SubmitTx(addFC))
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPartition, addFC), test.WaitDuration, test.WaitTick)
	return transferFCRecord.TransactionOrder
}
