package cmd

import (
	"bytes"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill/internal/script"
	ttxs "github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/tokens"
	twb "github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
)

const (
	cmdFlagSymbol                     = "symbol"
	cmdFlagDecimals                   = "decimals"
	cmdFlagParentType                 = "parent-type"
	cmdFlagSybTypeClause              = "subtype-clause"
	cmdFlagSybTypeClauseInput         = "subtype-input"
	cmdFlagMintClause                 = "mint-clause"
	cmdFlagMintClauseInput            = "mint-input"
	cmdFlagInheritBearerClause        = "inherit-bearer-clause"
	cmdFlagInheritBearerClauseInput   = "inherit-bearer-input"
	cmdFlagTokenDataUpdateClause      = "data-update-clause"
	cmdFlagTokenDataUpdateClauseInput = "data-update-input"
	cmdFlagAmount                     = "amount"
	cmdFlagType                       = "type"
	cmdFlagTokenId                    = "token-identifier"
	cmdFlagTokenURI                   = "token-uri"
	cmdFlagTokenData                  = "data"
	cmdFlagTokenDataFile              = "data-file"

	predicateTrue  = "true"
	predicatePtpkh = "ptpkh"

	maxBinaryFile64Kb = 64 * 1024
	maxDecimalPlaces  = 8
)

var NoParent = []byte{0x00}

type runTokenListTypesCmd func(cmd *cobra.Command, config *walletConfig, kind twb.Kind) error
type runTokenListCmd func(cmd *cobra.Command, config *walletConfig, kind twb.Kind, accountNumber *uint64) error

func tokenCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "create and manage fungible and non-fungible tokens",
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand like new-type, send etc")
		},
	}
	cmd.AddCommand(tokenCmdNewType(config))
	cmd.AddCommand(tokenCmdNewToken(config))
	cmd.AddCommand(tokenCmdUpdateNFTData(config))
	cmd.AddCommand(tokenCmdSend(config))
	cmd.AddCommand(tokenCmdDC(config))
	cmd.AddCommand(tokenCmdList(config, execTokenCmdList))
	cmd.AddCommand(tokenCmdListTypes(config, execTokenCmdListTypes))
	cmd.PersistentFlags().StringP(alphabillNodeURLCmdName, "u", defaultAlphabillNodeURL, "alphabill backend uri to connect to")
	cmd.PersistentFlags().BoolP(waitForConfCmdName, "w", false, "waits for transaction confirmation on the blockchain, otherwise just broadcasts the transaction")
	return cmd
}

func tokenCmdNewType(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new-type",
		Short: "create new token type",
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand: fungible|non-fungible")
		},
	}
	cmd.AddCommand(addCommonAccountFlags(addCommonTypeFlags(tokenCmdNewTypeFungible(config))))
	cmd.AddCommand(addCommonAccountFlags(addCommonTypeFlags(tokenCmdNewTypeNonFungible(config))))
	return cmd
}

func addCommonAccountFlags(cmd *cobra.Command) *cobra.Command {
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "which key to use for sending the transaction")
	return cmd
}

func addCommonTypeFlags(cmd *cobra.Command) *cobra.Command {
	cmd.Flags().String(cmdFlagSymbol, "", "token symbol (mandatory)")
	err := cmd.MarkFlagRequired(cmdFlagSymbol)
	if err != nil {
		return nil
	}
	cmd.Flags().BytesHex(cmdFlagParentType, NoParent, "unit identifier of a parent type in hexadecimal format (optional)")
	cmd.Flags().StringSlice(cmdFlagSybTypeClauseInput, nil, "input to satisfy the parent type creation clause (mandatory with --parent-type)")
	cmd.MarkFlagsRequiredTogether(cmdFlagParentType, cmdFlagSybTypeClauseInput)
	cmd.Flags().String(cmdFlagSybTypeClause, predicateTrue, "predicate to control sub typing, values <true|false|ptpkh>, defaults to 'true' (optional)")
	cmd.Flags().String(cmdFlagMintClause, predicatePtpkh, "predicate to control minting of this type, values <true|false|ptpkh>, defaults to 'ptpkh' (optional)")
	cmd.Flags().String(cmdFlagInheritBearerClause, predicateTrue, "predicate that will be inherited by subtypes into their bearer clauses, values <true|false|ptpkh>, defaults to 'true' (optional)")
	return cmd
}

func tokenCmdNewTypeFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fungible",
		Short: "create new fungible token type",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdNewTypeFungible(cmd, config)
		},
	}
	cmd.Flags().Uint32(cmdFlagDecimals, 8, "token decimal (optional)")
	cmd.Flags().BytesHex(cmdFlagType, nil, "type unit identifier (hex)")
	_ = cmd.Flags().MarkHidden(cmdFlagType)
	return cmd
}

func execTokenCmdNewTypeFungible(cmd *cobra.Command, config *walletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	typeId, err := getHexFlag(cmd, cmdFlagType)
	if err != nil {
		return err
	}
	symbol, err := cmd.Flags().GetString(cmdFlagSymbol)
	if err != nil {
		return err
	}
	decimals, err := cmd.Flags().GetUint32(cmdFlagDecimals)
	if err != nil {
		return err
	}
	if decimals > maxDecimalPlaces {
		return fmt.Errorf("argument \"%v\" for \"--decimals\" flag is out of range, max value %v", decimals, maxDecimalPlaces)
	}
	am := tw.GetAccountManager()
	parentType, creationInputs, err := readParentTypeInfo(cmd, am)
	if err != nil {
		return err
	}
	subTypeCreationPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagSybTypeClause, am)
	if err != nil {
		return err
	}
	mintTokenPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagMintClause, am)
	if err != nil {
		return err
	}
	invariantPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagInheritBearerClause, am)
	if err != nil {
		return err
	}
	a := &ttxs.CreateFungibleTokenTypeAttributes{
		Symbol:                             symbol,
		DecimalPlaces:                      decimals,
		ParentTypeId:                       parentType,
		SubTypeCreationPredicateSignatures: nil, // will be filled by the wallet
		SubTypeCreationPredicate:           subTypeCreationPredicate,
		TokenCreationPredicate:             mintTokenPredicate,
		InvariantPredicate:                 invariantPredicate,
	}
	id, err := tw.NewFungibleType(cmd.Context(), accountNumber, a, typeId, creationInputs)
	if err != nil {
		return err
	}
	consoleWriter.Println(fmt.Sprintf("Created new fungible token type with id=%X", id))
	return nil
}

func tokenCmdNewTypeNonFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "non-fungible",
		Short: "create new non-fungible token type",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdNewTypeNonFungible(cmd, config)
		},
	}
	cmd.Flags().BytesHex(cmdFlagType, nil, "type unit identifier (hex)")
	_ = cmd.Flags().MarkHidden(cmdFlagType)
	cmd.Flags().String(cmdFlagTokenDataUpdateClause, predicateTrue, "data update predicate, values <true|false|ptpkh>, defaults to 'true' (optional)")
	return cmd
}

func execTokenCmdNewTypeNonFungible(cmd *cobra.Command, config *walletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	typeId, err := getHexFlag(cmd, cmdFlagType)
	if err != nil {
		return err
	}
	symbol, err := cmd.Flags().GetString(cmdFlagSymbol)
	if err != nil {
		return err
	}
	am := tw.GetAccountManager()
	parentType, creationInputs, err := readParentTypeInfo(cmd, am)
	if err != nil {
		return err
	}
	subTypeCreationPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagSybTypeClause, am)
	if err != nil {
		return err
	}
	mintTokenPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagMintClause, am)
	if err != nil {
		return err
	}
	dataUpdatePredicate, err := parsePredicateClauseCmd(cmd, cmdFlagTokenDataUpdateClause, am)
	if err != nil {
		return err
	}
	invariantPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagInheritBearerClause, am)
	if err != nil {
		return err
	}
	a := &ttxs.CreateNonFungibleTokenTypeAttributes{
		Symbol:                             symbol,
		ParentTypeId:                       parentType,
		SubTypeCreationPredicateSignatures: nil, // will be filled by the wallet
		SubTypeCreationPredicate:           subTypeCreationPredicate,
		TokenCreationPredicate:             mintTokenPredicate,
		InvariantPredicate:                 invariantPredicate,
		DataUpdatePredicate:                dataUpdatePredicate,
	}
	id, err := tw.NewNonFungibleType(cmd.Context(), accountNumber, a, typeId, creationInputs)
	if err != nil {
		return err
	}
	consoleWriter.Println(fmt.Sprintf("Created new NFT type with id=%X", id))
	return nil
}

