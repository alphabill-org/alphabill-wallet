//go:build !nodocker

package wallet

import (
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/hash"
	tokenid "github.com/alphabill-org/alphabill-go-base/testutils/tokens"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	"github.com/alphabill-org/alphabill-wallet/util"
)

const (
	predicateTrue  = "true"
	predicatePtpkh = "ptpkh"
)

func TestFungibleToken_Subtyping_Integration(t *testing.T) {
	wallets, abNet := testutils.SetupNetworkWithWallets(t, testutils.WithTokensNode(t))

	symbol1 := "AB"
	// test subtyping
	typeID11 := tokenid.NewFungibleTokenTypeID(t)
	typeID12 := tokenid.NewFungibleTokenTypeID(t)
	typeID13 := tokenid.NewFungibleTokenTypeID(t)
	typeID14 := tokenid.NewFungibleTokenTypeID(t)

	addFeeCredit(t, wallets[0].Homedir, 100, "tokens", abNet.TokensRpcUrl, abNet.MoneyRpcUrl)

	tokenCmd := newWalletCmdExecutor("token", "--rpc-url", abNet.TokensRpcUrl).WithHome(wallets[0].Homedir)

	//first type
	tokenCmd.Exec(t,
		"new-type", "fungible",
		"--symbol", symbol1,
		"--type", typeID11.String(),
		"--subtype-clause", "0x83004101F6")

	// TODO: AB-1448
	// testutils.VerifyStdoutEventually(t, tokenCmd.ExecFunc(t, "list-types", "fungible"), typeID11.String())

	//second type
	//--parent-type without --subtype-input gives error
	tokenCmd.ExecWithError(t, "missing [subtype-input]",
		"new-type", "fungible",
		"--symbol", symbol1,
		"--type", typeID12.String(),
		"--subtype-clause", "ptpkh",
		"--parent-type", typeID11.String())

	//--subtype-input without --parent-type also gives error
	tokenCmd.ExecWithError(t, "missing [parent-type]",
		"new-type", "fungible",
		"--symbol", symbol1,
		"--type", typeID12.String(),
		"--subtype-clause", "ptpkh",
		"--subtype-input", "0x535100")

	//inheriting the first one and setting subtype clause to ptpkh
	tokenCmd.Exec(t, "new-type", "fungible",
		"--symbol", symbol1,
		"--type", typeID12.String(),
		"--subtype-clause", "ptpkh",
		"--parent-type", typeID11.String(),
		"--subtype-input", "0x")
	// TODO: AB-1448
	// testutils.VerifyStdoutEventually(t, tokenCmd.ExecFunc(t, "list-types", "fungible"), typeID12.String())

	//third type needs to satisfy both parents, immediate parent with ptpkh, grandparent with 0x535100
	tokenCmd.Exec(t, "new-type", "fungible",
		"--symbol", symbol1,
		"--type", typeID13.String(),
		"--subtype-clause", "true",
		"--parent-type", typeID12.String(),
		"--subtype-input", "ptpkh,empty")
	// TODO: AB-1448
	// testutils.VerifyStdoutEventually(t, tokenCmd.ExecFunc(t, "list-types", "fungible"), typeID13.String())

	//4th type
	tokenCmd.Exec(t, "new-type", "fungible",
		"--symbol", symbol1,
		"--type", typeID14.String(),
		"--subtype-clause", "true",
		"--parent-type", typeID13.String(),
		"--subtype-input", "empty,ptpkh,0x")
	// TODO: AB-1448
	// testutils.VerifyStdoutEventually(t, tokenCmd.ExecFunc(t, "list-types", "fungible"), typeID14.String())
}

