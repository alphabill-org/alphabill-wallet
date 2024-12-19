//go:build !nodocker

package wallet

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	tokenid "github.com/alphabill-org/alphabill-go-base/testutils/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	"github.com/alphabill-org/alphabill-wallet/client"
)

func TestNFTs_Integration(t *testing.T) {
	wallets, abNet := testutils.SetupNetworkWithWallets(t, testutils.WithTokensNode(t))

	addFeeCredit(t, wallets[0].Homedir, 100, "money", abNet.MoneyRpcUrl, abNet.MoneyRpcUrl)
	addFeeCredit(t, wallets[0].Homedir, 100, "tokens", abNet.TokensRpcUrl, abNet.MoneyRpcUrl)

	walletCmd := newWalletCmdExecutor().WithHome(wallets[0].Homedir)

	// send money to w1k2 to create fee credits
	walletCmd.Exec(t, "send",
		"--rpc-url", abNet.MoneyRpcUrl,
		"--amount", "100",
		"--address", fmt.Sprintf("0x%X", wallets[0].PubKeys[1]))

	// create fee credit for w1k2
	stdout := walletCmd.Exec(t, "fees", "add",
		"--rpc-url", abNet.MoneyRpcUrl,
		"--partition", "tokens",
		"--partition-rpc-url", abNet.TokensRpcUrl,
		"--key", "2",
		"--amount", "50")
	require.Equal(t, "Successfully created 50 fee credits on tokens partition.", stdout.Lines[0])

	// non-fungible token types
	typeID := tokenid.NewNonFungibleTokenTypeID(t)
	typeID2 := tokenid.NewNonFungibleTokenTypeID(t)
	symbol := "ABNFT"

	tokensCmd := walletCmd.WithPrefixArgs("token", "--rpc-url", abNet.TokensRpcUrl)
	tokensCmd.ExecWithError(t, "required flag(s) \"symbol\" not set", "new-type", "non-fungible")
	tokensCmd.Exec(t, "new-type", "non-fungible", "--key", "2", "--symbol", symbol, "--type", typeID.String(), "--subtype-clause", "ptpkh")
	tokensCmd.Exec(t, "new-type", "non-fungible",
		"--key", "2",
		"--symbol", symbol+"2",
		"--type", typeID2.String(),
		"--parent-type", typeID.String(),
		"--subtype-input", "ptpkh")

	// mint NFT
	stdout = tokensCmd.Exec(t, "new", "non-fungible", "--key", "2", "--type", typeID.String())
	nftID := extractTokenID(t, stdout.Lines[0])

	testutils.VerifyStdoutEventually(t, tokensCmd.ExecFunc(t, "list", "non-fungible", "--key", "2"),
		fmt.Sprintf("ID='%s'", nftID))

	// transfer NFT from w1k2 to w2k1
	tokensCmd.Exec(t, "send", "non-fungible", "--key", "2", "--token-identifier", nftID.String(), "--address", fmt.Sprintf("0x%X", wallets[1].PubKeys[0]))

	//check that w2k1 has the nft
	testutils.VerifyStdoutEventually(t, tokensCmd.WithHome(wallets[1].Homedir).ExecFunc(t, "list", "non-fungible"),
		fmt.Sprintf("ID='%s'", nftID))

	//check that w1k2 has no tokens left
	testutils.VerifyStdout(t, tokensCmd.Exec(t, "list", "non-fungible", "--key", "2"), "No tokens")

	// TODO AB-1448
	// list token types
	//testutils.VerifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list-types -r %s", rpcUrl)), "symbol=ABNFT (nft)")
	//testutils.VerifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list-types non-fungible -r %s", rpcUrl)), "symbol=ABNFT (nft)")

	// send money to w2k1 to create fee credits
	walletCmd.Exec(t, "send",
		"--rpc-url", abNet.MoneyRpcUrl,
		"--amount", "100",
		"--address", fmt.Sprintf("0x%X", wallets[1].PubKeys[0]))

	// create fee credit for w2k1
	stdout = walletCmd.WithHome(wallets[1].Homedir).Exec(t, "fees", "add",
		"--rpc-url", abNet.MoneyRpcUrl,
		"--partition", "tokens",
		"--partition-rpc-url", abNet.TokensRpcUrl,
		"--amount", "50")
	require.Equal(t, "Successfully created 50 fee credits on tokens partition.", stdout.Lines[0])

	// transfer back
	tokensCmd.WithHome(wallets[1].Homedir).Exec(t, "send", "non-fungible",
		"--key", "1",
		"--token-identifier", nftID.String(),
		"--address", fmt.Sprintf("0x%X", wallets[0].PubKeys[1]))

	// check that wallet 1 key 2 has the nft
	testutils.VerifyStdoutEventuallyWithTimeout(t,
		tokensCmd.ExecFunc(t, "list", "non-fungible", "--key", "2"),
		2*testutils.WaitDuration, 2*testutils.WaitTick,
		fmt.Sprintf("ID='%s'", nftID))

	testutils.VerifyStdout(t, tokensCmd.WithHome(wallets[1].Homedir).Exec(t, "list", "non-fungible"), "No tokens")

	// mint nft from w1 and set the owner to w2
	stdout = tokensCmd.Exec(t, "new", "non-fungible",
		"--key", "2",
		"--type", typeID.String(),
		"--bearer-clause", fmt.Sprintf("ptpkh:0x%X", hash.Sum256(wallets[1].PubKeys[0])))
	nftID2 := extractTokenID(t, stdout.Lines[0])
	testutils.VerifyStdout(t, tokensCmd.WithHome(wallets[1].Homedir).Exec(t, "list", "non-fungible"), fmt.Sprintf("ID='%s'", nftID2))
}