func tokenCmdNewToken(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new",
		Short: "mint new token",
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand: fungible|non-fungible")
		},
	}
	cmd.AddCommand(addCommonAccountFlags(tokenCmdNewTokenFungible(config)))
	cmd.AddCommand(addCommonAccountFlags(tokenCmdNewTokenNonFungible(config)))
	return cmd
}

func tokenCmdNewTokenFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fungible",
		Short: "mint new fungible token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdNewTokenFungible(cmd, config)
		},
	}
	cmd.Flags().String(cmdFlagAmount, "", "amount, must be bigger than 0 and is interpreted according to token type precision (decimals)")
	err := cmd.MarkFlagRequired(cmdFlagAmount)
	if err != nil {
		return nil
	}
	cmd.Flags().BytesHex(cmdFlagType, nil, "type unit identifier (hex)")
	err = cmd.MarkFlagRequired(cmdFlagType)
	if err != nil {
		return nil
	}
	cmd.Flags().StringSlice(cmdFlagMintClauseInput, []string{predicatePtpkh}, "input to satisfy the type's minting clause")
	return cmd
}

func execTokenCmdNewTokenFungible(cmd *cobra.Command, config *walletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	amountStr, err := cmd.Flags().GetString(cmdFlagAmount)
	if err != nil {
		return err
	}
	typeId, err := getHexFlag(cmd, cmdFlagType)
	if err != nil {
		return err
	}
	ci, err := readPredicateInput(cmd, cmdFlagMintClauseInput, tw.GetAccountManager())
	if err != nil {
		return err
	}
	tt, err := tw.GetTokenType(cmd.Context(), typeId)
	if err != nil {
		return err
	}
	// convert amount from string to uint64
	amount, err := stringToAmount(amountStr, tt.DecimalPlaces)
	if err != nil {
		return err
	}
	if amount == 0 {
		return fmt.Errorf("invalid parameter \"%s\" for \"--amount\": 0 is not valid amount", amountStr)
	}

	id, err := tw.NewFungibleToken(cmd.Context(), accountNumber, typeId, amount, ci)
	if err != nil {
		return err
	}

	consoleWriter.Println(fmt.Sprintf("Created new fungible token with id=%X", id))
	return nil
}

func tokenCmdNewTokenNonFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "non-fungible",
		Short: "mint new non-fungible token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdNewTokenNonFungible(cmd, config)
		},
	}
	cmd.Flags().BytesHex(cmdFlagType, nil, "type unit identifier (hex)")
	err := cmd.MarkFlagRequired(cmdFlagType)
	if err != nil {
		return nil
	}
	cmd.Flags().String(cmdFlagTokenURI, "", "URI to associated resource, ie. jpg file on IPFS")
	cmd.Flags().BytesHex(cmdFlagTokenData, nil, "custom data (hex)")
	cmd.Flags().String(cmdFlagTokenDataFile, "", "data file (max 64Kb) path")
	cmd.MarkFlagsMutuallyExclusive(cmdFlagTokenData, cmdFlagTokenDataFile)
	cmd.Flags().String(cmdFlagTokenDataUpdateClause, predicateTrue, "data update predicate, values <true|false|ptpkh>, defaults to 'true' (optional)")
	cmd.Flags().StringSlice(cmdFlagMintClauseInput, []string{predicatePtpkh}, "input to satisfy the type's minting clause")
	cmd.Flags().BytesHex(cmdFlagTokenId, nil, "unit identifier of token (hex)")
	_ = cmd.Flags().MarkHidden(cmdFlagTokenId)
	return cmd
}