func TestFungibleToken_InvariantPredicate_Integration(t *testing.T) {
	wallets, abNet := testutils.SetupNetworkWithWallets(t, testutils.WithTokensNode(t))

	symbol1 := "AB"
	typeID11 := tokenid.NewFungibleTokenTypeID(t)
	typeID12 := tokenid.NewFungibleTokenTypeID(t)

	addFeeCredit(t, wallets[0].Homedir, 100, "tokens", abNet.TokensRpcUrl, abNet.MoneyRpcUrl)

	tokenCmd := newWalletCmdExecutor("token", "--rpc-url", abNet.TokensRpcUrl).WithHome(wallets[0].Homedir)
	tokenCmd.Exec(t,
		"new-type", "fungible",
		"--symbol", symbol1,
		"--type", typeID11.String(),
		"--decimals", "0",
		"--inherit-bearer-clause", predicatePtpkh)
	// TODO: AB-1448
	// testutils.VerifyStdoutEventually(t, tokenCmd.ExecFunc(t, "list-types", "fungible"), typeID11.String())

	// second type inheriting the first one and leaves inherit-bearer clause to default (true)
	tokenCmd.Exec(t,
		"new-type", "fungible",
		"--symbol", symbol1,
		"--type", typeID12.String(),
		"--decimals", "0",
		"--parent-type", typeID11.String(),
		"--subtype-input", predicateTrue)
	// TODO: AB-1448
	// testutils.VerifyStdoutEventually(t, tokenCmd.ExecFunc(t, "list-types", "fungible"), typeID12.String())

	// mint
	tokenCmd.Exec(t,
		"new", "fungible",
		"--type", typeID12.String(),
		"--amount", "1000",
		"--mint-input", predicatePtpkh+","+predicatePtpkh)
	testutils.VerifyStdoutEventually(t, tokenCmd.ExecFunc(t, "list", "fungible"), "amount='1'000'")

	// send to w2
	tokenCmd.Exec(t,
		"send", "fungible",
		"--type", typeID12.String(),
		"--amount", "100",
		"--address", fmt.Sprintf("0x%X", wallets[1].PubKeys[0]),
		"--key", "1",
		"--inherit-bearer-input", predicateTrue+","+predicatePtpkh)
	testutils.VerifyStdoutEventually(t, tokenCmd.WithHome(wallets[1].Homedir).ExecFunc(t, "list", "fungible"), "amount='100'")
}

