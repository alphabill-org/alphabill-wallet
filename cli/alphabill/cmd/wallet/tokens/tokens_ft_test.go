package tokens

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/args"
	"github.com/alphabill-org/alphabill-wallet/client/rpc"
	test "github.com/alphabill-org/alphabill-wallet/internal/testutils"
	testobserve "github.com/alphabill-org/alphabill-wallet/internal/testutils/observability"
	testpartition "github.com/alphabill-org/alphabill-wallet/internal/testutils/partition"
	"github.com/alphabill-org/alphabill-wallet/util"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	"github.com/alphabill-org/alphabill-wallet/wallet/fees"
	moneywallet "github.com/alphabill-org/alphabill-wallet/wallet/money"
	"github.com/alphabill-org/alphabill-wallet/wallet/money/testutil"
	tokenswallet "github.com/alphabill-org/alphabill-wallet/wallet/tokens"
)

var defaultInitialBillID = money.NewBillID(nil, []byte{1})

func TestFungibleToken_Subtyping_Integration(t *testing.T) {
	network := NewAlphabillNetwork(t)
	tokensPartition, err := network.abNetwork.GetNodePartition(tokens.DefaultSystemIdentifier)
	require.NoError(t, err)
	homedirW1 := network.homeDir
	rpcUrl := tokensPartition.Nodes[0].AddrRPC

	symbol1 := "AB"
	// test subtyping
	typeID11 := randomFungibleTokenTypeID(t)
	typeID12 := randomFungibleTokenTypeID(t)
	typeID13 := randomFungibleTokenTypeID(t)
	typeID14 := randomFungibleTokenTypeID(t)

	//first type
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -r %s --symbol %s --type %s --subtype-clause 0x83004101F6", rpcUrl, symbol1, typeID11))
	require.Eventually(t, testpartition.BlockchainContains(tokensPartition, func(tx *types.TransactionOrder) bool {
		return bytes.Equal(tx.UnitID(), typeID11)
	}), test.WaitDuration, test.WaitTick)

	//second type
	//--parent-type without --subtype-input gives error
	execTokensCmdWithError(t, homedirW1, fmt.Sprintf("new-type fungible -r %s --symbol %s --type %s --subtype-clause %s --parent-type %s", rpcUrl, symbol1, typeID12, "ptpkh", typeID11), "missing [subtype-input]")

	//--subtype-input without --parent-type also gives error
	execTokensCmdWithError(t, homedirW1, fmt.Sprintf("new-type fungible -r %s --symbol %s --type %s --subtype-clause %s --subtype-input %s", rpcUrl, symbol1, typeID12, "ptpkh", "0x535100"), "missing [parent-type]")

	//inheriting the first one and setting subtype clause to ptpkh
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -r %s --symbol %s --type %s --subtype-clause %s --parent-type %s --subtype-input %s", rpcUrl, symbol1, typeID12, "ptpkh", typeID11, "0x"))
	require.Eventually(t, testpartition.BlockchainContains(tokensPartition, func(tx *types.TransactionOrder) bool {
		return bytes.Equal(tx.UnitID(), typeID12)
	}), test.WaitDuration, test.WaitTick)

	//third type needs to satisfy both parents, immediate parent with ptpkh, grandparent with 0x535100
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -r %s --symbol %s --type %s --subtype-clause %s --parent-type %s --subtype-input %s", rpcUrl, symbol1, typeID13, "true", typeID12, "ptpkh,empty"))
	require.Eventually(t, testpartition.BlockchainContains(tokensPartition, func(tx *types.TransactionOrder) bool {
		return bytes.Equal(tx.UnitID(), typeID13)
	}), test.WaitDuration, test.WaitTick)

	//4th type
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -r %s --symbol %s --type %s --subtype-clause %s --parent-type %s --subtype-input %s", rpcUrl, symbol1, typeID14, "true", typeID13, "empty,ptpkh,0x"))
	require.Eventually(t, testpartition.BlockchainContains(tokensPartition, func(tx *types.TransactionOrder) bool {
		return bytes.Equal(tx.UnitID(), typeID14)
	}), test.WaitDuration, test.WaitTick)
}