func execTokenCmdNewTokenNonFungible(cmd *cobra.Command, config *walletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	typeId, err := getHexFlag(cmd, cmdFlagType)
	if err != nil {
		return err
	}
	tokenId, err := getHexFlag(cmd, cmdFlagTokenId)
	if err != nil {
		return err
	}
	uri, err := cmd.Flags().GetString(cmdFlagTokenURI)
	if err != nil {
		return err
	}
	data, err := readNFTData(cmd, false)
	if err != nil {
		return err
	}
	am := tw.GetAccountManager()
	ci, err := readPredicateInput(cmd, cmdFlagMintClauseInput, am)
	if err != nil {
		return err
	}
	dataUpdatePredicate, err := parsePredicateClauseCmd(cmd, cmdFlagTokenDataUpdateClause, am)
	if err != nil {
		return err
	}
	a := &ttxs.MintNonFungibleTokenAttributes{
		Bearer:                           nil, // will be set in the wallet
		NftType:                          typeId,
		Uri:                              uri,
		Data:                             data,
		DataUpdatePredicate:              dataUpdatePredicate,
		TokenCreationPredicateSignatures: nil, // will be set in the wallet
	}
	id, err := tw.NewNFT(cmd.Context(), accountNumber, a, tokenId, ci)
	if err != nil {
		return err
	}

	consoleWriter.Println(fmt.Sprintf("Created new non-fungible token with id=%X", id))
	return nil
}

func tokenCmdSend(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send",
		Short: "send a token",
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand: fungible|non-fungible")
		},
	}
	cmd.AddCommand(tokenCmdSendFungible(config))
	cmd.AddCommand(tokenCmdSendNonFungible(config))
	return cmd
}

func tokenCmdSendFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fungible",
		Short: "send fungible token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdSendFungible(cmd, config)
		},
	}
	cmd.Flags().StringSlice(cmdFlagInheritBearerClauseInput, []string{predicateTrue}, "input to satisfy the type's minting clause")
	cmd.Flags().String(cmdFlagAmount, "", "amount, must be bigger than 0 and is interpreted according to token type precision (decimals)")
	err := cmd.MarkFlagRequired(cmdFlagAmount)
	if err != nil {
		return nil
	}
	cmd.Flags().BytesHex(cmdFlagType, nil, "type unit identifier (hex)")
	err = cmd.MarkFlagRequired(cmdFlagType)
	if err != nil {
		return nil
	}
	cmd.Flags().StringP(addressCmdName, "a", "", "compressed secp256k1 public key of the receiver in hexadecimal format, must start with 0x and be 68 characters in length")
	err = cmd.MarkFlagRequired(addressCmdName)
	if err != nil {
		return nil
	}
	return addCommonAccountFlags(cmd)
}

// getPubKeyBytes returns 'nil' for flag value 'true', must be interpreted as 'always true' predicate
func getPubKeyBytes(cmd *cobra.Command, flag string) ([]byte, error) {
	pubKeyHex, err := cmd.Flags().GetString(flag)
	if err != nil {
		return nil, err
	}
	var pubKey []byte
	if pubKeyHex == predicateTrue {
		pubKey = nil // this will assign 'always true' predicate
	} else {
		pk, ok := pubKeyHexToBytes(pubKeyHex)
		if !ok {
			return nil, fmt.Errorf("address in not in valid format: %s", pubKeyHex)
		}
		pubKey = pk
	}
	return pubKey, nil
}

