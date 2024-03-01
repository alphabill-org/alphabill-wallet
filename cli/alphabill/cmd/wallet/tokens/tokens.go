package tokens

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	cliaccount "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/util/account"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/args"
	"github.com/alphabill-org/alphabill-wallet/client/rpc"
	"github.com/alphabill-org/alphabill-wallet/util"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	tokenswallet "github.com/alphabill-org/alphabill-wallet/wallet/tokens"
)

const (
	cmdFlagSymbol                     = "symbol"
	cmdFlagName                       = "name"
	cmdFlagIconFile                   = "icon-file"
	cmdFlagDecimals                   = "decimals"
	cmdFlagParentType                 = "parent-type"
	cmdFlagSybTypeClause              = "subtype-clause"
	cmdFlagSybTypeClauseInput         = "subtype-input"
	cmdFlagMintClause                 = "mint-clause"
	cmdFlagBearerClause               = "bearer-clause"
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

	cmdFlagWithAll       = "with-all"
	cmdFlagWithTypeName  = "with-type-name"
	cmdFlagWithTokenURI  = "with-token-uri"
	cmdFlagWithTokenData = "with-token-data"

	predicateTrue  = "true"
	predicatePtpkh = "ptpkh"

	iconFileExtSvgz     = ".svgz"
	iconFileExtSvgzType = "image/svg+xml; encoding=gzip"

	maxBinaryFile64KiB = 64 * 1024
	maxDecimalPlaces   = 8
)

type runTokenListTypesCmd func(cmd *cobra.Command, config *types.WalletConfig, accountNumber *uint64, kind tokenswallet.Kind) error
type runTokenListCmd func(cmd *cobra.Command, config *types.WalletConfig, accountNumber *uint64, kind tokenswallet.Kind) error

func NewTokenCmd(config *types.WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "create and manage fungible and non-fungible tokens",
	}
	cmd.AddCommand(tokenCmdNewType(config))
	cmd.AddCommand(tokenCmdNewToken(config))
	cmd.AddCommand(tokenCmdUpdateNFTData(config))
	cmd.AddCommand(tokenCmdSend(config))
	cmd.AddCommand(tokenCmdDC(config))
	cmd.AddCommand(tokenCmdList(config, execTokenCmdList))
	cmd.AddCommand(tokenCmdListTypes(config, execTokenCmdListTypes))
	cmd.AddCommand(tokenCmdLock(config))
	cmd.AddCommand(tokenCmdUnlock(config))
	cmd.PersistentFlags().StringP(args.RpcUrl, "r", args.DefaultTokensRpcUrl, "rpc node url")
	cmd.PersistentFlags().StringP(args.WaitForConfCmdName, "w", "true", "waits for transaction confirmation on the blockchain, otherwise just broadcasts the transaction")
	return cmd
}

func tokenCmdNewType(config *types.WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new-type",
		Short: "create new token type",
	}
	cmd.AddCommand(addCommonAccountFlags(addCommonTypeFlags(tokenCmdNewTypeFungible(config))))
	cmd.AddCommand(addCommonAccountFlags(addCommonTypeFlags(tokenCmdNewTypeNonFungible(config))))
	return cmd
}

func addCommonAccountFlags(cmd *cobra.Command) *cobra.Command {
	cmd.Flags().Uint64P(args.KeyCmdName, "k", 1, "which key to use for sending the transaction")
	return cmd
}

func addDataFlags(cmd *cobra.Command) {
	altMsg := ". Alternatively flag %q can be used to add data."
	setHexFlag(cmd, cmdFlagTokenData, nil, "custom data (hex)"+fmt.Sprintf(altMsg, cmdFlagTokenDataFile))
	cmd.Flags().String(cmdFlagTokenDataFile, "", "data file (max 64Kb) path"+fmt.Sprintf(altMsg, cmdFlagTokenData))
	cmd.MarkFlagsMutuallyExclusive(cmdFlagTokenData, cmdFlagTokenDataFile)
}

func addCommonTypeFlags(cmd *cobra.Command) *cobra.Command {
	cmd.Flags().String(cmdFlagSymbol, "", "symbol (short name) of the token type (mandatory)")
	cmd.Flags().String(cmdFlagName, "", "full name of the token type (optional)")
	cmd.Flags().String(cmdFlagIconFile, "", "icon file name for the token type (optional)")
	if err := cmd.MarkFlagRequired(cmdFlagSymbol); err != nil {
		panic(err)
	}

	setHexFlag(cmd, cmdFlagParentType, nil, "unit identifier of a parent type in hexadecimal format")
	cmd.Flags().StringSlice(cmdFlagSybTypeClauseInput, nil, "input to satisfy the parent type creation clause (mandatory with --parent-type)")
	cmd.MarkFlagsRequiredTogether(cmdFlagParentType, cmdFlagSybTypeClauseInput)
	cmd.Flags().String(cmdFlagSybTypeClause, predicateTrue, "predicate to control sub typing, values <true|false|ptpkh>")
	cmd.Flags().String(cmdFlagMintClause, predicatePtpkh, "predicate to control minting of this type, values <true|false|ptpkh>")
	cmd.Flags().String(cmdFlagInheritBearerClause, predicateTrue, "predicate that will be inherited by subtypes into their bearer clauses, values <true|false|ptpkh>")
	return cmd
}

