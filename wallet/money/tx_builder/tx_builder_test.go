package tx_builder

import (
	"testing"

	"github.com/alphabill-org/alphabill/hash"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/util"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	mtypes "github.com/alphabill-org/alphabill-wallet/wallet/money/api"
)

const testMnemonic = "dinosaur simple verify deliver bless ridge monkey design venue six problem lucky"

var (
	receiverPubKey, _ = hexutil.Decode("0x1234511c7341399e876800a268855c894c43eb849a72ac5a9d26a0091041c12345")
	accountKey, _     = account.NewKeys(testMnemonic)
)

func TestSplitTransactionAmount(t *testing.T) {
	receiverPubKeyHash := hash.Sum256(receiverPubKey)
	keys, _ := account.NewKeys(testMnemonic)
	billID := money.NewBillID(nil, nil)
	b := &mtypes.Bill{
		ID: billID,
		BillData: &money.BillData{
			V:       500,
			Counter: 1234,
		},
	}
	amount := uint64(150)
	timeout := uint64(100)
	systemID := money.DefaultSystemIdentifier
	remainingValue := b.Value() - amount

	tx, err := NewSplitTx([]*money.TargetUnit{
		{OwnerCondition: templates.NewP2pkh256BytesFromKeyHash(receiverPubKeyHash), Amount: amount},
	}, remainingValue, keys.AccountKey, systemID, b, timeout, nil)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, systemID, tx.SystemID())
	require.EqualValues(t, billID, tx.UnitID())
	require.EqualValues(t, timeout, tx.Timeout())
	require.NotNil(t, tx.OwnerProof)

	so := &money.SplitAttributes{}
	err = tx.UnmarshalAttributes(so)
	require.NoError(t, err)
	require.Equal(t, amount, so.TargetUnits[0].Amount)
	require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(receiverPubKeyHash), so.TargetUnits[0].OwnerCondition)
	require.EqualValues(t, 350, so.RemainingValue)
	require.EqualValues(t, b.Counter(), so.Counter)
}

func TestCreateTransactions(t *testing.T) {
	tests := []struct {
		name        string
		bills       []*mtypes.Bill
		amount      uint64
		txCount     int
		verify      func(t *testing.T, systemID types.SystemID, txs []*types.TransactionOrder)
		expectedErr string
	}{
		{
			name:   "have more bills than target amount",
			bills:  []*mtypes.Bill{createBill(5), createBill(3), createBill(1)},
			amount: uint64(7),
			verify: func(t *testing.T, systemID types.SystemID, txs []*types.TransactionOrder) {
				// verify tx count
				require.Len(t, txs, 2)

				// verify first tx is transfer order of bill no1
				tx := txs[0]
				require.Equal(t, money.PayloadTypeTransfer, tx.PayloadType())
				transferAttr := &money.TransferAttributes{}
				err := tx.UnmarshalAttributes(transferAttr)
				require.NoError(t, err)
				require.EqualValues(t, 5, transferAttr.TargetValue)
				require.NoError(t, err)

				// verify second tx is split order of bill no2
				tx = txs[1]
				require.Equal(t, money.PayloadTypeSplit, tx.PayloadType())
				splitAttr := &money.SplitAttributes{}
				err = tx.UnmarshalAttributes(splitAttr)
				require.NoError(t, err)
				require.EqualValues(t, 2, splitAttr.TargetUnits[0].Amount)
			},
		},
		{
			name:   "have less bills than target amount",
			bills:  []*mtypes.Bill{createBill(5), createBill(1)},
			amount: uint64(7),
			verify: func(t *testing.T, systemID types.SystemID, txs []*types.TransactionOrder) {
				require.Empty(t, txs)
			},
			expectedErr: "insufficient balance",
		},
		{
			name:   "have exact amount of bills than target amount",
			bills:  []*mtypes.Bill{createBill(5), createBill(5)},
			amount: uint64(10),
			verify: func(t *testing.T, systemID types.SystemID, txs []*types.TransactionOrder) {
				// verify tx count
				require.Len(t, txs, 2)

				// verify both bills are transfer orders
				for _, tx := range txs {
					require.Equal(t, money.PayloadTypeTransfer, tx.PayloadType())
					transferAttr := &money.TransferAttributes{}
					err := tx.UnmarshalAttributes(transferAttr)
					require.NoError(t, err)
					require.EqualValues(t, 5, transferAttr.TargetValue)
				}
			},
		},
		{
			name:   "have exactly one bill with equal target amount",
			bills:  []*mtypes.Bill{createBill(5)},
			amount: uint64(5),
			verify: func(t *testing.T, systemID types.SystemID, txs []*types.TransactionOrder) {
				// verify tx count
				require.Len(t, txs, 1)

				// verify transfer tx
				require.Equal(t, money.PayloadTypeTransfer, txs[0].PayloadType())
				transferAttr := &money.TransferAttributes{}
				err := txs[0].UnmarshalAttributes(transferAttr)
				require.NoError(t, err)
				require.EqualValues(t, 5, transferAttr.TargetValue)
			},
		},
	}

	systemID := money.DefaultSystemIdentifier

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txs, err := CreateTransactions(receiverPubKey, tt.amount, systemID, tt.bills, accountKey.AccountKey, 100, nil)
			if tt.expectedErr != "" {
				require.ErrorContains(t, err, tt.expectedErr)
			} else {
				require.NoError(t, err)
				tt.verify(t, systemID, txs)
			}
		})
	}
}

func createBill(value uint64) *mtypes.Bill {
	return &mtypes.Bill{
		ID: util.Uint64ToBytes32(0),
		BillData: &money.BillData{
			V:       value,
			Counter: 0,
		},
	}
}