func TestFungibleTokens_Sending_Integration(t *testing.T) {
	wallets, abNet := testutils.SetupNetworkWithWallets(t, testutils.WithTokensNode(t))

	typeID1 := tokenid.NewFungibleTokenTypeID(t)
	// fungible token types
	symbol1 := "AB"

	walletCmd := newWalletCmdExecutor().WithHome(wallets[0].Homedir)
	tokensCmd := walletCmd.WithPrefixArgs("token", "--rpc-url", abNet.TokensRpcUrl)

	addFeeCredit(t, wallets[0].Homedir, 100, "money", abNet.MoneyRpcUrl, abNet.MoneyRpcUrl)
	addFeeCredit(t, wallets[0].Homedir, 100, "tokens", abNet.TokensRpcUrl, abNet.MoneyRpcUrl)

	tokensCmd.ExecWithError(t, "required flag(s) \"symbol\" not set", "new-type", "fungible")
	tokensCmd.Exec(t,
		"new-type", "fungible",
		"--symbol", symbol1,
		"--type", typeID1.String(),
		"--decimals", "0")

	// TODO AB-1448
	// testutils.VerifyStdout(t, tokensCmd.Exec(t, homedirW1, fmt.Sprintf("list-types fungible -r %s", rpcUrl)), "symbol=AB (fungible)")

	// mint tokens
	tokensCmd.Exec(t, "new", "fungible", "--type", typeID1.String(), "--amount", "5")
	tokensCmd.Exec(t, "new", "fungible", "--type", typeID1.String(), "--amount", "9")
	testutils.VerifyStdoutEventually(t, tokensCmd.ExecFunc(t, "list", "fungible"),
		"amount='5'", "amount='9'", "symbol='AB'")

	// check w2 is empty
	testutils.VerifyStdout(t, tokensCmd.WithHome(wallets[1].Homedir).Exec(t, "list", "fungible"), "No tokens")

	// transfer tokens w1 -> w2
	tokensCmd.Exec(t,
		"send", "fungible",
		"--type", typeID1.String(),
		"--amount", "6",
		"--address", fmt.Sprintf("0x%X", wallets[1].PubKeys[0]),
		"-k", "1") //split (9=>6+3)

	testutils.VerifyStdoutEventually(t, tokensCmd.ExecFunc(t, "list", "fungible"),
		"amount='5'", "amount='3'", "symbol='AB'")

	tokensCmd.Exec(t, "send", "fungible",
		"--type", typeID1.String(),
		"--amount", "6",
		"--address", fmt.Sprintf("0x%X", wallets[1].PubKeys[0]),
		"-k", "1") //transfer (5) + split (3=>2+1)

	//check immediately as tx must be confirmed
	testutils.VerifyStdout(t, tokensCmd.WithHome(wallets[1].Homedir).Exec(t, "list", "fungible"), "amount='6'", "amount='5'", "amount='1'", "symbol='AB'")

	//check what is left in w1
	testutils.VerifyStdoutEventually(t, tokensCmd.ExecFunc(t, "list", "fungible"), "amount='2'")

	// send money to w2k1 to create fee credits
	walletCmd.Exec(t, "send",
		"--amount", "100",
		"--address", fmt.Sprintf("0x%X", wallets[1].PubKeys[0]),
		"--rpc-url", abNet.MoneyRpcUrl)

	// add fee credit to w2k1
	addFeeCredit(t, wallets[1].Homedir, 50, "tokens", abNet.TokensRpcUrl, abNet.MoneyRpcUrl)

	// transfer back w2->w1 (AB-513)
	tokensCmd.WithHome(wallets[1].Homedir).Exec(t,
		"send", "fungible",
		"--type", typeID1.String(),
		"--amount", "6",
		"--address", fmt.Sprintf("0x%X", wallets[0].PubKeys[0]),
		"-k", "1")
	testutils.VerifyStdout(t, tokensCmd.Exec(t, "list", "fungible"), "amount='2'", "amount='6'")

	// transfer from w1 key1 to key2 (AB-1750)
	tokensCmd.Exec(t,
		"send", "fungible",
		"--type", typeID1.String(),
		"--amount", "1",
		"--address", fmt.Sprintf("0x%X", wallets[0].PubKeys[1]),
		"-k", "1")
	// verify list tokens displays tokens for both keys
	testutils.VerifyStdout(t, tokensCmd.Exec(t, "list", "fungible"), "Tokens owned by account #1", "Tokens owned by account #2")
}