func tokenCmdNewTypeFungible(config *types.WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fungible",
		Short: "create new fungible token type",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdNewTypeFungible(cmd, config)
		},
	}
	cmd.Flags().Uint32(cmdFlagDecimals, 8, "token decimal")
	setHexFlag(cmd, cmdFlagType, nil, "type unit identifier")
	_ = cmd.Flags().MarkHidden(cmdFlagType)
	return cmd
}

func execTokenCmdNewTypeFungible(cmd *cobra.Command, config *types.WalletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	typeId, err := getHexFlag(cmd, cmdFlagType)
	if err != nil {
		return err
	}
	symbol, err := cmd.Flags().GetString(cmdFlagSymbol)
	if err != nil {
		return err
	}
	name, err := cmd.Flags().GetString(cmdFlagName)
	if err != nil {
		return err
	}
	iconFilePath, err := cmd.Flags().GetString(cmdFlagIconFile)
	if err != nil {
		return err
	}
	icon, err := readIconFile(iconFilePath)
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
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()
	am := tw.GetAccountManager()
	parentType, creationInputs, err := readParentTypeInfo(cmd, accountNumber, am)
	if err != nil {
		return err
	}
	subTypeCreationPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagSybTypeClause, accountNumber, am)
	if err != nil {
		return err
	}
	mintTokenPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagMintClause, accountNumber, am)
	if err != nil {
		return err
	}
	invariantPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagInheritBearerClause, accountNumber, am)
	if err != nil {
		return err
	}
	a := tokenswallet.CreateFungibleTokenTypeAttributes{
		Symbol:                   symbol,
		Name:                     name,
		Icon:                     icon,
		DecimalPlaces:            decimals,
		ParentTypeId:             parentType,
		SubTypeCreationPredicate: subTypeCreationPredicate,
		TokenCreationPredicate:   mintTokenPredicate,
		InvariantPredicate:       invariantPredicate,
	}
	result, err := tw.NewFungibleType(cmd.Context(), accountNumber, a, typeId, creationInputs)
	if err != nil {
		return err
	}
	config.Base.ConsoleWriter.Println(fmt.Sprintf("Sent request for new fungible token type with id=%s", result.TokenTypeID))
	if result.FeeSum > 0 {
		config.Base.ConsoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", util.AmountToString(result.FeeSum, 8)))
	}
	return nil
}

func tokenCmdNewTypeNonFungible(config *types.WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "non-fungible",
		Short: "create new non-fungible token type",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdNewTypeNonFungible(cmd, config)
		},
	}
	setHexFlag(cmd, cmdFlagType, nil, "type unit identifier")
	_ = cmd.Flags().MarkHidden(cmdFlagType)
	cmd.Flags().String(cmdFlagTokenDataUpdateClause, predicateTrue, "data update predicate, values <true|false|ptpkh>")
	return cmd
}

func execTokenCmdNewTypeNonFungible(cmd *cobra.Command, config *types.WalletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	typeId, err := getHexFlag(cmd, cmdFlagType)
	if err != nil {
		return err
	}
	symbol, err := cmd.Flags().GetString(cmdFlagSymbol)
	if err != nil {
		return err
	}
	name, err := cmd.Flags().GetString(cmdFlagName)
	if err != nil {
		return err
	}
	iconFilePath, err := cmd.Flags().GetString(cmdFlagIconFile)
	if err != nil {
		return err
	}
	icon, err := readIconFile(iconFilePath)
	if err != nil {
		return err
	}
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()
	am := tw.GetAccountManager()
	parentType, creationInputs, err := readParentTypeInfo(cmd, accountNumber, am)
	if err != nil {
		return err
	}
	subTypeCreationPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagSybTypeClause, accountNumber, am)
	if err != nil {
		return err
	}
	mintTokenPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagMintClause, accountNumber, am)
	if err != nil {
		return err
	}
	dataUpdatePredicate, err := parsePredicateClauseCmd(cmd, cmdFlagTokenDataUpdateClause, accountNumber, am)
	if err != nil {
		return err
	}
	invariantPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagInheritBearerClause, accountNumber, am)
	if err != nil {
		return err
	}
	a := tokenswallet.CreateNonFungibleTokenTypeAttributes{
		Symbol:                   symbol,
		Name:                     name,
		Icon:                     icon,
		ParentTypeId:             parentType,
		SubTypeCreationPredicate: subTypeCreationPredicate,
		TokenCreationPredicate:   mintTokenPredicate,
		InvariantPredicate:       invariantPredicate,
		DataUpdatePredicate:      dataUpdatePredicate,
	}
	result, err := tw.NewNonFungibleType(cmd.Context(), accountNumber, a, typeId, creationInputs)
	if err != nil {
		return err
	}
	config.Base.ConsoleWriter.Println(fmt.Sprintf("Sent request for new NFT type with id=%s", result.TokenTypeID))
	if result.FeeSum > 0 {
		config.Base.ConsoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", util.AmountToString(result.FeeSum, 8)))
	}
	return nil
}