func execTokenCmdSendFungible(cmd *cobra.Command, config *walletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	typeId, err := getHexFlag(cmd, cmdFlagType)
	if err != nil {
		return err
	}

	amountStr, err := cmd.Flags().GetString(cmdFlagAmount)
	if err != nil {
		return err
	}

	pubKey, err := getPubKeyBytes(cmd, addressCmdName)
	if err != nil {
		return err
	}

	ib, err := readPredicateInput(cmd, cmdFlagInheritBearerClauseInput, tw.GetAccountManager())
	if err != nil {
		return err
	}

	// get token type and convert amount string
	tt, err := tw.GetTokenType(cmd.Context(), typeId)
	if err != nil {
		return err
	}
	// convert amount from string to uint64
	targetValue, err := stringToAmount(amountStr, tt.DecimalPlaces)
	if err != nil {
		return err
	}
	if targetValue == 0 {
		return fmt.Errorf("invalid parameter \"%s\" for \"--amount\": 0 is not valid amount", amountStr)
	}
	return tw.SendFungible(cmd.Context(), accountNumber, typeId, targetValue, pubKey, ib)
}

func tokenCmdSendNonFungible(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "non-fungible",
		Short: "transfer non-fungible token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdSendNonFungible(cmd, config)
		},
	}
	cmd.Flags().StringSlice(cmdFlagInheritBearerClauseInput, []string{predicateTrue}, "input to satisfy the type's minting clause")
	cmd.Flags().BytesHex(cmdFlagTokenId, nil, "unit identifier of token (hex)")
	err := cmd.MarkFlagRequired(cmdFlagTokenId)
	if err != nil {
		return nil
	}
	cmd.Flags().StringP(addressCmdName, "a", "", "compressed secp256k1 public key of the receiver in hexadecimal format, must start with 0x and be 68 characters in length")
	err = cmd.MarkFlagRequired(addressCmdName)
	if err != nil {
		return nil
	}
	return addCommonAccountFlags(cmd)
}

func execTokenCmdSendNonFungible(cmd *cobra.Command, config *walletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	tokenId, err := getHexFlag(cmd, cmdFlagTokenId)
	if err != nil {
		return err
	}

	pubKey, err := getPubKeyBytes(cmd, addressCmdName)
	if err != nil {
		return err
	}

	ib, err := readPredicateInput(cmd, cmdFlagInheritBearerClauseInput, tw.GetAccountManager())
	if err != nil {
		return err
	}

	return tw.TransferNFT(cmd.Context(), accountNumber, tokenId, pubKey, ib)
}

func tokenCmdDC(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collect-dust",
		Short: "join fungible tokens into one unit",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdDC(cmd, config)
		},
	}
	return cmd
}

func execTokenCmdDC(cmd *cobra.Command, config *walletConfig) error {
	// TODO: AB-751
	return nil
}

func tokenCmdUpdateNFTData(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "update the data field on a non-fungible token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdUpdateNFTData(cmd, config)
		},
	}
	cmd.Flags().BytesHex(cmdFlagTokenId, nil, "token identifier (hex)")
	err := cmd.MarkFlagRequired(cmdFlagTokenId)
	if err != nil {
		panic(err)
	}
	cmd.Flags().BytesHex(cmdFlagTokenData, nil, "custom data (hex)")
	cmd.Flags().String(cmdFlagTokenDataFile, "", "data file (max 64Kb) path")
	cmd.MarkFlagsMutuallyExclusive(cmdFlagTokenData, cmdFlagTokenDataFile)
	cmd.Flags().StringSlice(cmdFlagTokenDataUpdateClauseInput, []string{predicateTrue, predicateTrue}, "input to satisfy the data-update clauses")
	return addCommonAccountFlags(cmd)
}

func execTokenCmdUpdateNFTData(cmd *cobra.Command, config *walletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}

	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	tokenId, err := getHexFlag(cmd, cmdFlagTokenId)
	if err != nil {
		return err
	}

	data, err := readNFTData(cmd, true)
	if err != nil {
		return err
	}

	du, err := readPredicateInput(cmd, cmdFlagTokenDataUpdateClauseInput, tw.GetAccountManager())
	if err != nil {
		return err
	}

	return tw.UpdateNFTData(cmd.Context(), accountNumber, tokenId, data, du)
}

