package tokens

import (
	"crypto/rand"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/wallet/account"
)

func TestParsePredicateArgument(t *testing.T) {
	mock := &accountManagerMock{keyHash: []byte{0x1, 0x2}}
	tests := []struct {
		input string
		// expectations:
		result    types.PredicateBytes
		accNumber uint64
		err       string
	}{
		{
			input:  "",
			result: nil,
		},
		{
			input:  "empty",
			result: nil,
		},
		{
			input:  "true",
			result: nil,
		},
		{
			input:  "false",
			result: nil,
		},
		{
			input:  "0x",
			result: nil,
		},
		{
			input:  "0x5301",
			result: []byte{0x53, 0x01},
		},
		{
			input: "0xinvalid",
			err:   `encoding/hex: invalid byte: U+0069 'i'`,
		},
		{
			input: "foobar",
			err:   `invalid predicate argument: "foobar"`,
		},
		{
			input: "ptpkh:0x01",
			err:   `invalid key number: 'ptpkh:0x01': strconv.ParseUint: parsing "0x01": invalid syntax`,
		},
		{
			input: "ptpkh:0",
			err:   "invalid key number: 0",
		},
		{
			input:     "ptpkh",
			accNumber: uint64(1),
		},
		{
			input:     "ptpkh:1",
			accNumber: uint64(1),
		},
		{
			input:     "ptpkh:10",
			accNumber: uint64(10),
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			argument, err := parsePredicateArgument(tt.input, tt.accNumber, mock)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
				if tt.accNumber > 0 {
					require.Equal(t, tt.accNumber, argument.AccountNumber)
				} else {
					require.EqualValues(t, tt.result, argument.Argument)
				}
			}
		})
	}
}

func Test_parsePredicateArgument_file(t *testing.T) {
	// share temp dir for all the subtest
	tmpDir := t.TempDir()

	t.Run("empty argument", func(t *testing.T) {
		// provide file prefix but no name - this resolves to current working DIR!
		buf, err := parsePredicateArgument(filePrefix, 0, nil)
		require.ErrorIs(t, err, syscall.EISDIR)
		require.Empty(t, buf)
	})

	t.Run("nonexisting file", func(t *testing.T) {
		buf, err := parsePredicateArgument(filePrefix+filepath.Join(tmpDir, "doesnt.exist"), 0, nil)
		require.ErrorIs(t, err, fs.ErrNotExist)
		require.Empty(t, buf)
	})

	t.Run("success", func(t *testing.T) {
		data := make([]byte, 10)
		_, err := rand.Read(data)
		require.NoError(t, err)

		filename := filepath.Join(tmpDir, "predicate.cbor")
		require.NoError(t, os.WriteFile(filename, data, 0666))

		buf, err := parsePredicateArgument(filePrefix+filename, 0, nil)
		require.NoError(t, err)
		require.Equal(t, &PredicateInput{Argument: data}, buf)
	})
}

func TestParsePredicateClause(t *testing.T) {
	mock := &accountManagerMock{keyHash: []byte{0x1, 0x2}}
	tests := []struct {
		// inputs:
		clause    string
		accNumber uint64
		// expectations:
		predicate     []byte
		expectedIndex uint64
		err           string
	}{
		{
			clause:    "",
			predicate: templates.AlwaysTrueBytes(),
		}, {
			clause: "foo",
			err:    "invalid predicate clause",
		},
		{
			clause:    "0x53510087",
			predicate: []byte{0x53, 0x51, 0x00, 0x87},
		},
		{
			clause:    "true",
			predicate: templates.AlwaysTrueBytes(),
		},
		{
			clause:    "false",
			predicate: templates.AlwaysFalseBytes(),
		},
		{
			clause: "ptpkh:",
			err:    "invalid predicate clause",
		},
		{
			clause:        "ptpkh",
			expectedIndex: uint64(0),
			err:           "invalid key number: 0 in 'ptpkh'",
		},
		{
			clause:        "ptpkh",
			accNumber:     2,
			expectedIndex: uint64(1),
			predicate:     templates.NewP2pkh256BytesFromKeyHash(mock.keyHash),
		},
		{
			clause: "ptpkh:0",
			err:    "invalid key number: 0",
		},
		{
			clause:        "ptpkh:2",
			expectedIndex: uint64(1),
			predicate:     templates.NewP2pkh256BytesFromKeyHash(mock.keyHash),
		},
		{
			clause:    "ptpkh:0x0102",
			predicate: templates.NewP2pkh256BytesFromKeyHash(mock.keyHash),
		},
		{
			clause: "ptpkh:0X",
			err:    "invalid predicate clause",
		},
	}

	for _, tt := range tests {
		t.Run(tt.clause, func(t *testing.T) {
			mock.recordedIndex = 0
			predicate, err := ParsePredicateClause(tt.clause, tt.accNumber, mock)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.predicate, predicate)
			require.Equal(t, tt.expectedIndex, mock.recordedIndex)
		})
	}
}