func tokenCmdNewToken(config *types.WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new",
		Short: "mint new token",
	}
	cmd.AddCommand(addCommonAccountFlags(tokenCmdNewTokenFungible(config)))
	cmd.AddCommand(addCommonAccountFlags(tokenCmdNewTokenNonFungible(config)))
	return cmd
}

func tokenCmdNewTokenFungible(config *types.WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fungible",
		Short: "mint new fungible token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdNewTokenFungible(cmd, config)
		},
	}
	cmd.Flags().String(cmdFlagBearerClause, predicatePtpkh, "predicate that defines the ownership of this fungible token, values <true|false|ptpkh>")
	cmd.Flags().String(cmdFlagAmount, "", "amount, must be bigger than 0 and is interpreted according to token type precision (decimals)")
	err := cmd.MarkFlagRequired(cmdFlagAmount)
	if err != nil {
		return nil
	}
	setHexFlag(cmd, cmdFlagType, nil, "type unit identifier")
	err = cmd.MarkFlagRequired(cmdFlagType)
	if err != nil {
		return nil
	}
	cmd.Flags().StringSlice(cmdFlagMintClauseInput, []string{predicatePtpkh}, "input to satisfy the type's minting clause")
	return cmd
}

func execTokenCmdNewTokenFungible(cmd *cobra.Command, config *types.WalletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	am := tw.GetAccountManager()
	defer tw.Shutdown()

	amountStr, err := cmd.Flags().GetString(cmdFlagAmount)
	if err != nil {
		return err
	}
	typeId, err := getHexFlag(cmd, cmdFlagType)
	if err != nil {
		return err
	}
	ci, err := readPredicateInput(cmd, cmdFlagMintClauseInput, accountNumber, am)
	if err != nil {
		return err
	}
	tt, err := tw.GetTokenType(cmd.Context(), typeId)
	if err != nil {
		return err
	}
	// convert amount from string to uint64
	amount, err := util.StringToAmount(amountStr, tt.DecimalPlaces)
	if err != nil {
		return err
	}
	if amount == 0 {
		return fmt.Errorf("invalid parameter \"%s\" for \"--amount\": 0 is not valid amount", amountStr)
	}
	bearerPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagBearerClause, accountNumber, am)
	if err != nil {
		return err
	}
	result, err := tw.NewFungibleToken(cmd.Context(), accountNumber, typeId, amount, bearerPredicate, ci)
	if err != nil {
		return err
	}

	config.Base.ConsoleWriter.Println(fmt.Sprintf("Sent request for new fungible token with id=%s", result.TokenID))
	if result.FeeSum > 0 {
		config.Base.ConsoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", util.AmountToString(result.FeeSum, 8)))
	}
	return nil
}

func tokenCmdNewTokenNonFungible(config *types.WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "non-fungible",
		Short: "mint new non-fungible token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdNewTokenNonFungible(cmd, config)
		},
	}
	addDataFlags(cmd)
	cmd.Flags().String(cmdFlagBearerClause, predicatePtpkh, "predicate that defines the ownership of this non-fungible token, values <true|false|ptpkh>")
	setHexFlag(cmd, cmdFlagType, nil, "type unit identifier")
	err := cmd.MarkFlagRequired(cmdFlagType)
	if err != nil {
		return nil
	}
	cmd.Flags().String(cmdFlagName, "", "name of the token (optional)")
	cmd.Flags().String(cmdFlagTokenURI, "", "URI to associated resource, ie. jpg file on IPFS")
	cmd.Flags().String(cmdFlagTokenDataUpdateClause, predicateTrue, "data update predicate, values <true|false|ptpkh>")
	cmd.Flags().StringSlice(cmdFlagMintClauseInput, []string{predicatePtpkh}, "input to satisfy the type's minting clause")
	setHexFlag(cmd, cmdFlagTokenId, nil, "token identifier")
	_ = cmd.Flags().MarkHidden(cmdFlagTokenId)
	return cmd
}

