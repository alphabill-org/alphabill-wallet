package txbuilder

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/testutil"
)

const testMnemonic = "dinosaur simple verify deliver bless ridge monkey design venue six problem lucky"

var (
	receiverPubKey, _ = hexutil.Decode("0x1234511c7341399e876800a268855c894c43eb849a72ac5a9d26a0091041c12345")
	accountKey, _     = account.NewKeys(testMnemonic)
)

func TestCreateTransactions(t *testing.T) {
	tests := []struct {
		name        string
		bills       []*sdktypes.Bill
		amount      uint64
		txCount     int
		verify      func(t *testing.T, partitionID types.PartitionID, txs []*types.TransactionOrder)
		expectedErr string
	}{
		{
			name:   "have more bills than target amount",
			bills:  []*sdktypes.Bill{createBill(5), createBill(3), createBill(1)},
			amount: uint64(7),
			verify: func(t *testing.T, partitionID types.PartitionID, txs []*types.TransactionOrder) {
				// verify tx count
				require.Len(t, txs, 2)

				// verify first tx is transfer order of bill no1
				tx := txs[0]
				require.Equal(t, money.TransactionTypeTransfer, tx.Type)
				transferAttr := &money.TransferAttributes{}
				err := tx.UnmarshalAttributes(transferAttr)
				require.NoError(t, err)
				require.EqualValues(t, 5, transferAttr.TargetValue)

				// verify second tx is split order of bill no2
				tx = txs[1]
				require.Equal(t, money.TransactionTypeSplit, tx.Type)
				splitAttr := &money.SplitAttributes{}
				err = tx.UnmarshalAttributes(splitAttr)
				require.NoError(t, err)
				require.EqualValues(t, 2, splitAttr.TargetUnits[0].Amount)
			},
		},
		{
			name:   "have less bills than target amount",
			bills:  []*sdktypes.Bill{createBill(5), createBill(1)},
			amount: uint64(7),
			verify: func(t *testing.T, partitionID types.PartitionID, txs []*types.TransactionOrder) {
				require.Empty(t, txs)
			},
			expectedErr: "insufficient balance",
		},
		{
			name:   "have exact amount of bills than target amount",
			bills:  []*sdktypes.Bill{createBill(5), createBill(5)},
			amount: uint64(10),
			verify: func(t *testing.T, partitionID types.PartitionID, txs []*types.TransactionOrder) {
				// verify tx count
				require.Len(t, txs, 2)

				// verify both bills are transfer orders
				for _, tx := range txs {
					require.Equal(t, money.TransactionTypeTransfer, tx.Type)
					transferAttr := &money.TransferAttributes{}
					err := tx.UnmarshalAttributes(transferAttr)
					require.NoError(t, err)
					require.EqualValues(t, 5, transferAttr.TargetValue)
				}
			},
		},
		{
			name:   "have exactly one bill with equal target amount",
			bills:  []*sdktypes.Bill{createBill(5)},
			amount: uint64(5),
			verify: func(t *testing.T, partitionID types.PartitionID, txs []*types.TransactionOrder) {
				// verify tx count
				require.Len(t, txs, 1)

				// verify transfer tx
				require.Equal(t, money.TransactionTypeTransfer, txs[0].Type)
				transferAttr := &money.TransferAttributes{}
				err := txs[0].UnmarshalAttributes(transferAttr)
				require.NoError(t, err)
				require.EqualValues(t, 5, transferAttr.TargetValue)
			},
		},
	}

	partitionID := money.DefaultPartitionID

	txSigner, err := sdktypes.NewMoneyTxSignerFromKey(accountKey.AccountKey.PrivKey)
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txs, err := CreateTransactions(receiverPubKey, tt.amount, tt.bills, txSigner, 100, nil, nil, 10)
			if tt.expectedErr != "" {
				require.ErrorContains(t, err, tt.expectedErr)
			} else {
				require.NoError(t, err)
				tt.verify(t, partitionID, txs)
			}
		})
	}
}

func createBill(value uint64) *sdktypes.Bill {
	return testutil.NewBill(value, 0)
}