func TestWalletCreateFungibleTokenTypeAndTokenAndSendCmd_IntegrationTest(t *testing.T) {
	// mint tokens
	wallets, abNet := testutils.SetupNetworkWithWallets(t, testutils.WithTokensNode(t))

	addFeeCredit(t, wallets[0].Homedir, 100, "money", abNet.MoneyRpcUrl, abNet.MoneyRpcUrl)
	addFeeCredit(t, wallets[0].Homedir, 100, "tokens", abNet.TokensRpcUrl, abNet.MoneyRpcUrl)

	typeID := tokenid.NewFungibleTokenTypeID(t)
	symbol := "AB"
	name := "Long name for AB"

	walletCmd := newWalletCmdExecutor().WithHome(wallets[0].Homedir)
	tokensCmd := walletCmd.WithPrefixArgs("token", "--rpc-url", abNet.TokensRpcUrl)

	// create type
	tokensCmd.Exec(t, "new-type", "fungible",
		"--symbol", symbol,
		"--name", name,
		"--type", typeID.String(),
		"--decimals", "3")

	// non-existing id
	nonExistingTypeId := tokenid.NewFungibleTokenID(t)

	newFungibleCmd := tokensCmd.WithPrefixArgs("new", "fungible", "--type", typeID.String())

	// new token creation fails
	newFungibleCmd.ExecWithError(t, "invalid fungible token type id: expected type 0X1, got 0X3",
		"--amount", "3", "--type", nonExistingTypeId.String())
	newFungibleCmd.ExecWithError(t, "0 is not valid amount", "--amount", "0")
	newFungibleCmd.ExecWithError(t, "0 is not valid amount", "--amount", "00.000")
	newFungibleCmd.ExecWithError(t, "more than one comma", "--amount", "00.0.00")
	newFungibleCmd.ExecWithError(t, "missing integer part", "--amount", ".00")
	newFungibleCmd.ExecWithError(t, "invalid amount string", "--amount", "a.00")
	newFungibleCmd.ExecWithError(t, "invalid amount string", "--amount", "0.0a")
	newFungibleCmd.ExecWithError(t, "invalid precision", "--amount", "1.1111")
	// out of range because decimals = 3 the value is equal to 18446744073709551615000
	newFungibleCmd.ExecWithError(t, "out of range", "--amount", "18446744073709551615")

	// creation succeeds
	newFungibleCmd.Exec(t, "--amount", "3")
	newFungibleCmd.Exec(t, "--amount", "1.1")
	newFungibleCmd.Exec(t, "--amount", "1.11")
	newFungibleCmd.Exec(t, "--amount", "1.111")

	testutils.VerifyStdoutEventually(t, tokensCmd.ExecFunc(t, "list", "fungible"),
		"amount='3.000'", "amount='1.100'", "amount='1.110'", "amount='1.111'")

	// mint tokens from w1 and set the owner to w2
	newFungibleCmd.Exec(t, "--amount", "2.222", "--bearer-clause", fmt.Sprintf("ptpkh:0x%X", hash.Sum256(wallets[1].PubKeys[0])))

	testutils.VerifyStdoutEventually(t, tokensCmd.WithHome(wallets[1].Homedir).ExecFunc(t, "list", "fungible"), "amount='2.222'")

	sendFungibleCmd := tokensCmd.WithPrefixArgs(
		"send", "fungible",
		"-k", "1",
		"--type", typeID.String(),
		"--address", fmt.Sprintf("0x%X", wallets[1].PubKeys[0]))

	// test send fails
	sendFungibleCmd.ExecWithError(t, "invalid fungible token type id: expected type 0X1, got 0X3", "--amount", "2", "--type", nonExistingTypeId.String())
	sendFungibleCmd.ExecWithError(t, "0 is not valid amount", "--amount", "0")
	sendFungibleCmd.ExecWithError(t, "0 is not valid amount", "--amount", "000.000")
	sendFungibleCmd.ExecWithError(t, "more than one comma", "--amount", "00.0.00")
	sendFungibleCmd.ExecWithError(t, "missing integer part", "--amount", ".00")
	sendFungibleCmd.ExecWithError(t, "invalid amount string", "--amount", "a.00")
	sendFungibleCmd.ExecWithError(t, "invalid precision", "--amount", "1.1111")
}

func TestFungibleTokens_CollectDust_Integration(t *testing.T) {
	wallets, abNet := testutils.SetupNetworkWithWallets(t, testutils.WithTokensNode(t))

	addFeeCredit(t, wallets[0].Homedir, 100, "money", abNet.MoneyRpcUrl, abNet.MoneyRpcUrl)
	addFeeCredit(t, wallets[0].Homedir, 100, "tokens", abNet.TokensRpcUrl, abNet.MoneyRpcUrl)

	typeID1 := tokenid.NewFungibleTokenTypeID(t)
	symbol1 := "AB"

	walletCmd := newWalletCmdExecutor().WithHome(wallets[0].Homedir)
	tokensCmd := walletCmd.WithPrefixArgs("token", "--rpc-url", abNet.TokensRpcUrl)

	tokensCmd.Exec(t, "new-type", "fungible", "--symbol", symbol1, "--type", typeID1.String(), "--decimals", "0")

	// TODO AB-1448
	// testutils.VerifyStdout(t, tokensCmd.Exec(t, homedir, fmt.Sprintf("list-types fungible -r %s", rpcUrl)), "symbol=AB (fungible)")

	// mint tokens (without confirming, for speed)
	mintIterations := 10
	expectedAmounts := make([]string, 0, mintIterations)
	expectedTotal := 0
	for i := 1; i <= mintIterations; i++ {
		tokensCmd.Exec(t, "new", "fungible", "--type", typeID1.String(), "--amount", strconv.Itoa(i), "-w", "false")
		expectedAmounts = append(expectedAmounts, fmt.Sprintf("amount='%v'", i))
		expectedTotal += i
	}

	// check w1 tokens
	testutils.VerifyStdoutEventuallyWithTimeout(t, tokensCmd.ExecFunc(t, "list", "fungible"),
		2*testutils.WaitDuration, 2*testutils.WaitTick, expectedAmounts...)

	// run DC
	output := tokensCmd.Exec(t, "collect-dust", "-k", "1", "--type", typeID1.String())
	testutils.VerifyStdout(t, output, "Paid 0.000'000'32 fees for dust collection on Account number 1.")

	// verify there exists token with the expected amount
	output = tokensCmd.Exec(t, "list", "fungible")
	testutils.VerifyStdout(t, output, fmt.Sprintf("amount='%v'", util.InsertSeparator(fmt.Sprint(expectedTotal), false)))
}