func execTokenCmdNewTokenNonFungible(cmd *cobra.Command, config *types.WalletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	typeId, err := getHexFlag(cmd, cmdFlagType)
	if err != nil {
		return err
	}
	tokenID, err := getHexFlag(cmd, cmdFlagTokenId)
	if err != nil {
		return err
	}
	name, err := cmd.Flags().GetString(cmdFlagName)
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
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()
	am := tw.GetAccountManager()
	ci, err := readPredicateInput(cmd, cmdFlagMintClauseInput, accountNumber, am)
	if err != nil {
		return err
	}
	bearerPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagBearerClause, accountNumber, am)
	if err != nil {
		return err
	}
	dataUpdatePredicate, err := parsePredicateClauseCmd(cmd, cmdFlagTokenDataUpdateClause, accountNumber, am)
	if err != nil {
		return err
	}
	a := tokenswallet.MintNonFungibleTokenAttributes{
		Bearer:              bearerPredicate,
		Name:                name,
		NftType:             typeId,
		Uri:                 uri,
		Data:                data,
		DataUpdatePredicate: dataUpdatePredicate,
	}
	result, err := tw.NewNFT(cmd.Context(), accountNumber, a, tokenID, ci)
	if err != nil {
		return err
	}

	config.Base.ConsoleWriter.Println(fmt.Sprintf("Sent request for new non-fungible token with id=%s", result.TokenID))
	if result.FeeSum > 0 {
		config.Base.ConsoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", util.AmountToString(result.FeeSum, 8)))
	}
	return nil
}

func tokenCmdSend(config *types.WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send",
		Short: "send a token",
	}
	cmd.AddCommand(tokenCmdSendFungible(config))
	cmd.AddCommand(tokenCmdSendNonFungible(config))
	return cmd
}

func tokenCmdSendFungible(config *types.WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fungible",
		Short: "send fungible token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdSendFungible(cmd, config)
		},
	}
	cmd.Flags().StringSlice(cmdFlagInheritBearerClauseInput, []string{predicateTrue}, "input to satisfy the type's invariant clause")
	cmd.Flags().String(cmdFlagAmount, "", "amount, must be bigger than 0 and is interpreted according to token type precision (decimals)")
	err := cmd.MarkFlagRequired(cmdFlagAmount)
	if err != nil {
		return nil
	}
	setHexFlag(cmd, cmdFlagType, nil, "type unit identifier")
	err = cmd.MarkFlagRequired(cmdFlagType)
	if err != nil {
		return nil
	}
	cmd.Flags().StringP(args.AddressCmdName, "a", "", "compressed secp256k1 public key of the receiver in hexadecimal format, must start with 0x and be 68 characters in length")
	err = cmd.MarkFlagRequired(args.AddressCmdName)
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
		pk, ok := cliaccount.PubKeyHexToBytes(pubKeyHex)
		if !ok {
			return nil, fmt.Errorf("address in not in valid format: %s", pubKeyHex)
		}
		pubKey = pk
	}
	return pubKey, nil
}

func execTokenCmdSendFungible(cmd *cobra.Command, config *types.WalletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
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

	pubKey, err := getPubKeyBytes(cmd, args.AddressCmdName)
	if err != nil {
		return err
	}

	ib, err := readPredicateInput(cmd, cmdFlagInheritBearerClauseInput, accountNumber, tw.GetAccountManager())
	if err != nil {
		return err
	}

	// get token type and convert amount string
	tt, err := tw.GetTokenType(cmd.Context(), typeId)
	if err != nil {
		return err
	}
	// convert amount from string to uint64
	targetValue, err := util.StringToAmount(amountStr, tt.DecimalPlaces)
	if err != nil {
		return err
	}
	if targetValue == 0 {
		return fmt.Errorf("invalid parameter \"%s\" for \"--amount\": 0 is not valid amount", amountStr)
	}
	result, err := tw.SendFungible(cmd.Context(), accountNumber, typeId, targetValue, pubKey, ib)
	if err != nil {
		return err
	}
	if result.FeeSum > 0 {
		config.Base.ConsoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", util.AmountToString(result.FeeSum, 8)))
	}
	return err
}

