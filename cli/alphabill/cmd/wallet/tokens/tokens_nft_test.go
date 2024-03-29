package tokens

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill/partition/event"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/testutils"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/partition"
	"github.com/alphabill-org/alphabill-wallet/internal/testutils/partition/event"
	"github.com/alphabill-org/alphabill-wallet/wallet/fees"
	moneywallet "github.com/alphabill-org/alphabill-wallet/wallet/money"
)

func TestNFTs_Integration(t *testing.T) {
	network := NewAlphabillNetwork(t)
	_, err := network.abNetwork.GetNodePartition(money.DefaultSystemIdentifier)
	require.NoError(t, err)
	tokenPartition, err := network.abNetwork.GetNodePartition(tokens.DefaultSystemIdentifier)
	require.NoError(t, err)
	homedirW1 := network.homeDir
	w1key2 := network.walletKey2
	rpcUrl := tokenPartition.Nodes[0].AddrRPC
	rpcClient := network.tokensRpcClient
	ctx := network.ctx

	// create w2
	w2, homedirW2 := testutils.CreateNewTokenWallet(t, rpcUrl)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()

	// send money to w1k2 to create fee credits
	wallet := loadMoneyWallet(t, network.walletHomeDir, network.moneyRpcClient)
	_, err = wallet.Send(context.Background(), moneywallet.SendCmd{Receivers: []moneywallet.ReceiverData{{PubKey: w1key2.PubKey, Amount: 100 * 1e8}}, WaitForConfirmation: true})
	require.NoError(t, err)
	wallet.Close()

	// create fee credit for w1k2
	tokensWallet := loadTokensWallet(t, network.walletHomeDir, network.moneyRpcClient, network.tokensRpcClient)
	_, err = tokensWallet.AddFeeCredit(context.Background(), fees.AddFeeCmd{AccountIndex: 1, Amount: 50 * 1e8, DisableLocking: true})
	require.NoError(t, err)
	tokensWallet.Shutdown()

	// non-fungible token types
	typeID := randomNonFungibleTokenTypeID(t)
	typeID2 := randomNonFungibleTokenTypeID(t)
	nftID := randomNonFungibleTokenID(t)
	symbol := "ABNFT"
	execTokensCmdWithError(t, homedirW1, "new-type non-fungible", "required flag(s) \"symbol\" not set")
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type non-fungible -k 2 --symbol %s -r %s --type %s --subtype-clause ptpkh", symbol, rpcUrl, typeID))
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type non-fungible -k 2 --symbol %s -r %s --type %s --parent-type %s --subtype-input ptpkh", symbol+"2", rpcUrl, typeID2, typeID))

	// mint NFT
	execTokensCmd(t, homedirW1, fmt.Sprintf("new non-fungible -k 2 -r %s --type %s --token-identifier %s", rpcUrl, typeID, nftID))
	require.Eventually(t, testpartition.BlockchainContains(tokenPartition, func(tx *types.TransactionOrder) bool {
		return tx.PayloadType() == tokens.PayloadTypeMintNFT && bytes.Equal(tx.UnitID(), nftID)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenIndexed(t, ctx, rpcClient, w1key2.PubKeyHash.Sha256, nftID)

	// transfer NFT
	execTokensCmd(t, homedirW1, fmt.Sprintf("send non-fungible -k 2 -r %s --token-identifier %s --address 0x%X", rpcUrl, nftID, w2key.PubKey))
	require.Eventually(t, testpartition.BlockchainContains(tokenPartition, func(tx *types.TransactionOrder) bool {
		return tx.PayloadType() == tokens.PayloadTypeTransferNFT && bytes.Equal(tx.UnitID(), nftID)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenIndexed(t, ctx, rpcClient, w2key.PubKeyHash.Sha256, nftID)
	testutils.VerifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list non-fungible -r %s", rpcUrl)), fmt.Sprintf("ID='%s'", nftID))

	//check what is left in w1, nothing, that is
	testutils.VerifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list non-fungible -k 2 -r %s", rpcUrl)), "No tokens")

	// TODO AB-1448
	// list token types
	//testutils.VerifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list-types -r %s", rpcUrl)), "symbol=ABNFT (nft)")
	//testutils.VerifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list-types non-fungible -r %s", rpcUrl)), "symbol=ABNFT (nft)")

	// send money to w2 to create fee credits
	wallet = loadMoneyWallet(t, network.walletHomeDir, network.moneyRpcClient)
	_, err = wallet.Send(context.Background(), moneywallet.SendCmd{Receivers: []moneywallet.ReceiverData{{PubKey: w2key.PubKey, Amount: 100 * 1e8}}, WaitForConfirmation: true})
	require.NoError(t, err)
	wallet.Close()

	// create fee credit for w2
	tokensWallet = loadTokensWallet(t, filepath.Join(homedirW2, "wallet"), network.moneyRpcClient, network.tokensRpcClient)
	_, err = tokensWallet.AddFeeCredit(context.Background(), fees.AddFeeCmd{Amount: 50 * 1e8, DisableLocking: true})
	require.NoError(t, err)
	tokensWallet.Shutdown()

	// transfer back
	execTokensCmd(t, homedirW2, fmt.Sprintf("send non-fungible -r %s --token-identifier %s --address 0x%X -k 1", rpcUrl, nftID, w1key2.PubKey))
	ensureTokenIndexed(t, ctx, rpcClient, w1key2.PubKeyHash.Sha256, nftID)

	// mint nft from w1 and set the owner to w2
	nftID2 := randomNonFungibleTokenID(t)
	testutils.VerifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list non-fungible -r %s", rpcUrl)), "No tokens")
	execTokensCmd(t, homedirW1, fmt.Sprintf("new non-fungible -k 2 -r %s --type %s --bearer-clause ptpkh:0x%X --token-identifier %s", rpcUrl, typeID, w2key.PubKeyHash.Sha256, nftID2))
	testutils.VerifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list non-fungible -r %s", rpcUrl)), fmt.Sprintf("ID='%s'", nftID2))
}

func TestNFTDataUpdateCmd_Integration(t *testing.T) {
	network := NewAlphabillNetwork(t)
	tokenPartition, err := network.abNetwork.GetNodePartition(tokens.DefaultSystemIdentifier)
	require.NoError(t, err)
	homedir := network.homeDir
	w1key := network.walletKey1
	rpcUrl := tokenPartition.Nodes[0].AddrRPC
	rpcClient := network.tokensRpcClient
	ctx := network.ctx

	typeID := randomNonFungibleTokenTypeID(t)
	symbol := "ABNFT"

	// create type
	execTokensCmd(t, homedir, fmt.Sprintf("new-type non-fungible --symbol %s -r %s --type %s", symbol, rpcUrl, typeID))

	// create non-fungible token from using data-file
	nftID := randomNonFungibleTokenID(t)
	data := make([]byte, 1024)
	n, err := rand.Read(data)
	require.NoError(t, err)
	require.EqualValues(t, n, len(data))
	tmpfile, err := os.CreateTemp(t.TempDir(), "test")
	require.NoError(t, err)
	_, err = tmpfile.Write(data)
	require.NoError(t, err)
	execTokensCmd(t, homedir, fmt.Sprintf("new non-fungible -r %s --type %s --token-identifier %s --data-file %s", rpcUrl, typeID, nftID, tmpfile.Name()))
	require.Eventually(t, testpartition.BlockchainContains(tokenPartition, func(tx *types.TransactionOrder) bool {
		if tx.PayloadType() == tokens.PayloadTypeMintNFT && bytes.Equal(tx.UnitID(), nftID) {
			mintNonFungibleAttr := &tokens.MintNonFungibleTokenAttributes{}
			require.NoError(t, tx.UnmarshalAttributes(mintNonFungibleAttr))
			require.Equal(t, data, mintNonFungibleAttr.Data)
			return true
		}
		return false
	}), test.WaitDuration, test.WaitTick)
	nft := ensureTokenIndexed(t, ctx, rpcClient, w1key.PubKeyHash.Sha256, nftID)
	testutils.VerifyStdout(t, execTokensCmd(t, homedir, fmt.Sprintf("list non-fungible -r %s", rpcUrl)), fmt.Sprintf("ID='%s'", nftID))
	require.Equal(t, data, nft.NftData)

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
	execTokensCmd(t, homedir, fmt.Sprintf("update -r %s --token-identifier %s --data-file %s", rpcUrl, nftID, tmpfile.Name()))
	require.Eventually(t, testpartition.BlockchainContains(tokenPartition, func(tx *types.TransactionOrder) bool {
		if tx.PayloadType() == tokens.PayloadTypeUpdateNFT && bytes.Equal(tx.UnitID(), nftID) {
			dataUpdateAttrs := &tokens.UpdateNonFungibleTokenAttributes{}
			require.NoError(t, tx.UnmarshalAttributes(dataUpdateAttrs))
			require.Equal(t, data2, dataUpdateAttrs.Data)
			return true
		}
		return false
	}), test.WaitDuration, test.WaitTick)

	// check that data was updated on the rpc node
	require.Eventually(t, func() bool {
		return bytes.Equal(data2, ensureTokenIndexed(t, ctx, rpcClient, w1key.PubKeyHash.Sha256, nftID).NftData)
	}, 2*test.WaitDuration, test.WaitTick)

	// create non-updatable nft
	nftID2 := randomNonFungibleTokenID(t)
	execTokensCmd(t, homedir, fmt.Sprintf("new non-fungible -r %s --type %s --token-identifier %s --data 01 --data-update-clause false", rpcUrl, typeID, nftID2))
	nft2 := ensureTokenIndexed(t, ctx, rpcClient, w1key.PubKeyHash.Sha256, nftID2)
	require.Equal(t, []byte{0x01}, nft2.NftData)

	// try to update and Observe failure
	execTokensCmd(t, homedir, fmt.Sprintf("update -r %s --token-identifier %s --data 02 --data-update-input false,true -w false", rpcUrl, nftID2))
	testevent.ContainsEvent(t, tokenPartition.Nodes[0].EventHandler, event.TransactionFailed)
}

func TestNFT_InvariantPredicate_Integration(t *testing.T) {
	network := NewAlphabillNetwork(t)
	tokenPartition, err := network.abNetwork.GetNodePartition(tokens.DefaultSystemIdentifier)
	require.NoError(t, err)
	homedirW1 := network.homeDir
	w1key := network.walletKey1
	rpcUrl := tokenPartition.Nodes[0].AddrRPC
	rpcClient := network.tokensRpcClient
	ctx := network.ctx

	w2, homedirW2 := testutils.CreateNewTokenWallet(t, rpcUrl)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()

	symbol1 := "ABNFT"
	typeID11 := randomNonFungibleTokenTypeID(t)
	typeID12 := randomNonFungibleTokenTypeID(t)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type non-fungible -r %s --symbol %s --type %s --inherit-bearer-clause %s", rpcUrl, symbol1, typeID11, predicatePtpkh))
	require.Eventually(t, testpartition.BlockchainContains(tokenPartition, func(tx *types.TransactionOrder) bool {
		return bytes.Equal(tx.UnitID(), typeID11)
	}), test.WaitDuration, test.WaitTick)

	//second type inheriting the first one and leaves inherit-bearer clause to default (true)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type non-fungible -r %s --symbol %s --type %s --parent-type %s --subtype-input %s", rpcUrl, symbol1, typeID12, typeID11, predicateTrue))
	require.Eventually(t, testpartition.BlockchainContains(tokenPartition, func(tx *types.TransactionOrder) bool {
		return bytes.Equal(tx.UnitID(), typeID12)
	}), test.WaitDuration, test.WaitTick)

	//mint
	id := randomNonFungibleTokenID(t)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new non-fungible -r %s --type %s --token-identifier %s --mint-input %s,%s", rpcUrl, typeID12, id, predicatePtpkh, predicatePtpkh))
	ensureTokenIndexed(t, ctx, rpcClient, w1key.PubKeyHash.Sha256, id)
	testutils.VerifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list non-fungible -r %s", rpcUrl)), "symbol='ABNFT'")
	//send to w2
	execTokensCmd(t, homedirW1, fmt.Sprintf("send non-fungible -r %s --token-identifier %s --address 0x%X -k 1 --inherit-bearer-input %s,%s", rpcUrl, id, w2key.PubKey, predicateTrue, predicatePtpkh))
	ensureTokenIndexed(t, ctx, rpcClient, w2key.PubKeyHash.Sha256, id)
	testutils.VerifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list non-fungible -r %s", rpcUrl)), "symbol='ABNFT'")
}

func TestNFT_LockUnlock_Integration(t *testing.T) {
	network := NewAlphabillNetwork(t)
	_, err := network.abNetwork.GetNodePartition(money.DefaultSystemIdentifier)
	require.NoError(t, err)
	tokensPartition, err := network.abNetwork.GetNodePartition(tokens.DefaultSystemIdentifier)
	require.NoError(t, err)
	homedirW1 := network.homeDir
	rpcUrl := tokensPartition.Nodes[0].AddrRPC
	rpcClient := network.tokensRpcClient
	w1key := network.walletKey1
	ctx := network.ctx

	typeID := randomNonFungibleTokenTypeID(t)
	nftID := randomNonFungibleTokenID(t)
	symbol := "ABNFT"
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type non-fungible -k 1 --symbol %s -r %s --type %s", symbol, rpcUrl, typeID))

	// mint NFT
	execTokensCmd(t, homedirW1, fmt.Sprintf("new non-fungible -k 1 -r %s --type %s --token-identifier %s", rpcUrl, typeID, nftID))
	require.Eventually(t, testpartition.BlockchainContains(tokensPartition, func(tx *types.TransactionOrder) bool {
		return tx.PayloadType() == tokens.PayloadTypeMintNFT && bytes.Equal(tx.UnitID(), nftID)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenIndexed(t, ctx, rpcClient, w1key.PubKeyHash.Sha256, nftID)

	// lock NFT
	execTokensCmd(t, homedirW1, fmt.Sprintf("lock -r %s --token-identifier %s -k 1", rpcUrl, nftID))
	testutils.VerifyStdoutEventually(t, func() *testutils.TestConsoleWriter {
		return execTokensCmd(t, homedirW1, fmt.Sprintf("list non-fungible -r %s", rpcUrl))
	}, "locked='manually locked by user'")

	// unlock NFT
	execTokensCmd(t, homedirW1, fmt.Sprintf("unlock -r %s --token-identifier %s -k 1", rpcUrl, nftID))
	testutils.VerifyStdoutEventually(t, func() *testutils.TestConsoleWriter {
		return execTokensCmd(t, homedirW1, fmt.Sprintf("list non-fungible -r %s", rpcUrl))
	}, "locked=''")
}