func tokenCmdList(config *walletConfig, runner runTokenListCmd) *cobra.Command {
	var accountNumber uint64
	cmd := &cobra.Command{
		Use:   "list",
		Short: "lists all available tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, twb.Any, &accountNumber)
		},
	}
	// add persistent password flags
	cmd.PersistentFlags().BoolP(passwordPromptCmdName, "p", false, passwordPromptUsage)
	cmd.PersistentFlags().String(passwordArgCmdName, "", passwordArgUsage)
	// add sub commands
	cmd.AddCommand(tokenCmdListFungible(config, runner, &accountNumber))
	cmd.AddCommand(tokenCmdListNonFungible(config, runner, &accountNumber))
	cmd.PersistentFlags().Uint64VarP(&accountNumber, keyCmdName, "k", 0, "which key to use for sending the transaction, 0 for all tokens from all accounts")
	return cmd
}

func tokenCmdListFungible(config *walletConfig, runner runTokenListCmd, accountNumber *uint64) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fungible",
		Short: "lists fungible tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, twb.Fungible, accountNumber)
		},
	}
	return cmd
}

func tokenCmdListNonFungible(config *walletConfig, runner runTokenListCmd, accountNumber *uint64) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "non-fungible",
		Short: "lists non-fungible tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, twb.NonFungible, accountNumber)
		},
	}
	return cmd
}

func execTokenCmdList(cmd *cobra.Command, config *walletConfig, kind twb.Kind, accountNumber *uint64) error {
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	res, err := tw.ListTokens(cmd.Context(), kind, *accountNumber)
	if err != nil {
		return err
	}

	accounts := make([]uint64, 0, len(res))
	for accNr := range res {
		accounts = append(accounts, accNr)
	}
	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i] < accounts[j]
	})

	atLeastOneFound := false
	for _, accNr := range accounts {
		toks := res[accNr]
		var ownerKey string
		if accNr == 0 {
			ownerKey = "Tokens spendable by anyone:"
		} else {
			ownerKey = fmt.Sprintf("Tokens owned by account #%v", accNr)
		}
		consoleWriter.Println(ownerKey)
		sort.Slice(toks, func(i, j int) bool {
			// Fungible, then Non-fungible
			return toks[i].Kind < toks[j].Kind
		})
		for _, tok := range toks {
			atLeastOneFound = true
			if tok.Kind == twb.Fungible {
				amount := amountToString(tok.Amount, tok.Decimals)
				consoleWriter.Println(fmt.Sprintf("ID='%X', Symbol='%s', amount='%v', token-type='%X' (%v)", tok.ID, tok.Symbol, amount, tok.TypeID, tok.Kind))
			} else {
				consoleWriter.Println(fmt.Sprintf("ID='%X', Symbol='%s', token-type='%X', URI='%s' (%v)", tok.ID, tok.Symbol, tok.TypeID, tok.NftURI, tok.Kind))
			}
		}
	}
	if !atLeastOneFound {
		consoleWriter.Println("No tokens")
	}
	return nil
}

func tokenCmdListTypes(config *walletConfig, runner runTokenListTypesCmd) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-types",
		Short: "lists token types",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, twb.Any)
		},
	}
	// add password flags as persistent
	cmd.PersistentFlags().BoolP(passwordPromptCmdName, "p", false, passwordPromptUsage)
	cmd.PersistentFlags().String(passwordArgCmdName, "", passwordArgUsage)
	// add optional sub-commands to filter fungible and non-fungible types
	cmd.AddCommand(&cobra.Command{
		Use:   "fungible",
		Short: "lists fungible types",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, twb.Fungible)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "non-fungible",
		Short: "lists non-fungible types",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, twb.NonFungible)
		},
	})
	return cmd
}