func tokenCmdSendNonFungible(config *types.WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "non-fungible",
		Short: "transfer non-fungible token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdSendNonFungible(cmd, config)
		},
	}
	cmd.Flags().StringSlice(cmdFlagInheritBearerClauseInput, []string{predicateTrue}, "input to satisfy the type's invariant clause")
	setHexFlag(cmd, cmdFlagTokenId, nil, "token identifier")
	err := cmd.MarkFlagRequired(cmdFlagTokenId)
	if err != nil {
		return nil
	}
	cmd.Flags().StringP(args.AddressCmdName, "a", "", "compressed secp256k1 public key of the receiver in hexadecimal format, must start with 0x and be 68 characters in length")
	err = cmd.MarkFlagRequired(args.AddressCmdName)
	if err != nil {
		return nil
	}
	return addCommonAccountFlags(cmd)
}

func execTokenCmdSendNonFungible(cmd *cobra.Command, config *types.WalletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	tokenID, err := getHexFlag(cmd, cmdFlagTokenId)
	if err != nil {
		return err
	}

	pubKey, err := getPubKeyBytes(cmd, args.AddressCmdName)
	if err != nil {
		return err
	}

	ib, err := readPredicateInput(cmd, cmdFlagInheritBearerClauseInput, accountNumber, tw.GetAccountManager())
	if err != nil {
		return err
	}

	result, err := tw.TransferNFT(cmd.Context(), accountNumber, tokenID, pubKey, ib)
	if err != nil {
		return err
	}
	if result.FeeSum > 0 {
		config.Base.ConsoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", util.AmountToString(result.FeeSum, 8)))
	}
	return err
}

func tokenCmdDC(config *types.WalletConfig) *cobra.Command {
	var accountNumber uint64

	cmd := &cobra.Command{
		Use:   "collect-dust",
		Short: "join fungible tokens into one unit",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdDC(cmd, config, &accountNumber)
		},
	}

	cmd.Flags().Uint64VarP(&accountNumber, args.KeyCmdName, "k", 0, "which key to use for dust collection, 0 for all tokens from all accounts")
	cmd.Flags().StringSlice(cmdFlagType, nil, "type unit identifier (hex)")
	cmd.Flags().StringSlice(cmdFlagInheritBearerClauseInput, []string{predicateTrue}, "input to satisfy the type's invariant clause")

	return cmd
}

func execTokenCmdDC(cmd *cobra.Command, config *types.WalletConfig, accountNumber *uint64) error {
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	typeIDStrs, err := cmd.Flags().GetStringSlice(cmdFlagType)
	if err != nil {
		return err
	}
	var typez []tokenswallet.TokenTypeID
	for _, tokenType := range typeIDStrs {
		typeBytes, err := tokenswallet.DecodeHexOrEmpty(tokenType)
		if err != nil {
			return err
		}
		if len(typeBytes) > 0 {
			typez = append(typez, typeBytes)
		}
	}
	ib, err := readPredicateInput(cmd, cmdFlagInheritBearerClauseInput, *accountNumber, tw.GetAccountManager())
	if err != nil {
		return err
	}

	results, err := tw.CollectDust(cmd.Context(), *accountNumber, typez, ib)
	if err != nil {
		return err
	}
	for _, result := range results {
		if result.FeeSum > 0 {
			config.Base.ConsoleWriter.Println(fmt.Sprintf("Paid %s fees for dust collection on Account number %d.", util.AmountToString(result.FeeSum, 8), result.AccountNumber))
		}
	}
	return err
}

func tokenCmdUpdateNFTData(config *types.WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "update the data field on a non-fungible token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdUpdateNFTData(cmd, config)
		},
	}
	setHexFlag(cmd, cmdFlagTokenId, nil, "token identifier")
	if err := cmd.MarkFlagRequired(cmdFlagTokenId); err != nil {
		panic(err)
	}

	addDataFlags(cmd)
	cmd.Flags().StringSlice(cmdFlagTokenDataUpdateClauseInput, []string{predicateTrue, predicateTrue}, "input to satisfy the data-update clauses")
	return addCommonAccountFlags(cmd)
}

func execTokenCmdUpdateNFTData(cmd *cobra.Command, config *types.WalletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}

	tokenID, err := getHexFlag(cmd, cmdFlagTokenId)
	if err != nil {
		return err
	}

	data, err := readNFTData(cmd, true)
	if err != nil {
		return err
	}

	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	du, err := readPredicateInput(cmd, cmdFlagTokenDataUpdateClauseInput, accountNumber, tw.GetAccountManager())
	if err != nil {
		return err
	}

	result, err := tw.UpdateNFTData(cmd.Context(), accountNumber, tokenID, data, du)
	if err != nil {
		return err
	}
	if result.FeeSum > 0 {
		config.Base.ConsoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", util.AmountToString(result.FeeSum, 8)))
	}
	return err
}