func TestNFTs_CustomBearer_Integration(t *testing.T) {
	wallets, abNet := testutils.SetupNetworkWithWallets(t, testutils.WithTokensNode(t))

	addFeeCredit(t, wallets[0].Homedir, 100, "money", abNet.MoneyRpcUrl, abNet.MoneyRpcUrl)
	addFeeCredit(t, wallets[0].Homedir, 100, "tokens", abNet.TokensRpcUrl, abNet.MoneyRpcUrl)

	walletCmd := newWalletCmdExecutor().WithHome(wallets[0].Homedir)

	// non-fungible token types
	typeID := tokenid.NewNonFungibleTokenTypeID(t)
	symbol := "ABNFT"

	tokensCmd := walletCmd.WithPrefixArgs("token", "--rpc-url", abNet.TokensRpcUrl)
	tokensCmd.Exec(t, "new-type", "non-fungible", "--key", "1", "--symbol", symbol, "--type", typeID.String(), "--subtype-clause", "false")

	// mint NFT and set bearer to "always true"
	stdout := tokensCmd.Exec(t, "new", "non-fungible", "--key", "1", "--type", typeID.String(), "--bearer-clause", "true")
	nftID := extractTokenID(t, stdout.Lines[0])

	// check token exists via RPC because "always true" does not belong to any wallet
	ctx := context.Background()
	client, err := client.NewTokensPartitionClient(ctx, abNet.TokensRpcUrl)
	require.NoError(t, err)
	t.Cleanup(client.Close)
	nft, err := client.GetNonFungibleToken(ctx, nftID)
	require.NoError(t, err)
	require.NotNil(t, nft)
	require.Equal(t, nftID, nft.ID)
	require.Equal(t, typeID, nft.TypeID)
	require.EqualValues(t, templates.AlwaysTrueBytes(), nft.OwnerPredicate)

	// create temp file and write 'CBOR NULL' byte into it:
	tmpfile, err := os.CreateTemp(t.TempDir(), "test")
	require.NoError(t, err)
	_, err = tmpfile.Write([]byte{0xf6})
	require.NoError(t, err)

	// transfer NFT to w1k2 using custom --bearer-clause-input flag with a proof to "always true" predicate
	tokensCmd.Exec(t, "send", "non-fungible", "--key", "1", "--token-identifier", nftID.String(), "--address", fmt.Sprintf("0x%X", wallets[0].PubKeys[1]),
		"--bearer-clause-input", "@"+tmpfile.Name())

	//check that w1k2 has the nft
	testutils.VerifyStdoutEventually(t, tokensCmd.WithHome(wallets[0].Homedir).ExecFunc(t, "list", "non-fungible", "--key", "2"),
		fmt.Sprintf("ID='%s'", nftID))
}