func execTokenCmdListTypes(cmd *cobra.Command, config *walletConfig, kind twb.Kind) error {
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	res, err := tw.ListTokenTypes(cmd.Context(), kind)
	if err != nil {
		return err
	}
	for _, tok := range res {
		consoleWriter.Println(fmt.Sprintf("ID=%X, symbol=%s (%v)", tok.ID, tok.Symbol, tok.Kind))
	}
	return nil
}

func initTokensWallet(cmd *cobra.Command, config *walletConfig) (*tokens.Wallet, error) {
	uri, err := cmd.Flags().GetString(alphabillNodeURLCmdName)
	if err != nil {
		return nil, err
	}
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return nil, err
	}
	confirmTx, err := cmd.Flags().GetBool(waitForConfCmdName)
	if err != nil {
		return nil, err
	}
	tw, err := tokens.New(ttxs.DefaultTokenTxSystemIdentifier, uri, am, confirmTx)
	if err != nil {
		return nil, err
	}
	return tw, nil
}

func readParentTypeInfo(cmd *cobra.Command, am account.Manager) (twb.TokenTypeID, []*tokens.PredicateInput, error) {
	parentType, err := getHexFlag(cmd, cmdFlagParentType)
	if err != nil {
		return nil, nil, err
	}

	if len(parentType) == 0 || bytes.Equal(parentType, NoParent) {
		return NoParent, []*tokens.PredicateInput{{Argument: script.PredicateArgumentEmpty()}}, nil
	}

	creationInputs, err := readPredicateInput(cmd, cmdFlagSybTypeClauseInput, am)
	if err != nil {
		return nil, nil, err
	}

	return parentType, creationInputs, nil
}

func readPredicateInput(cmd *cobra.Command, flag string, am account.Manager) ([]*tokens.PredicateInput, error) {
	creationInputStrs, err := cmd.Flags().GetStringSlice(flag)
	if err != nil {
		return nil, err
	}
	if len(creationInputStrs) == 0 {
		return []*tokens.PredicateInput{{Argument: script.PredicateArgumentEmpty()}}, nil
	}
	creationInputs, err := tokens.ParsePredicates(creationInputStrs, am)
	if err != nil {
		return nil, err
	}
	return creationInputs, nil
}

// parsePredicateClause uses the following format:
// empty string returns "always true"
// true
// false
// ptpkh
// ptpkh:1
// ptpkh:0x<hex> where hex value is the hash of a public key
func parsePredicateClauseCmd(cmd *cobra.Command, flag string, am account.Manager) ([]byte, error) {
	clause, err := cmd.Flags().GetString(flag)
	if err != nil {
		return nil, err
	}
	return tokens.ParsePredicateClause(clause, am)
}

func readNFTData(cmd *cobra.Command, required bool) ([]byte, error) {
	if required && !cmd.Flags().Changed(cmdFlagTokenData) && !cmd.Flags().Changed(cmdFlagTokenDataFile) {
		return nil, fmt.Errorf("either of ['--%s', '--%s'] flags must be specified", cmdFlagTokenData, cmdFlagTokenDataFile)
	}
	data, err := getHexFlag(cmd, cmdFlagTokenData)
	if err != nil {
		return nil, err
	}
	dataFilePath, err := cmd.Flags().GetString(cmdFlagTokenDataFile)
	if err != nil {
		return nil, err
	}
	if len(dataFilePath) > 0 {
		data, err = readDataFile(dataFilePath)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

// getHexFlag returns nil in case array is empty (weird behaviour by cobra)
func getHexFlag(cmd *cobra.Command, flag string) ([]byte, error) {
	res, err := cmd.Flags().GetBytesHex(flag)
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, err
	}
	return res, err
}

func readDataFile(path string) ([]byte, error) {
	size, err := util.GetFileSize(path)
	if err != nil {
		return nil, fmt.Errorf("data-file read error: %w", err)
	}
	// verify file max 64KB
	if size > maxBinaryFile64Kb {
		return nil, fmt.Errorf("data-file read error: file size over 64Kb limit")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("data-file read error: %w", err)
	}
	return data, nil
}