func tokenCmdList(config *types.WalletConfig, runner runTokenListCmd) *cobra.Command {
	var accountNumber uint64
	cmd := &cobra.Command{
		Use:   "list",
		Short: "lists all available tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, &accountNumber, tokenswallet.Any)
		},
	}
	// add persistent password flags
	cmd.PersistentFlags().BoolP(args.PasswordPromptCmdName, "p", false, args.PasswordPromptUsage)
	cmd.PersistentFlags().String(args.PasswordArgCmdName, "", args.PasswordArgUsage)

	cmd.Flags().Bool(cmdFlagWithAll, false, "Show all available fields for each token")
	cmd.Flags().Bool(cmdFlagWithTypeName, false, "Show type name field")
	cmd.Flags().Bool(cmdFlagWithTokenURI, false, "Show non-fungible token URI field")
	cmd.Flags().Bool(cmdFlagWithTokenData, false, "Show non-fungible token data field")

	// add sub commands
	cmd.AddCommand(tokenCmdListFungible(config, runner, &accountNumber))
	cmd.AddCommand(tokenCmdListNonFungible(config, runner, &accountNumber))
	cmd.PersistentFlags().Uint64VarP(&accountNumber, args.KeyCmdName, "k", 0, "which key to use for sending the transaction, 0 for all tokens from all accounts")
	return cmd
}

func tokenCmdListFungible(config *types.WalletConfig, runner runTokenListCmd, accountNumber *uint64) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fungible",
		Short: "lists fungible tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, accountNumber, tokenswallet.Fungible)
		},
	}

	cmd.Flags().Bool(cmdFlagWithAll, false, "Show all available fields for each token")
	cmd.Flags().Bool(cmdFlagWithTypeName, false, "Show type name field")

	return cmd
}

func tokenCmdListNonFungible(config *types.WalletConfig, runner runTokenListCmd, accountNumber *uint64) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "non-fungible",
		Short: "lists non-fungible tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, accountNumber, tokenswallet.NonFungible)
		},
	}

	cmd.Flags().Bool(cmdFlagWithAll, false, "Show all available fields for each token")
	cmd.Flags().Bool(cmdFlagWithTypeName, false, "Show type name field")
	cmd.Flags().Bool(cmdFlagWithTokenURI, false, "Show token URI field")
	cmd.Flags().Bool(cmdFlagWithTokenData, false, "Show token data field")

	return cmd
}

func execTokenCmdList(cmd *cobra.Command, config *types.WalletConfig, accountNumber *uint64, kind tokenswallet.Kind) error {
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	res, err := tw.ListTokens(cmd.Context(), kind, *accountNumber)
	if err != nil {
		return err
	}

	withAll, err := cmd.Flags().GetBool(cmdFlagWithAll)
	if err != nil {
		return err
	}

	withTypeName, withTokenURI, withTokenData := false, false, false
	if !withAll {
		withTypeName, err = cmd.Flags().GetBool(cmdFlagWithTypeName)
		if err != nil {
			return err
		}
		if kind == tokenswallet.Any || kind == tokenswallet.NonFungible {
			withTokenURI, err = cmd.Flags().GetBool(cmdFlagWithTokenURI)
			if err != nil {
				return err
			}
			withTokenData, err = cmd.Flags().GetBool(cmdFlagWithTokenData)
			if err != nil {
				return err
			}
		}
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
		config.Base.ConsoleWriter.Println(ownerKey)
		sort.Slice(toks, func(i, j int) bool {
			// Fungible, then Non-fungible
			return toks[i].Kind < toks[j].Kind
		})
		for _, tok := range toks {
			atLeastOneFound = true

			var typeName, nftURI, nftData string
			if withAll || withTypeName {
				typeName = fmt.Sprintf(", token-type-name='%s'", tok.TypeName)
			}
			if withAll || withTokenURI {
				nftURI = fmt.Sprintf(", URI='%s'", tok.NftURI)
			}
			if withAll || withTokenData {
				nftData = fmt.Sprintf(", data='%X'", tok.NftData)
			}
			kind := fmt.Sprintf(" (%v)", tok.Kind)

			if tok.Kind == tokenswallet.Fungible {
				amount := util.AmountToString(tok.Amount, tok.Decimals)
				config.Base.ConsoleWriter.Println(fmt.Sprintf("ID='%s', symbol='%s', amount='%v', token-type='%s', locked='%s'",
					tok.ID, tok.Symbol, amount, tok.TypeID, wallet.LockReason(tok.Locked).String()) + typeName + kind)
			} else {
				config.Base.ConsoleWriter.Println(fmt.Sprintf("ID='%s', symbol='%s', name='%s', token-type='%s', locked='%s'",
					tok.ID, tok.Symbol, tok.NftName, tok.TypeID, wallet.LockReason(tok.Locked).String()) + typeName + nftURI + nftData + kind)
			}
		}
	}
	if !atLeastOneFound {
		config.Base.ConsoleWriter.Println("No tokens")
	}
	return nil
}