func TestFungibleToken_InvariantPredicate_Integration(t *testing.T) {
	network := NewAlphabillNetwork(t)
	tokensPartition, err := network.abNetwork.GetNodePartition(tokens.DefaultSystemIdentifier)
	require.NoError(t, err)
	homedirW1 := network.homeDir
	w1key := network.walletKey1
	rpcUrl := tokensPartition.Nodes[0].AddrRPC
	rpcClient := network.tokensRpcClient
	ctx := network.ctx

	symbol1 := "AB"
	typeID11 := randomFungibleTokenTypeID(t)
	typeID12 := randomFungibleTokenTypeID(t)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -r %s  --symbol %s --type %s --decimals 0 --inherit-bearer-clause %s", rpcUrl, symbol1, typeID11, predicatePtpkh))
	require.Eventually(t, testpartition.BlockchainContains(tokensPartition, func(tx *types.TransactionOrder) bool {
		return bytes.Equal(tx.UnitID(), typeID11)
	}), test.WaitDuration, test.WaitTick)

	// second type inheriting the first one and leaves inherit-bearer clause to default (true)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -r %s  --symbol %s --type %s --decimals 0 --parent-type %s --subtype-input %s", rpcUrl, symbol1, typeID12, typeID11, predicateTrue))
	require.Eventually(t, testpartition.BlockchainContains(tokensPartition, func(tx *types.TransactionOrder) bool {
		return bytes.Equal(tx.UnitID(), typeID12)
	}), test.WaitDuration, test.WaitTick)

	// mint
	execTokensCmd(t, homedirW1, fmt.Sprintf("new fungible -r %s  --type %s --amount %v --mint-input %s,%s", rpcUrl, typeID12, 1000, predicatePtpkh, predicatePtpkh))
	ensureTokenIndexed(t, ctx, rpcClient, w1key.PubKeyHash.Sha256, nil)
	testutils.VerifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -r %s", rpcUrl)), "amount='1'000'")

	// create w2
	w2, homedirW2 := testutils.CreateNewTokenWallet(t, rpcUrl)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()

	// send to w2
	execTokensCmd(t, homedirW1, fmt.Sprintf("send fungible -r %s --type %s --amount 100 --address 0x%X -k 1 --inherit-bearer-input %s,%s", rpcUrl, typeID12, w2key.PubKey, predicateTrue, predicatePtpkh))
	ensureTokenIndexed(t, ctx, rpcClient, w2key.PubKeyHash.Sha256, nil)
	testutils.VerifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list fungible -r %s", rpcUrl)), "amount='100'")
}