func TestNFTDataUpdateCmd_Integration(t *testing.T) {
	wallets, abNet := testutils.SetupNetworkWithWallets(t, testutils.WithTokensNode(t))

	typeID := tokenid.NewNonFungibleTokenTypeID(t)
	symbol := "ABNFT"

	// create type
	walletCmd := newWalletCmdExecutor().WithHome(wallets[0].Homedir)
	tokensCmd := walletCmd.WithPrefixArgs("token", "--rpc-url", abNet.TokensRpcUrl)

	addFeeCredit(t, wallets[0].Homedir, 100, "tokens", abNet.TokensRpcUrl, abNet.MoneyRpcUrl)

	tokensCmd.Exec(t, "new-type", "non-fungible", "--symbol", symbol, "--type", typeID.String())

	// create non-fungible token from using data-file
	data := make([]byte, 1024)
	n, err := rand.Read(data)
	require.NoError(t, err)
	require.EqualValues(t, n, len(data))
	tmpfile, err := os.CreateTemp(t.TempDir(), "test")
	require.NoError(t, err)
	_, err = tmpfile.Write(data)
	require.NoError(t, err)
	stdout := tokensCmd.Exec(t, "new", "non-fungible", "--type", typeID.String(), "--data-file", tmpfile.Name())
	nftID := extractTokenID(t, stdout.Lines[0])
	testutils.VerifyStdoutEventually(t, tokensCmd.ExecFunc(t, "list", "non-fungible", "--with-token-data"),
		fmt.Sprintf("ID='%s'", nftID), fmt.Sprintf("data='%X'", data))

	// generate new data
	data2 := make([]byte, 1024)
	n, err = rand.Read(data2)
	require.NoError(t, err)
	require.EqualValues(t, n, len(data2))
	require.False(t, bytes.Equal(data, data2))
	tmpfile, err = os.CreateTemp(t.TempDir(), "test")
	require.NoError(t, err)
	_, err = tmpfile.Write(data2)
	require.NoError(t, err)

	// update data, assumes default [--data-update-input true,true]
	tokensCmd.Exec(t, "update", "--token-identifier", nftID.String(), "--data-file", tmpfile.Name())

	// check that data was updated on the rpc node
	testutils.VerifyStdoutEventually(t, tokensCmd.ExecFunc(t, "list", "non-fungible", "--with-token-data"),
		fmt.Sprintf("ID='%s'", nftID), fmt.Sprintf("data='%X'", data2))

	// create non-updatable nft
	stdout = tokensCmd.Exec(t, "new", "non-fungible", "--type", typeID.String(), "--data", "01", "--data-update-clause", "false")
	nftID2 := extractTokenID(t, stdout.Lines[0])

	testutils.VerifyStdoutEventually(t, tokensCmd.ExecFunc(t, "list", "non-fungible", "--with-token-data"),
		fmt.Sprintf("ID='%s'", nftID2), "data='01'")

	// try to update and Observe failure
	// TODO: a very slow way (10s) to verify that transaction failed, can we do better without inspecting node internals?
	// or configure shorter confirmation timeout (AB-868)
	tokensCmd.ExecWithError(t, "transaction(s) failed",
		"update", "--token-identifier", nftID2.String(), "--data", "02", "--data-update-input", "false", "--inherit-data-update-input", "true")
}