func tokenCmdListTypes(config *types.WalletConfig, runner runTokenListTypesCmd) *cobra.Command {
	var accountNumber uint64
	cmd := &cobra.Command{
		Use:   "list-types",
		Short: "lists token types",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, &accountNumber, tokenswallet.Any)
		},
	}
	// add password flags as persistent
	cmd.PersistentFlags().BoolP(args.PasswordPromptCmdName, "p", false, args.PasswordPromptUsage)
	cmd.PersistentFlags().String(args.PasswordArgCmdName, "", args.PasswordArgUsage)
	cmd.PersistentFlags().Uint64VarP(&accountNumber, args.KeyCmdName, "k", 0, "show types created from a specific key, 0 for all keys")
	// add optional sub-commands to filter fungible and non-fungible types
	cmd.AddCommand(&cobra.Command{
		Use:   "fungible",
		Short: "lists fungible types",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, &accountNumber, tokenswallet.Fungible)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "non-fungible",
		Short: "lists non-fungible types",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, &accountNumber, tokenswallet.NonFungible)
		},
	})
	return cmd
}

func execTokenCmdListTypes(cmd *cobra.Command, config *types.WalletConfig, accountNumber *uint64, kind tokenswallet.Kind) error {
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	res, err := tw.ListTokenTypes(cmd.Context(), *accountNumber, kind)
	if err != nil {
		return err
	}
	for _, tok := range res {
		name := ""
		if tok.Name != "" {
			name = fmt.Sprintf(", name=%s", tok.Name)
		}
		kind := fmt.Sprintf(" (%v)", tok.Kind)
		config.Base.ConsoleWriter.Println(fmt.Sprintf("ID=%s, symbol=%s", tok.ID, tok.Symbol) + name + kind)
	}
	return nil
}

func tokenCmdLock(config *types.WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lock",
		Short: "locks a fungible or non-fungible token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdLock(cmd, config)
		},
	}
	setHexFlag(cmd, cmdFlagTokenId, nil, "token identifier")
	if err := cmd.MarkFlagRequired(cmdFlagTokenId); err != nil {
		panic(err)
	}
	cmd.Flags().StringSlice(cmdFlagInheritBearerClauseInput, []string{predicateTrue}, "input to satisfy the type's invariant clause")
	return addCommonAccountFlags(cmd)
}

func execTokenCmdLock(cmd *cobra.Command, config *types.WalletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	tokenID, err := getHexFlag(cmd, cmdFlagTokenId)
	if err != nil {
		return err
	}

	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	ib, err := readPredicateInput(cmd, cmdFlagInheritBearerClauseInput, accountNumber, tw.GetAccountManager())
	if err != nil {
		return err
	}

	result, err := tw.LockToken(cmd.Context(), accountNumber, tokenID, ib)
	if err != nil {
		return err
	}
	if result.FeeSum > 0 {
		config.Base.ConsoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", util.AmountToString(result.FeeSum, 8)))
	}
	return nil
}

func tokenCmdUnlock(config *types.WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unlock",
		Short: "unlocks a fungible or non-fungible token",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenCmdUnlock(cmd, config)
		},
	}
	setHexFlag(cmd, cmdFlagTokenId, nil, "token identifier")
	if err := cmd.MarkFlagRequired(cmdFlagTokenId); err != nil {
		panic(err)
	}
	cmd.Flags().StringSlice(cmdFlagInheritBearerClauseInput, []string{predicateTrue}, "input to satisfy the type's invariant clause")
	return addCommonAccountFlags(cmd)
}

func execTokenCmdUnlock(cmd *cobra.Command, config *types.WalletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	tokenID, err := getHexFlag(cmd, cmdFlagTokenId)
	if err != nil {
		return err
	}

	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Shutdown()

	ib, err := readPredicateInput(cmd, cmdFlagInheritBearerClauseInput, accountNumber, tw.GetAccountManager())
	if err != nil {
		return err
	}

	result, err := tw.UnlockToken(cmd.Context(), accountNumber, tokenID, ib)
	if err != nil {
		return err
	}
	if result.FeeSum > 0 {
		config.Base.ConsoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", util.AmountToString(result.FeeSum, 8)))
	}
	return err
}