func TestFungibleTokens_LockUnlock_Integration(t *testing.T) {
	wallets, abNet := testutils.SetupNetworkWithWallets(t, testutils.WithTokensNode(t))

	addFeeCredit(t, wallets[0].Homedir, 100, "money", abNet.MoneyRpcUrl, abNet.MoneyRpcUrl)
	addFeeCredit(t, wallets[0].Homedir, 100, "tokens", abNet.TokensRpcUrl, abNet.MoneyRpcUrl)

	typeID := tokenid.NewFungibleTokenTypeID(t)
	symbol := "AB"

	walletCmd := newWalletCmdExecutor().WithHome(wallets[0].Homedir)
	tokensCmd := walletCmd.WithPrefixArgs("token", "--rpc-url", abNet.TokensRpcUrl)

	tokensCmd.Exec(t, "new-type", "fungible", "--symbol", symbol, "--type", typeID.String(), "--decimals", "0")

	// TODO AB-1448
	// testutils.VerifyStdout(t, tokensCmd.Exec(t, homedirW1, fmt.Sprintf("list-types fungible -r %s", rpcUrl)), "symbol=AB (fungible)")

	// mint tokens
	tokensCmd.Exec(t, "new", "fungible", "--type", typeID.String(), "--amount", "5")
	testutils.VerifyStdoutEventually(t, tokensCmd.ExecFunc(t, "list", "fungible"), "amount='5'")

	// get minted token id
	var tokenID string
	out := tokensCmd.Exec(t, "list", "fungible")
	for _, l := range out.Lines {
		id := extractID(l)
		if id != "" {
			tokenID = id
			break
		}
	}

	// lock token
	tokensCmd.Exec(t, "lock", "--token-identifier", tokenID, "-k", "1")
	testutils.VerifyStdoutEventually(t, tokensCmd.ExecFunc(t, "list", "fungible"), "lockStatus='4 (manually locked by user)'")

	// unlock token
	tokensCmd.Exec(t, "unlock", "--token-identifier", tokenID, "-k", "1")
	testutils.VerifyStdoutEventually(t, tokensCmd.ExecFunc(t, "list", "fungible"), "lockStatus='0 (unlocked)'")
}

func addFeeCredit(t *testing.T, home string, amount uint64, partition, partitionRpcUrl, moneyRpcUrl string) {
	amountStr := strconv.FormatUint(amount, 10)
	feesCmd := newWalletCmdExecutor(
		"--rpc-url", moneyRpcUrl,
		"--partition", partition,
		"--partition-rpc-url", partitionRpcUrl).WithHome(home)

	stdout := feesCmd.Exec(t, "fees", "add", "--amount", amountStr)
	require.Equal(t, "Successfully created "+amountStr+" fee credits on "+partition+" partition.", stdout.Lines[0])
}

func extractID(input string) string {
	re := regexp.MustCompile(`ID='([^']+)'`)
	match := re.FindStringSubmatch(input)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}