func TestFungibleTokens_Sending_Integration(t *testing.T) {
	network := NewAlphabillNetwork(t)
	_, err := network.abNetwork.GetNodePartition(money.DefaultSystemIdentifier)
	require.NoError(t, err)
	tokensPartition, err := network.abNetwork.GetNodePartition(tokens.DefaultSystemIdentifier)
	require.NoError(t, err)
	homedirW1 := network.homeDir
	w1key := network.walletKey1
	rpcUrl := tokensPartition.Nodes[0].AddrRPC

	typeID1 := randomFungibleTokenTypeID(t)
	// fungible token types
	symbol1 := "AB"
	execTokensCmdWithError(t, homedirW1, "new-type fungible", "required flag(s) \"symbol\" not set")
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible  --symbol %s -r %s --type %s --decimals 0", symbol1, rpcUrl, typeID1))

	// TODO AB-1448
	// testutils.VerifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list-types fungible -r %s", rpcUrl)), "symbol=AB (fungible)")

	// mint tokens
	crit := func(amount uint64) func(tx *types.TransactionOrder) bool {
		return func(tx *types.TransactionOrder) bool {
			if tx.PayloadType() == tokens.PayloadTypeMintFungibleToken {
				attrs := &tokens.MintFungibleTokenAttributes{}
				require.NoError(t, tx.UnmarshalAttributes(attrs))
				return attrs.Value == amount
			}
			return false
		}
	}
	execTokensCmd(t, homedirW1, fmt.Sprintf("new fungible  -r %s --type %s --amount 5", rpcUrl, typeID1))
	execTokensCmd(t, homedirW1, fmt.Sprintf("new fungible  -r %s --type %s --amount 9", rpcUrl, typeID1))
	require.Eventually(t, testpartition.BlockchainContains(tokensPartition, crit(5)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(tokensPartition, crit(9)), test.WaitDuration, test.WaitTick)
	testutils.VerifyStdoutEventually(t, func() *testutils.TestConsoleWriter {
		return execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -r %s", rpcUrl))
	}, "amount='5'", "amount='9'", "symbol='AB'")

	// create second wallet
	w2, homedirW2 := testutils.CreateNewTokenWallet(t, rpcUrl)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()

	// check w2 is empty
	testutils.VerifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list fungible  -r %s", rpcUrl)), "No tokens")

	// transfer tokens w1 -> w2
	execTokensCmd(t, homedirW1, fmt.Sprintf("send fungible -r %s --type %s --amount 6 --address 0x%X -k 1", rpcUrl, typeID1, w2key.PubKey)) //split (9=>6+3)
	testutils.VerifyStdoutEventually(t, func() *testutils.TestConsoleWriter {
		return execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -r %s", rpcUrl))
	}, "amount='5'", "amount='3'", "symbol='AB'")
	execTokensCmd(t, homedirW1, fmt.Sprintf("send fungible -r %s --type %s --amount 6 --address 0x%X -k 1", rpcUrl, typeID1, w2key.PubKey)) //transfer (5) + split (3=>2+1)

	//check immediately as tx must be confirmed
	testutils.VerifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list fungible -r %s", rpcUrl)), "amount='6'", "amount='5'", "amount='1'", "symbol='AB'")

	//check what is left in w1
	testutils.VerifyStdoutEventually(t, func() *testutils.TestConsoleWriter {
		return execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -r %s", rpcUrl))
	}, "amount='2'")

	// send money to w2 to create fee credits
	wallet := loadMoneyWallet(t, network.walletHomeDir, network.moneyRpcClient)
	_, err = wallet.Send(context.Background(), moneywallet.SendCmd{Receivers: []moneywallet.ReceiverData{{PubKey: w2key.PubKey, Amount: 100 * 1e8}}, WaitForConfirmation: true})
	require.NoError(t, err)
	wallet.Close()

	// add fee credit w2
	tokensWallet := loadTokensWallet(t, filepath.Join(homedirW2, "wallet"), network.moneyRpcClient, network.tokensRpcClient)
	_, err = tokensWallet.AddFeeCredit(context.Background(), fees.AddFeeCmd{Amount: 50 * 1e8, DisableLocking: true})
	require.NoError(t, err)
	tokensWallet.Shutdown()

	// transfer back w2->w1 (AB-513)
	execTokensCmd(t, homedirW2, fmt.Sprintf("send fungible -r %s --type %s --amount 6 --address 0x%X -k 1", rpcUrl, typeID1, w1key.PubKey))
	testutils.VerifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -r %s", rpcUrl)), "amount='2'", "amount='6'")
}

func TestWalletCreateFungibleTokenTypeAndTokenAndSendCmd_IntegrationTest(t *testing.T) {
	const decimals = 3
	// mint tokens
	crit := func(amount uint64) func(tx *types.TransactionOrder) bool {
		return func(tx *types.TransactionOrder) bool {
			if tx.PayloadType() == tokens.PayloadTypeMintFungibleToken {
				attrs := &tokens.MintFungibleTokenAttributes{}
				require.NoError(t, tx.UnmarshalAttributes(attrs))
				return attrs.Value == amount
			}
			return false
		}
	}

	network := NewAlphabillNetwork(t)
	tokensPart, err := network.abNetwork.GetNodePartition(tokens.DefaultSystemIdentifier)
	require.NoError(t, err)
	homedir := network.homeDir
	tokensPartition, err := network.abNetwork.GetNodePartition(tokens.DefaultSystemIdentifier)
	require.NoError(t, err)
	rpcUrl := tokensPartition.Nodes[0].AddrRPC

	w2, homedirW2 := testutils.CreateNewTokenWallet(t, rpcUrl)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()
	typeID := tokens.NewFungibleTokenTypeID(nil, []byte{0x10})
	symbol := "AB"
	name := "Long name for AB"

	// create type
	execTokensCmd(t, homedir, fmt.Sprintf("new-type fungible  --symbol %s --name %s -r %s --type %s --decimals %v", symbol, name, rpcUrl, typeID, decimals))

	// non-existing id
	nonExistingTypeId := tokens.NewFungibleTokenID(nil, []byte{0x11})

	// verify error
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %s --amount 3", rpcUrl, nonExistingTypeId), fmt.Sprintf("invalid token type id: %s", nonExistingTypeId))

	// new token creation fails
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %s --amount 0", rpcUrl, typeID), "0 is not valid amount")
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %s --amount 00.000", rpcUrl, typeID), "0 is not valid amount")
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %s --amount 00.0.00", rpcUrl, typeID), "more than one comma")
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %s --amount .00", rpcUrl, typeID), "missing integer part")
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %s --amount a.00", rpcUrl, typeID), "invalid amount string")
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %s --amount 0.0a", rpcUrl, typeID), "invalid amount string")
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %s --amount 1.1111", rpcUrl, typeID), "invalid precision")

	// out of range because decimals = 3 the value is equal to 18446744073709551615000
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %s --amount 18446744073709551615", rpcUrl, typeID), "out of range")

	// creation succeeds
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible  -r %s --type %s --amount 3", rpcUrl, typeID))
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible  -r %s --type %s --amount 1.1", rpcUrl, typeID))
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible  -r %s --type %s --amount 1.11", rpcUrl, typeID))
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible  -r %s --type %s --amount 1.111", rpcUrl, typeID))
	require.Eventually(t, testpartition.BlockchainContains(tokensPart, crit(3000)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(tokensPart, crit(1100)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(tokensPart, crit(1110)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(tokensPart, crit(1111)), test.WaitDuration, test.WaitTick)

	// mint tokens from w1 and set the owner to w2
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible  -r %s --type %s --amount 2.222 --bearer-clause ptpkh:0x%X", rpcUrl, typeID, w2key.PubKeyHash.Sha256))
	require.Eventually(t, testpartition.BlockchainContains(tokensPart, crit(2222)), test.WaitDuration, test.WaitTick)
	testutils.VerifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list fungible -r %s", rpcUrl)), "amount='2.222'")

	// test send fails
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %s --amount 2 --address 0x%X -k 1", rpcUrl, nonExistingTypeId, w2key.PubKey), fmt.Sprintf("invalid token type id: %s", nonExistingTypeId))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %s --amount 0 --address 0x%X -k 1", rpcUrl, typeID, w2key.PubKey), "0 is not valid amount")
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %s --amount 000.000 --address 0x%X -k 1", rpcUrl, typeID, w2key.PubKey), "0 is not valid amount")
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %s --amount 00.0.00 --address 0x%X -k 1", rpcUrl, typeID, w2key.PubKey), "more than one comma")
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %s --amount .00 --address 0x%X -k 1", rpcUrl, typeID, w2key.PubKey), "missing integer part")
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %s --amount a.00 --address 0x%X -k 1", rpcUrl, typeID, w2key.PubKey), "invalid amount string")
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %s --amount 1.1111 --address 0x%X -k 1", rpcUrl, typeID, w2key.PubKey), "invalid precision")
}

func TestFungibleTokens_CollectDust_Integration(t *testing.T) {
	network := NewAlphabillNetwork(t)
	homedir := network.homeDir
	tokensPartition, err := network.abNetwork.GetNodePartition(tokens.DefaultSystemIdentifier)
	require.NoError(t, err)
	rpcUrl := tokensPartition.Nodes[0].AddrRPC

	typeID1 := randomFungibleTokenTypeID(t)
	symbol1 := "AB"
	execTokensCmd(t, homedir, fmt.Sprintf("new-type fungible --symbol %s -r %s --type %s --decimals 0", symbol1, rpcUrl, typeID1))

	// TODO AB-1448
	// testutils.VerifyStdout(t, execTokensCmd(t, homedir, fmt.Sprintf("list-types fungible -r %s", rpcUrl)), "symbol=AB (fungible)")

	// mint tokens (without confirming, for speed)
	mintIterations := 10
	expectedAmounts := make([]string, 0, mintIterations)
	expectedTotal := 0
	for i := 1; i <= mintIterations; i++ {
		execTokensCmd(t, homedir, fmt.Sprintf("new fungible -r %s --type %s --amount %v -w false", rpcUrl, typeID1, i))
		expectedAmounts = append(expectedAmounts, fmt.Sprintf("amount='%v'", i))
		expectedTotal += i
	}

	// check w1 tokens
	testutils.VerifyStdoutEventuallyWithTimeout(t, func() *testutils.TestConsoleWriter {
		return execTokensCmd(t, homedir, fmt.Sprintf("list fungible -r %s", rpcUrl))
	}, 2*test.WaitDuration, 2*test.WaitTick, expectedAmounts...)

	// run DC
	execTokensCmd(t, homedir, fmt.Sprintf("collect-dust -r %s", rpcUrl))

	// verify there exists token with the expected amount
	output := execTokensCmd(t, homedir, fmt.Sprintf("list fungible -r %s", rpcUrl))
	testutils.VerifyStdout(t, output, fmt.Sprintf("amount='%v'", util.InsertSeparator(fmt.Sprint(expectedTotal), false)))
}

func TestFungibleTokens_LockUnlock_Integration(t *testing.T) {
	network := NewAlphabillNetwork(t)
	_, err := network.abNetwork.GetNodePartition(money.DefaultSystemIdentifier)
	require.NoError(t, err)
	homedirW1 := network.homeDir
	tokensPartition, err := network.abNetwork.GetNodePartition(tokens.DefaultSystemIdentifier)
	require.NoError(t, err)
	rpcUrl := tokensPartition.Nodes[0].AddrRPC

	typeID := randomFungibleTokenTypeID(t)
	symbol := "AB"
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible --symbol %s -r %s --type %s --decimals 0", symbol, rpcUrl, typeID))

	// TODO AB-1448
	// testutils.VerifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list-types fungible -r %s", rpcUrl)), "symbol=AB (fungible)")

	// mint tokens
	crit := func(amount uint64) func(tx *types.TransactionOrder) bool {
		return func(tx *types.TransactionOrder) bool {
			if tx.PayloadType() == tokens.PayloadTypeMintFungibleToken {
				attrs := &tokens.MintFungibleTokenAttributes{}
				require.NoError(t, tx.UnmarshalAttributes(attrs))
				return attrs.Value == amount
			}
			return false
		}
	}
	execTokensCmd(t, homedirW1, fmt.Sprintf("new fungible  -r %s --type %s --amount 5", rpcUrl, typeID))
	require.Eventually(t, testpartition.BlockchainContains(tokensPartition, crit(5)), test.WaitDuration, test.WaitTick)

	// get minted token id
	var tokenID string
	out := execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -r %s", rpcUrl))
	for _, l := range out.Lines {
		id := extractID(l)
		if id != "" {
			tokenID = id
			break
		}
	}

	// lock token
	execTokensCmd(t, homedirW1, fmt.Sprintf("lock -r %s --token-identifier %s -k 1", rpcUrl, tokenID))
	testutils.VerifyStdoutEventually(t, func() *testutils.TestConsoleWriter {
		return execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -r %s", rpcUrl))
	}, "locked='manually locked by user'")

	// unlock token
	execTokensCmd(t, homedirW1, fmt.Sprintf("unlock -r %s --token-identifier %s -k 1", rpcUrl, tokenID))
	testutils.VerifyStdoutEventually(t, func() *testutils.TestConsoleWriter {
		return execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -r %s", rpcUrl))
	}, "locked=''")
}

func extractID(input string) string {
	re := regexp.MustCompile(`ID='([^']+)'`)
	match := re.FindStringSubmatch(input)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

type AlphabillNetwork struct {
	abNetwork *testpartition.AlphabillNetwork

	moneyRpcClient  *rpc.Client
	tokensRpcClient *rpc.TokensClient

	homeDir       string
	walletHomeDir string
	walletKey1    *account.AccountKey
	walletKey2    *account.AccountKey
	ctx           context.Context
}

// starts money and tokens partition
// sends initial bill to money wallet
// creates fee credit on money wallet and token wallet
func NewAlphabillNetwork(t *testing.T) *AlphabillNetwork {
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	observe := testobserve.NewFactory(t)
	log := observe.DefaultLogger()

	homedirW1 := t.TempDir()
	walletDir := filepath.Join(homedirW1, "wallet")
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	defer am.Close()
	require.NoError(t, am.CreateKeys(""))
	w1key, err := am.GetAccountKey(0)
	require.NoError(t, err)
	_, _, err = am.AddAccount()
	require.NoError(t, err)
	w1key2, err := am.GetAccountKey(1)
	require.NoError(t, err)

	genesisConfig := &testutil.MoneyGenesisConfig{
		InitialBillID:      defaultInitialBillID,
		InitialBillValue:   1e18,
		InitialBillOwner:   templates.NewP2pkh256BytesFromKey(w1key.PubKey),
		DCMoneySupplyValue: 10000,
	}
	moneyPartition := testutils.CreateMoneyPartition(t, genesisConfig, 1)
	tokensPartition := testutils.CreateTokensPartition(t)
	abNet := testutils.StartAlphabill(t, []*testpartition.NodePartition{moneyPartition, tokensPartition})

	testutils.StartRpcServers(t, moneyPartition)
	moneyRpcClient, err := rpc.DialContext(ctx, args.BuildRpcUrl(moneyPartition.Nodes[0].AddrRPC))
	require.NoError(t, err)

	testutils.StartRpcServers(t, tokensPartition)
	rpcClient, err := rpc.DialContext(ctx, args.BuildRpcUrl(tokensPartition.Nodes[0].AddrRPC))
	require.NoError(t, err)
	tokensRpcClient := rpc.NewTokensClient(rpcClient)

	feeManagerDB, err := fees.NewFeeManagerDB(walletDir)
	require.NoError(t, err)
	defer feeManagerDB.Close()

	moneyWallet, err := moneywallet.LoadExistingWallet(am, feeManagerDB, moneyRpcClient, log)
	require.NoError(t, err)
	defer moneyWallet.Close()

	tokenFeeManager := fees.NewFeeManager(am, feeManagerDB, money.DefaultSystemIdentifier, moneyRpcClient, moneywallet.FeeCreditRecordIDFormPublicKey, tokens.DefaultSystemIdentifier, tokensRpcClient, tokenswallet.FeeCreditRecordIDFromPublicKey, log)
	defer tokenFeeManager.Close()

	tokensWallet, err := tokenswallet.New(tokens.DefaultSystemIdentifier, tokensRpcClient, am, true, tokenFeeManager, log)
	require.NoError(t, err)
	require.NotNil(t, tokensWallet)
	defer tokensWallet.Shutdown()

	// create fees on money partition
	_, err = moneyWallet.AddFeeCredit(ctx, fees.AddFeeCmd{Amount: 1000})
	require.NoError(t, err)

	// create fees on token partition
	_, err = tokensWallet.AddFeeCredit(ctx, fees.AddFeeCmd{Amount: 1000})
	require.NoError(t, err)

	return &AlphabillNetwork{
		abNetwork:       abNet,
		moneyRpcClient:  moneyRpcClient,
		tokensRpcClient: tokensRpcClient,
		homeDir:         homedirW1,
		walletHomeDir:   walletDir,
		walletKey1:      w1key,
		walletKey2:      w1key2,
		ctx:             ctx,
	}
}

func loadMoneyWallet(t *testing.T, walletDir string, moneyRpcClient *rpc.Client) *moneywallet.Wallet {
	am, err := account.NewManager(walletDir, "", false)
	require.NoError(t, err)
	t.Cleanup(am.Close)

	feeManagerDB, err := fees.NewFeeManagerDB(walletDir)
	require.NoError(t, err)
	t.Cleanup(func() { feeManagerDB.Close() })

	moneyWallet, err := moneywallet.LoadExistingWallet(am, feeManagerDB, moneyRpcClient, testobserve.Default(t).Logger())
	require.NoError(t, err)
	t.Cleanup(moneyWallet.Close)

	return moneyWallet
}

func loadTokensWallet(t *testing.T, walletDir string, moneyRpcClient *rpc.Client, tokensRpcClient *rpc.TokensClient) *tokenswallet.Wallet {
	am, err := account.NewManager(walletDir, "", false)
	require.NoError(t, err)
	t.Cleanup(am.Close)

	feeManagerDB, err := fees.NewFeeManagerDB(walletDir)
	require.NoError(t, err)
	t.Cleanup(func() { feeManagerDB.Close() })

	tokenFeeManager := fees.NewFeeManager(am, feeManagerDB, money.DefaultSystemIdentifier, moneyRpcClient, moneywallet.FeeCreditRecordIDFormPublicKey, tokens.DefaultSystemIdentifier, tokensRpcClient, tokenswallet.FeeCreditRecordIDFromPublicKey, testobserve.Default(t).Logger())
	t.Cleanup(tokenFeeManager.Close)

	tokensWallet, err := tokenswallet.New(tokens.DefaultSystemIdentifier, tokensRpcClient, am, true, tokenFeeManager, testobserve.Default(t).Logger())
	require.NoError(t, err)
	require.NotNil(t, tokensWallet)
	t.Cleanup(tokensWallet.Shutdown)

	return tokensWallet
}