func initTokensWallet(cmd *cobra.Command, config *types.WalletConfig) (*tokenswallet.Wallet, error) {
	rpcUrl, err := cmd.Flags().GetString(args.RpcUrl)
	if err != nil {
		return nil, err
	}
	am, err := cliaccount.LoadExistingAccountManager(config)
	if err != nil {
		return nil, err
	}
	confirmTxStr, err := cmd.Flags().GetString(args.WaitForConfCmdName)
	if err != nil {
		return nil, err
	}
	confirmTx, err := strconv.ParseBool(confirmTxStr)
	if err != nil {
		return nil, err
	}
	rpcClient, err := rpc.DialContext(cmd.Context(), args.BuildRpcUrl(rpcUrl))
	if err != nil {
		return nil, fmt.Errorf("failed to dial rpc client: %w", err)
	}
	tokensClient := rpc.NewTokensClient(rpcClient)
	// TODO add info endpoint to rpc client?
	//infoResponse, err := backendClient.GetInfo(cmd.Context())
	//if err != nil {
	//	return nil, err
	//}
	//tokensTypeVar := types.TokensType
	//if !strings.HasPrefix(infoResponse.Name, tokensTypeVar.String()) {
	//	return nil, errors.New("invalid wallet backend API URL provided for tokens partition")
	//}
	return tokenswallet.New(tokens.DefaultSystemIdentifier, tokensClient, am, confirmTx, nil, config.Base.Observe.Logger())
}

func readParentTypeInfo(cmd *cobra.Command, keyNr uint64, am account.Manager) (tokenswallet.TokenTypeID, []*tokenswallet.PredicateInput, error) {
	parentType, err := getHexFlag(cmd, cmdFlagParentType)
	if err != nil {
		return nil, nil, err
	}

	if len(parentType) == 0 {
		return nil, []*tokenswallet.PredicateInput{}, nil
	}

	creationInputs, err := readPredicateInput(cmd, cmdFlagSybTypeClauseInput, keyNr, am)
	if err != nil {
		return nil, nil, err
	}

	return parentType, creationInputs, nil
}

func readPredicateInput(cmd *cobra.Command, flag string, keyNr uint64, am account.Manager) ([]*tokenswallet.PredicateInput, error) {
	creationInputStrs, err := cmd.Flags().GetStringSlice(flag)
	if err != nil {
		return nil, err
	}
	if len(creationInputStrs) == 0 {
		return []*tokenswallet.PredicateInput{{Argument: nil}}, nil
	}
	creationInputs, err := tokenswallet.ParsePredicates(creationInputStrs, keyNr, am)
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
func parsePredicateClauseCmd(cmd *cobra.Command, flag string, keyNr uint64, am account.Manager) ([]byte, error) {
	clause, err := cmd.Flags().GetString(flag)
	if err != nil {
		return nil, err
	}
	return tokenswallet.ParsePredicateClause(clause, keyNr, am)
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
		data, err = readFile(dataFilePath, cmdFlagTokenDataFile, maxBinaryFile64KiB)
		if err != nil {
			return nil, err
		}
	}
	return data, nil
}

// getHexFlag returns the custom flag value that was set by setHexFlag
func getHexFlag(cmd *cobra.Command, name string) ([]byte, error) {
	return *cmd.Flag(name).Value.(*types.BytesHex), nil
}

// setHexFlag adds custom hex value flag (allows 0x prefix) to command flagset
func setHexFlag(cmd *cobra.Command, name string, value []byte, usage string) {
	var hexFlag types.BytesHex = value
	cmd.Flags().Var(&hexFlag, name, usage)
}

func readIconFile(iconFilePath string) (*tokenswallet.Icon, error) {
	if len(iconFilePath) == 0 {
		return nil, nil
	}
	icon := &tokenswallet.Icon{}

	ext := filepath.Ext(iconFilePath)
	if len(ext) == 0 {
		return nil, fmt.Errorf("%s read error: missing file extension", cmdFlagIconFile)
	}

	mime.AddExtensionType(iconFileExtSvgz, iconFileExtSvgzType)
	icon.Type = mime.TypeByExtension(ext)
	if len(icon.Type) == 0 {
		return nil, fmt.Errorf("%s read error: could not determine MIME type from file extension", cmdFlagIconFile)
	}

	data, err := readFile(iconFilePath, cmdFlagIconFile, maxBinaryFile64KiB)
	if err != nil {
		return nil, err
	}
	icon.Data = data
	return icon, nil
}

func readFile(path string, flag string, sizeLimit int64) ([]byte, error) {
	size, err := getFileSize(path)
	if err != nil {
		return nil, fmt.Errorf("%s read error: %w", flag, err)
	}
	if size > sizeLimit {
		return nil, fmt.Errorf("%s read error: file size over %vKiB limit", flag, sizeLimit/1024)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s read error: %w", flag, err)
	}
	return data, nil
}

func getFileSize(filepath string) (int64, error) {
	fi, err := os.Stat(filepath)
	if err != nil {
		return 0, err
	}
	// get the size
	return fi.Size(), nil
}