func TestNFT_InvariantPredicate_Integration(t *testing.T) {
	wallets, abNet := testutils.SetupNetworkWithWallets(t, testutils.WithTokensNode(t))

	symbol1 := "ABNFT"
	typeID11 := tokenid.NewNonFungibleTokenTypeID(t)
	typeID12 := tokenid.NewNonFungibleTokenTypeID(t)

	tokensCmd := newWalletCmdExecutor("token", "--rpc-url", abNet.TokensRpcUrl).WithHome(wallets[0].Homedir)

	addFeeCredit(t, wallets[0].Homedir, 100, "tokens", abNet.TokensRpcUrl, abNet.MoneyRpcUrl)

	// create type
	tokensCmd.Exec(t, "new-type", "non-fungible",
		"--symbol", symbol1,
		"--type", typeID11.String(),
		"--inherit-bearer-clause", predicatePtpkh)
	// TODO: AB-1448 verify with list-types command

	// second type inheriting the first one and leaves inherit-bearer clause to default (true)
	tokensCmd.Exec(t, "new-type", "non-fungible",
		"--symbol", symbol1,
		"--type", typeID12.String(),
		"--parent-type", typeID11.String(),
		"--subtype-input", predicateTrue)
	// TODO: AB-1448 verify with list-types command

	// mint
	stdout := tokensCmd.Exec(t, "new", "non-fungible",
		"--type", typeID12.String(),
		"--mint-input", predicatePtpkh+","+predicatePtpkh)
	id := extractTokenID(t, stdout.Lines[0])
	testutils.VerifyStdoutEventually(t, tokensCmd.ExecFunc(t, "list", "non-fungible"), fmt.Sprintf("ID='%s'", id))

	// send to w2
	tokensCmd.Exec(t, "send", "non-fungible",
		"--token-identifier", id.String(),
		"--address", fmt.Sprintf("0x%X", wallets[1].PubKeys[0]),
		"--key", "1",
		"--inherit-bearer-input", predicateTrue+","+predicatePtpkh)
	testutils.VerifyStdoutEventually(t, tokensCmd.WithHome(wallets[1].Homedir).ExecFunc(t, "list", "non-fungible"),
		fmt.Sprintf("ID='%s'", id))
}

func TestNFT_LockUnlock_Integration(t *testing.T) {
	wallets, abNet := testutils.SetupNetworkWithWallets(t, testutils.WithTokensNode(t))

	typeID := tokenid.NewNonFungibleTokenTypeID(t)
	symbol := "ABNFT"

	walletCmd := newWalletCmdExecutor().WithHome(wallets[0].Homedir)
	tokensCmd := walletCmd.WithPrefixArgs("token", "--rpc-url", abNet.TokensRpcUrl)

	addFeeCredit(t, wallets[0].Homedir, 100, "tokens", abNet.TokensRpcUrl, abNet.MoneyRpcUrl)

	tokensCmd.Exec(t, "new-type", "non-fungible", "--key", "1", "--symbol", symbol, "--type", typeID.String())

	// mint NFT
	stdout := tokensCmd.Exec(t, "new", "non-fungible", "--key", "1", "--type", typeID.String())
	nftID := extractTokenID(t, stdout.Lines[0])
	testutils.VerifyStdoutEventually(t, tokensCmd.ExecFunc(t, "list", "non-fungible"), fmt.Sprintf("ID='%s'", nftID))

	// lock NFT
	tokensCmd.Exec(t, "lock", "--token-identifier", nftID.String(), "--key", "1")
	testutils.VerifyStdoutEventually(t, tokensCmd.ExecFunc(t, "list", "non-fungible"), "locked='manually locked by user'")

	// unlock NFT
	tokensCmd.Exec(t, "unlock", "--token-identifier", nftID.String(), "--key", "1")
	testutils.VerifyStdoutEventually(t, tokensCmd.ExecFunc(t, "list", "non-fungible"), "locked=''")
}

func extractTokenID(t *testing.T, s string) types.UnitID {
	// Sent request for new non-fungible token with id=7EEEAA3B9F14871BB561A50C0A337C5B05475AC2C69E5675A10CB5C2727858A323
	nftIDStr := s[len(s)-66:]
	nftIdBytes, err := hex.DecodeString(nftIDStr)
	require.NoError(t, err)
	return nftIdBytes
}