func Test_ParsePredicateClause_file(t *testing.T) {
	// share temp dir for all the subtest
	tmpDir := t.TempDir()

	t.Run("empty argument", func(t *testing.T) {
		// provide file prefix but no name - this resolves to current working DIR!
		buf, err := ParsePredicateClause(filePrefix, 0, nil)
		require.ErrorIs(t, err, syscall.EISDIR)
		require.Empty(t, buf)
	})

	t.Run("nonexisting file", func(t *testing.T) {
		buf, err := ParsePredicateClause(filePrefix+filepath.Join(tmpDir, "some.random.name"), 0, nil)
		require.ErrorIs(t, err, fs.ErrNotExist)
		require.Empty(t, buf)
	})

	t.Run("success", func(t *testing.T) {
		data := make([]byte, 10)
		_, err := rand.Read(data)
		require.NoError(t, err)

		filename := filepath.Join(tmpDir, "predicate.cbor")
		require.NoError(t, os.WriteFile(filename, data, 0666))

		buf, err := ParsePredicateClause(filePrefix+filename, 0, nil)
		require.NoError(t, err)
		require.Equal(t, data, buf)
	})
}

func TestDecodeHexOrEmpty(t *testing.T) {
	tests := []struct {
		input  string
		result []byte
		err    string
	}{
		{
			input:  "",
			result: nil,
		},
		{
			input:  "empty",
			result: nil,
		},
		{
			input:  "0x",
			result: nil,
		},
		{
			input: "0x534",
			err:   "odd length hex string",
		},
		{
			input: "0x53q",
			err:   "invalid byte",
		},
		{
			input:  "53",
			result: []byte{0x53},
		},
		{
			input:  "0x5354",
			result: []byte{0x53, 0x54},
		},
		{
			input:  "5354",
			result: []byte{0x53, 0x54},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			res, err := DecodeHexOrEmpty(tt.input)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.result, res)
			}
		})
	}
}

type accountManagerMock struct {
	keyHash       []byte
	recordedIndex uint64
}

func (a *accountManagerMock) GetAccountKey(accountIndex uint64) (*account.AccountKey, error) {
	a.recordedIndex = accountIndex
	return &account.AccountKey{PubKeyHash: &account.KeyHashes{Sha256: a.keyHash}}, nil
}

func (a *accountManagerMock) GetAll() []account.Account {
	return nil
}

func (a *accountManagerMock) CreateKeys(mnemonic string) error {
	return nil
}

func (a *accountManagerMock) AddAccount() (uint64, []byte, error) {
	return 0, nil, nil
}

func (a *accountManagerMock) GetMnemonic() (string, error) {
	return "", nil
}

func (a *accountManagerMock) GetAccountKeys() ([]*account.AccountKey, error) {
	return nil, nil
}

func (a *accountManagerMock) GetMaxAccountIndex() (uint64, error) {
	return 0, nil
}

func (a *accountManagerMock) GetPublicKey(accountIndex uint64) ([]byte, error) {
	return nil, nil
}

func (a *accountManagerMock) GetPublicKeys() ([][]byte, error) {
	return nil, nil
}

func (a *accountManagerMock) IsEncrypted() (bool, error) {
	return false, nil
}

func (a *accountManagerMock) Close() {
}
