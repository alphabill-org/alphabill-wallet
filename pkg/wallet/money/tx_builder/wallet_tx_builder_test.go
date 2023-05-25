package tx_builder

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

const testMnemonic = "dinosaur simple verify deliver bless ridge monkey design venue six problem lucky"

var (
	receiverPubKey, _ = hexutil.Decode("0x1234511c7341399e876800a268855c894c43eb849a72ac5a9d26a0091041c12345")
	accountKey, _     = account.NewKeys(testMnemonic)
)

func TestSplitTransactionAmount(t *testing.T) {
	receiverPubKeyHash := hash.Sum256(receiverPubKey)
	keys, _ := account.NewKeys(testMnemonic)
	billID := uint256.NewInt(0)
	billIDBytes32 := billID.Bytes32()
	billIDBytes := billIDBytes32[:]
	b := &wallet.Bill{
		Id:     billIDBytes,
		Value:  500,
		TxHash: []byte{1, 2, 3, 4},
	}
	amount := uint64(150)
	timeout := uint64(100)
	systemID := []byte{0, 0, 0, 0}

	tx, err := NewSplitTx(amount, receiverPubKey, keys.AccountKey, systemID, b, timeout, nil)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, systemID, tx.SystemID())
	require.EqualValues(t, billIDBytes, tx.UnitID())
	require.EqualValues(t, timeout, tx.Timeout())
	require.NotNil(t, tx.OwnerProof)

	so := &moneytx.SplitAttributes{}
	err = tx.UnmarshalAttributes(so)
	require.NoError(t, err)
	require.Equal(t, amount, so.Amount)
	require.EqualValues(t, script.PredicatePayToPublicKeyHashDefault(receiverPubKeyHash), so.TargetBearer)
	require.EqualValues(t, 350, so.RemainingValue)
	require.EqualValues(t, b.TxHash, so.Backlink)
}

func TestCreateTransactions(t *testing.T) {
	tests := []struct {
		name        string
		bills       []*wallet.Bill
		amount      uint64
		txCount     int
		verify      func(t *testing.T, systemID []byte, txs []*types.TransactionOrder)
		expectedErr string
	}{
		{
			name:   "have more bills than target amount",
			bills:  []*wallet.Bill{createBill(5), createBill(3), createBill(1)},
			amount: uint64(7),
			verify: func(t *testing.T, systemID []byte, txs []*types.TransactionOrder) {
				// verify tx count
				require.Len(t, txs, 2)

				// verify first tx is transfer order of bill no1
				tx := txs[0]
				require.Equal(t, moneytx.PayloadTypeTransfer, tx.PayloadType())
				transferAttr := &moneytx.TransferAttributes{}
				err := tx.UnmarshalAttributes(transferAttr)
				require.NoError(t, err)
				require.EqualValues(t, 5, transferAttr.TargetValue)
				require.NoError(t, err)

				// verify second tx is split order of bill no2
				tx = txs[1]
				require.Equal(t, moneytx.PayloadTypeSplit, tx.PayloadType())
				splitAttr := &moneytx.SplitAttributes{}
				err = tx.UnmarshalAttributes(splitAttr)
				require.NoError(t, err)
				require.EqualValues(t, 2, splitAttr.Amount)
			},
		},
		{
			name:   "have less bills than target amount",
			bills:  []*wallet.Bill{createBill(5), createBill(1)},
			amount: uint64(7),
			verify: func(t *testing.T, systemID []byte, txs []*types.TransactionOrder) {
				require.Empty(t, txs)
			},
			expectedErr: "insufficient balance",
		},
		{
			name:   "have exact amount of bills than target amount",
			bills:  []*wallet.Bill{createBill(5), createBill(5)},
			amount: uint64(10),
			verify: func(t *testing.T, systemID []byte, txs []*types.TransactionOrder) {
				// verify tx count
				require.Len(t, txs, 2)

				// verify both bills are transfer orders
				for _, tx := range txs {
					require.Equal(t, moneytx.PayloadTypeTransfer, tx.PayloadType())
					transferAttr := &moneytx.TransferAttributes{}
					err := tx.UnmarshalAttributes(transferAttr)
					require.NoError(t, err)
					require.EqualValues(t, 5, transferAttr.TargetValue)
				}
			},
		},
		{
			name:   "have exactly one bill with equal target amount",
			bills:  []*wallet.Bill{createBill(5)},
			amount: uint64(5),
			verify: func(t *testing.T, systemID []byte, txs []*types.TransactionOrder) {
				// verify tx count
				require.Len(t, txs, 1)

				// verify transfer tx
				require.Equal(t, moneytx.PayloadTypeTransfer, txs[0].PayloadType())
				transferAttr := &moneytx.TransferAttributes{}
				err := txs[0].UnmarshalAttributes(transferAttr)
				require.NoError(t, err)
				require.EqualValues(t, 5, transferAttr.TargetValue)
			},
		},
	}

	systemID := []byte{0, 0, 0, 0}

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

func createBill(value uint64) *wallet.Bill {
	return &wallet.Bill{
		Value:  value,
		Id:     util.Uint64ToBytes32(0),
		TxHash: []byte{},
	}
}
