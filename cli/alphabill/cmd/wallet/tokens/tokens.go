package tokens

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	basetypes "github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	cliaccount "github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/util/account"
	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/wallet/args"
	"github.com/alphabill-org/alphabill-wallet/client"
	sdktypes "github.com/alphabill-org/alphabill-wallet/client/types"
	"github.com/alphabill-org/alphabill-wallet/util"
	"github.com/alphabill-org/alphabill-wallet/wallet"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
	tokenswallet "github.com/alphabill-org/alphabill-wallet/wallet/tokens"
	"github.com/spf13/cobra"
)

const (
	cmdFlagSymbol                            = "symbol"
	cmdFlagName                              = "name"
	cmdFlagIconFile                          = "icon-file"
	cmdFlagDecimals                          = "decimals"
	cmdFlagParentType                        = "parent-type"
	cmdFlagSybTypeClause                     = "subtype-clause"
	cmdFlagSybTypeClauseInput                = "subtype-input"
	cmdFlagMintClause                        = "mint-clause"
	cmdFlagBearerClause                      = "bearer-clause"
	cmdFlagBearerClauseInput                 = "bearer-clause-input"
	cmdFlagMintClauseInput                   = "mint-input"
	cmdFlagInheritBearerClause               = "inherit-bearer-clause"
	cmdFlagInheritBearerClauseInput          = "inherit-bearer-input"
	cmdFlagTokenDataUpdateClause             = "data-update-clause"
	cmdFlagTokenDataUpdateClauseInput        = "data-update-input"
	cmdFlagInheritTokenDataUpdateClauseInput = "inherit-data-update-input"
	cmdFlagAmount                            = "amount"
	cmdFlagType                              = "type"
	cmdFlagTokenID                           = "token-identifier"
	cmdFlagTokenURI                          = "token-uri"
	cmdFlagTokenData                         = "data"
	cmdFlagTokenDataFile                     = "data-file"

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
	allAccounts        = 0

	helpPredicateValues = `Valid values are either one of the predicate template name [ true | false | ptpkh | ptpkh:n | ptpkh:0x<hex-string> ] ` +
		`or @<filename> to load predicate from given file.`
	helpPredicateArgument = "Valid values are:\n[ true | false | empty ] - these will esentially mean \"no argument\"\n" +
		"[ ptpkh | ptpkh:n ] - creates argument for the ptpkh predicate template using either default account key or account n key respectively\n" +
		"@<filename> - load argument from file, the file content will be used as-is.\n"
)

const (
	Any Kind = 1 << iota
	Fungible
	NonFungible
)

type (
	Kind byte

	runTokenListTypesCmd func(cmd *cobra.Command, config *types.WalletConfig, accountNumber *uint64, kind Kind) error
	runTokenListCmd      func(cmd *cobra.Command, config *types.WalletConfig, accountNumber *uint64, kind Kind) error
	runTokenCmdDC        func(cmd *cobra.Command, config *types.WalletConfig, accountNumber *uint64) error
)

func NewTokenCmd(config *types.WalletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "create and manage fungible and non-fungible tokens",
	}
	cmd.AddCommand(tokenCmdNewType(config))
	cmd.AddCommand(tokenCmdNewToken(config))
	cmd.AddCommand(tokenCmdUpdateNFTData(config))
	cmd.AddCommand(tokenCmdSend(config))
	cmd.AddCommand(tokenCmdDC(config, execTokenCmdDC))
	cmd.AddCommand(tokenCmdList(config, execTokenCmdList))
	cmd.AddCommand(tokenCmdListTypes(config, execTokenCmdListTypes))
	cmd.AddCommand(tokenCmdLock(config))
	cmd.AddCommand(tokenCmdUnlock(config))
	cmd.PersistentFlags().StringP(args.RpcUrl, "r", args.DefaultTokensRpcUrl, "rpc node url")
	args.AddWaitForProofFlags(cmd, cmd.PersistentFlags())
	args.AddMaxFeeFlag(cmd, cmd.PersistentFlags())
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
	cmd.Flags().String(cmdFlagSybTypeClause, predicateTrue, "predicate to control sub typing. "+helpPredicateValues)
	cmd.Flags().String(cmdFlagMintClause, predicatePtpkh, "predicate to control minting of this type. "+helpPredicateValues)
	cmd.Flags().String(cmdFlagInheritBearerClause, predicateTrue, "predicate that will be inherited by subtypes into their bearer clauses. "+helpPredicateValues)
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
	typeID, err := getHexFlag(cmd, cmdFlagType)
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
	defer tw.Close()
	am := tw.GetAccountManager()
	parentType, creationInputs, err := readParentTypeInfo(cmd, accountNumber, am)
	if err != nil {
		return err
	}
	subTypeCreationPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagSybTypeClause, accountNumber, am)
	if err != nil {
		return err
	}
	tokenMintingPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagMintClause, accountNumber, am)
	if err != nil {
		return err
	}
	tokenTypeOwnerPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagInheritBearerClause, accountNumber, am)
	if err != nil {
		return err
	}
	tt := &sdktypes.FungibleTokenType{
		NetworkID:                tw.NetworkID(),
		PartitionID:              tw.PartitionID(),
		ID:                       typeID,
		ParentTypeID:             parentType,
		Symbol:                   symbol,
		Name:                     name,
		Icon:                     icon,
		SubTypeCreationPredicate: subTypeCreationPredicate,
		TokenMintingPredicate:    tokenMintingPredicate,
		TokenTypeOwnerPredicate:  tokenTypeOwnerPredicate,
		DecimalPlaces:            decimals,
	}
	result, err := tw.NewFungibleType(cmd.Context(), accountNumber, tt, creationInputs)
	if err != nil {
		return err
	}
	config.Base.ConsoleWriter.Println(fmt.Sprintf("Sent request for new fungible token type with id=%s", result.GetUnit()))
	if result.FeeSum > 0 {
		config.Base.ConsoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", util.AmountToString(result.FeeSum, 8)))
	}
	if err := saveTxProofs(cmd, result.GetProofs(), config.Base.ConsoleWriter); err != nil {
		return fmt.Errorf("saving transaction proof(s): %w", err)
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
	cmd.Flags().String(cmdFlagTokenDataUpdateClause, predicateTrue, "data update predicate. "+helpPredicateValues)
	return cmd
}

func execTokenCmdNewTypeNonFungible(cmd *cobra.Command, config *types.WalletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	typeID, err := getHexFlag(cmd, cmdFlagType)
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
	defer tw.Close()
	am := tw.GetAccountManager()
	parentType, creationInputs, err := readParentTypeInfo(cmd, accountNumber, am)
	if err != nil {
		return err
	}
	subTypeCreationPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagSybTypeClause, accountNumber, am)
	if err != nil {
		return err
	}
	tokenMintingPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagMintClause, accountNumber, am)
	if err != nil {
		return err
	}
	dataUpdatePredicate, err := parsePredicateClauseCmd(cmd, cmdFlagTokenDataUpdateClause, accountNumber, am)
	if err != nil {
		return err
	}
	tokenTypeOwnerPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagInheritBearerClause, accountNumber, am)
	if err != nil {
		return err
	}
	tt := &sdktypes.NonFungibleTokenType{
		NetworkID:                tw.NetworkID(),
		PartitionID:              tw.PartitionID(),
		ID:                       typeID,
		ParentTypeID:             parentType,
		Symbol:                   symbol,
		Name:                     name,
		Icon:                     icon,
		SubTypeCreationPredicate: subTypeCreationPredicate,
		TokenMintingPredicate:    tokenMintingPredicate,
		TokenTypeOwnerPredicate:  tokenTypeOwnerPredicate,
		DataUpdatePredicate:      dataUpdatePredicate,
	}
	result, err := tw.NewNonFungibleType(cmd.Context(), accountNumber, tt, creationInputs)
	if err != nil {
		return err
	}
	config.Base.ConsoleWriter.Println(fmt.Sprintf("Sent request for new NFT type with id=%s", result.GetUnit()))
	if result.FeeSum > 0 {
		config.Base.ConsoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", util.AmountToString(result.FeeSum, 8)))
	}
	if err := saveTxProofs(cmd, result.GetProofs(), config.Base.ConsoleWriter); err != nil {
		return fmt.Errorf("saving transaction proof(s): %w", err)
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
	cmd.Flags().String(cmdFlagBearerClause, predicatePtpkh, "predicate that defines the ownership of this fungible token. "+helpPredicateValues)
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
	cmd.Flags().String(cmdFlagMintClauseInput, predicatePtpkh, "input to satisfy the type's minting clause. "+helpPredicateArgument)
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
	defer tw.Close()

	amountStr, err := cmd.Flags().GetString(cmdFlagAmount)
	if err != nil {
		return err
	}
	typeID, err := getHexFlag(cmd, cmdFlagType)
	if err != nil {
		return err
	}
	mintPredicateInput, err := readSinglePredicateInput(cmd, cmdFlagMintClauseInput, accountNumber, am)
	if err != nil {
		return err
	}
	tt, err := tw.GetFungibleTokenType(cmd.Context(), typeID)
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
	ownerPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagBearerClause, accountNumber, am)
	if err != nil {
		return err
	}

	ft := &sdktypes.FungibleToken{
		NetworkID:      tw.NetworkID(),
		PartitionID:    tw.PartitionID(),
		TypeID:         typeID,
		OwnerPredicate: ownerPredicate,
		Amount:         amount,
	}
	result, err := tw.NewFungibleToken(cmd.Context(), accountNumber, ft, mintPredicateInput)
	if err != nil {
		return err
	}

	config.Base.ConsoleWriter.Println(fmt.Sprintf("Sent request for new fungible token with id=%s", result.GetUnit()))
	if result.FeeSum > 0 {
		config.Base.ConsoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", util.AmountToString(result.FeeSum, 8)))
	}
	if err := saveTxProofs(cmd, result.GetProofs(), config.Base.ConsoleWriter); err != nil {
		return fmt.Errorf("saving transaction proof(s): %w", err)
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
	cmd.Flags().String(cmdFlagBearerClause, predicatePtpkh, "predicate that defines the ownership of this non-fungible token. "+helpPredicateValues)
	setHexFlag(cmd, cmdFlagType, nil, "type unit identifier")
	err := cmd.MarkFlagRequired(cmdFlagType)
	if err != nil {
		return nil
	}
	cmd.Flags().String(cmdFlagName, "", "name of the token (optional)")
	cmd.Flags().String(cmdFlagTokenURI, "", "URI to associated resource, ie. jpg file on IPFS")
	cmd.Flags().String(cmdFlagTokenDataUpdateClause, predicateTrue, "data update predicate. "+helpPredicateValues)
	cmd.Flags().String(cmdFlagMintClauseInput, predicatePtpkh, "input to satisfy the type's minting clause. "+helpPredicateArgument)
	return cmd
}

func execTokenCmdNewTokenNonFungible(cmd *cobra.Command, config *types.WalletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	typeID, err := getHexFlag(cmd, cmdFlagType)
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
	defer tw.Close()
	am := tw.GetAccountManager()
	mintPredicateInput, err := readSinglePredicateInput(cmd, cmdFlagMintClauseInput, accountNumber, am)
	if err != nil {
		return err
	}
	ownerPredicate, err := parsePredicateClauseCmd(cmd, cmdFlagBearerClause, accountNumber, am)
	if err != nil {
		return err
	}
	dataUpdatePredicate, err := parsePredicateClauseCmd(cmd, cmdFlagTokenDataUpdateClause, accountNumber, am)
	if err != nil {
		return err
	}

	tt, err := tw.GetNonFungibleTokenType(cmd.Context(), typeID)
	if err != nil {
		return err
	}
	if tt == nil {
		return fmt.Errorf("non-fungible token type %s not found", typeID)
	}

	nft := &sdktypes.NonFungibleToken{
		NetworkID:           tw.NetworkID(),
		PartitionID:         tw.PartitionID(),
		TypeID:              typeID,
		OwnerPredicate:      ownerPredicate,
		Name:                name,
		URI:                 uri,
		Data:                data,
		DataUpdatePredicate: dataUpdatePredicate,
	}
	result, err := tw.NewNFT(cmd.Context(), accountNumber, nft, mintPredicateInput)
	if err != nil {
		return err
	}
	config.Base.ConsoleWriter.Println(fmt.Sprintf("Sent request for new non-fungible token with id=%s", result.GetUnit()))
	if result.FeeSum > 0 {
		config.Base.ConsoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", util.AmountToString(result.FeeSum, 8)))
	}
	if err := saveTxProofs(cmd, result.GetProofs(), config.Base.ConsoleWriter); err != nil {
		return fmt.Errorf("saving transaction proof(s): %w", err)
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
	cmd.Flags().StringSlice(cmdFlagInheritBearerClauseInput, []string{predicateTrue}, "input to satisfy the owner predicates inherited from types. "+helpPredicateArgument)
	cmd.Flags().String(cmdFlagBearerClauseInput, predicatePtpkh, "input to satisfy the bearer clause. "+helpPredicateArgument)
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
	defer tw.Close()

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

	ib, err := readPredicateInputs(cmd, cmdFlagInheritBearerClauseInput, accountNumber, tw.GetAccountManager())
	if err != nil {
		return err
	}

	ownerProofInput, err := readSinglePredicateInput(cmd, cmdFlagBearerClauseInput, accountNumber, tw.GetAccountManager())
	if err != nil {
		return err
	}

	// get token type and convert amount string
	tt, err := tw.GetFungibleTokenType(cmd.Context(), typeId)
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
	result, err := tw.SendFungible(cmd.Context(), accountNumber, typeId, targetValue, pubKey, ownerProofInput, ib)
	if err != nil {
		return err
	}
	for _, sub := range result.Submissions {
		if sub.Confirmed() && !sub.Success() {
			config.Base.ConsoleWriter.Println(fmt.Sprintf("Transaction failed for unit %s with status %d", sub.UnitID, sub.Status()))
		}
	}
	if result.FeeSum > 0 {
		config.Base.ConsoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", util.AmountToString(result.FeeSum, 8)))
	}
	if err := saveTxProofs(cmd, result.GetProofs(), config.Base.ConsoleWriter); err != nil {
		return fmt.Errorf("saving transaction proof(s): %w", err)
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
	cmd.Flags().StringSlice(cmdFlagInheritBearerClauseInput, []string{predicateTrue}, "input to satisfy the owner predicates inherited from types. "+helpPredicateArgument)
	cmd.Flags().String(cmdFlagBearerClauseInput, predicatePtpkh, "input to satisfy the bearer clause. "+helpPredicateArgument)
	setHexFlag(cmd, cmdFlagTokenID, nil, "token identifier")
	err := cmd.MarkFlagRequired(cmdFlagTokenID)
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
	defer tw.Close()

	tokenID, err := getHexFlag(cmd, cmdFlagTokenID)
	if err != nil {
		return err
	}

	pubKey, err := getPubKeyBytes(cmd, args.AddressCmdName)
	if err != nil {
		return err
	}

	typeOwnerPredicateInputs, err := readPredicateInputs(cmd, cmdFlagInheritBearerClauseInput, accountNumber, tw.GetAccountManager())
	if err != nil {
		return err
	}

	ownerPredicateInput, err := readSinglePredicateInput(cmd, cmdFlagBearerClauseInput, accountNumber, tw.GetAccountManager())
	if err != nil {
		return err
	}

	result, err := tw.TransferNFT(cmd.Context(), accountNumber, tokenID, pubKey, typeOwnerPredicateInputs, ownerPredicateInput)
	if err != nil {
		return err
	}
	config.Base.ConsoleWriter.Println(fmt.Sprintf("Sent request to transfer NFT with id=%s", result.GetUnit()))
	if result.FeeSum > 0 {
		config.Base.ConsoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", util.AmountToString(result.FeeSum, 8)))
	}
	if err := saveTxProofs(cmd, result.GetProofs(), config.Base.ConsoleWriter); err != nil {
		return fmt.Errorf("saving transaction proof(s): %w", err)
	}
	return err
}

func tokenCmdDC(config *types.WalletConfig, runner runTokenCmdDC) *cobra.Command {
	var accountNumber uint64

	cmd := &cobra.Command{
		Use:   "collect-dust",
		Short: "join fungible tokens into one unit",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, &accountNumber)
		},
	}

	cmd.Flags().Uint64VarP(&accountNumber, args.KeyCmdName, "k", 0, "which key to use for dust collection, 0 for all tokens from all accounts")
	cmd.Flags().StringSlice(cmdFlagType, nil, "type unit identifier (hex)")
	cmd.Flags().StringSlice(cmdFlagInheritBearerClauseInput, []string{predicateTrue}, "input to satisfy the owner predicates inherited from types. "+helpPredicateArgument)
	cmd.Flags().String(cmdFlagBearerClauseInput, predicatePtpkh, "input to satisfy the bearer clause. "+helpPredicateArgument)

	if err := cmd.MarkFlagRequired(cmdFlagType); err != nil {
		panic(err)
	}

	return cmd
}

func execTokenCmdDC(cmd *cobra.Command, config *types.WalletConfig, accountNumber *uint64) error {
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Close()

	typeIDStrs, err := cmd.Flags().GetStringSlice(cmdFlagType)
	if err != nil {
		return err
	}
	var typez []sdktypes.TokenTypeID
	for _, tokenType := range typeIDStrs {
		typeBytes, err := tokenswallet.DecodeHexOrEmpty(tokenType)
		if err != nil {
			return err
		}
		if len(typeBytes) > 0 {
			typez = append(typez, typeBytes)
		}
	}

	// TODO: check the case with an inherit predicate other than "always true" and accNr = 0, might fail
	ib, err := readPredicateInputs(cmd, cmdFlagInheritBearerClauseInput, *accountNumber, tw.GetAccountManager())
	if err != nil {
		return err
	}

	// TODO: solve for "All accounts" case
	ownerPredicateInput, err := readSinglePredicateInput(cmd, cmdFlagBearerClauseInput, *accountNumber, tw.GetAccountManager())
	if err != nil {
		return err
	}

	results, err := tw.CollectDust(cmd.Context(), *accountNumber, typez, ownerPredicateInput, ib)
	if err != nil {
		return err
	}
	for idx, result := range results {
		if len(result) == 0 {
			config.Base.ConsoleWriter.Println(fmt.Sprintf("Nothing to swap on account #%d", idx+1))
		} else {
			for _, dcResult := range result {
				config.Base.ConsoleWriter.Println(fmt.Sprintf("Paid %s fees for dust collection on Account number %d.", util.AmountToString(dcResult.FeeSum, 8), idx+1))
			}
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
	setHexFlag(cmd, cmdFlagTokenID, nil, "token identifier")
	if err := cmd.MarkFlagRequired(cmdFlagTokenID); err != nil {
		panic(err)
	}

	addDataFlags(cmd)
	cmd.Flags().String(cmdFlagTokenDataUpdateClauseInput, predicateTrue, "input to satisfy the token's data-update clause. "+helpPredicateArgument)
	cmd.Flags().StringSlice(cmdFlagInheritTokenDataUpdateClauseInput, []string{predicateTrue}, "input to satisfy the data-update clauses of inherited types. "+helpPredicateArgument)
	return addCommonAccountFlags(cmd)
}

func execTokenCmdUpdateNFTData(cmd *cobra.Command, config *types.WalletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}

	tokenID, err := getHexFlag(cmd, cmdFlagTokenID)
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
	defer tw.Close()

	tokenDataUpdatePredicateInput, err := readSinglePredicateInput(cmd, cmdFlagTokenDataUpdateClauseInput, accountNumber, tw.GetAccountManager())
	if err != nil {
		return err
	}

	tokenTypeDataUpdatePredicateInputs, err := readPredicateInputs(cmd, cmdFlagInheritTokenDataUpdateClauseInput, accountNumber, tw.GetAccountManager())
	if err != nil {
		return err
	}

	result, err := tw.UpdateNFTData(cmd.Context(), accountNumber, tokenID, data, tokenDataUpdatePredicateInput, tokenTypeDataUpdatePredicateInputs)
	if err != nil {
		return err
	}
	config.Base.ConsoleWriter.Println(fmt.Sprintf("Sent request to update NFT with id=%s", result.GetUnit()))
	for _, sub := range result.Submissions {
		if sub.Confirmed() && !sub.Success() {
			config.Base.ConsoleWriter.Println(fmt.Sprintf("Transaction failed for unit %s with status %d", sub.UnitID, sub.Status()))
		}
	}
	if result.FeeSum > 0 {
		config.Base.ConsoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", util.AmountToString(result.FeeSum, 8)))
	}
	if err := saveTxProofs(cmd, result.GetProofs(), config.Base.ConsoleWriter); err != nil {
		return fmt.Errorf("saving transaction proof(s): %w", err)
	}
	return err
}

func tokenCmdList(config *types.WalletConfig, runner runTokenListCmd) *cobra.Command {
	var accountNumber uint64
	cmd := &cobra.Command{
		Use:   "list",
		Short: "lists all available tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, &accountNumber, Any)
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
	cmd.PersistentFlags().Uint64VarP(&accountNumber, args.KeyCmdName, "k", allAccounts, "which account tokens to list (0 for all accounts)")
	return cmd
}

func tokenCmdListFungible(config *types.WalletConfig, runner runTokenListCmd, accountNumber *uint64) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fungible",
		Short: "lists fungible tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, accountNumber, Fungible)
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
			return runner(cmd, config, accountNumber, NonFungible)
		},
	}

	cmd.Flags().Bool(cmdFlagWithAll, false, "Show all available fields for each token")
	cmd.Flags().Bool(cmdFlagWithTypeName, false, "Show type name field")
	cmd.Flags().Bool(cmdFlagWithTokenURI, false, "Show token URI field")
	cmd.Flags().Bool(cmdFlagWithTokenData, false, "Show token data field")

	return cmd
}

func execTokenCmdList(cmd *cobra.Command, config *types.WalletConfig, accountNumber *uint64, kind Kind) error {
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Close()

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
		if kind == Any || kind == NonFungible {
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

	var firstAccountNumber, lastAccountNumber uint64
	if *accountNumber == allAccounts {
		firstAccountNumber = 1
		maxAccountIndex, err := tw.GetAccountManager().GetMaxAccountIndex()
		if err != nil {
			return err
		}
		lastAccountNumber = maxAccountIndex + 1
	} else {
		firstAccountNumber = *accountNumber
		lastAccountNumber = *accountNumber
	}

	atLeastOneFound := false
	for accountNumber := firstAccountNumber; accountNumber <= lastAccountNumber; accountNumber++ {
		ownerAccount := fmt.Sprintf("Tokens owned by account #%v", accountNumber)
		atLeastOneFoundForAccount := false

		if kind == Any || kind == Fungible {
			tokens, err := tw.ListFungibleTokens(cmd.Context(), accountNumber)
			if err != nil {
				return err
			}
			if len(tokens) > 0 {
				atLeastOneFound = true
				atLeastOneFoundForAccount = true
				config.Base.ConsoleWriter.Println(ownerAccount)
			}
			for _, t := range tokens {
				var typeName string
				if withAll || withTypeName {
					typeName = fmt.Sprintf(", token-type-name='%s'", t.TypeName)
				}
				amount := util.AmountToString(t.Amount, t.DecimalPlaces)
				config.Base.ConsoleWriter.Println(fmt.Sprintf("ID='%s', symbol='%s', amount='%v', token-type='%s', locked='%s'",
					t.ID, t.Symbol, amount, t.TypeID, wallet.LockReason(t.LockStatus).String()) + typeName + " (fungible)")
			}
		}

		if kind == Any || kind == NonFungible {
			tokens, err := tw.ListNonFungibleTokens(cmd.Context(), accountNumber)
			if err != nil {
				return err
			}
			if len(tokens) > 0 {
				atLeastOneFound = true
				if !atLeastOneFoundForAccount {
					config.Base.ConsoleWriter.Println(ownerAccount)
				}
			}
			for _, t := range tokens {
				var typeName, nftURI, nftData string
				if withAll || withTypeName {
					typeName = fmt.Sprintf(", token-type-name='%s'", t.TypeName)
				}
				if withAll || withTokenURI {
					nftURI = fmt.Sprintf(", URI='%s'", t.URI)
				}
				if withAll || withTokenData {
					nftData = fmt.Sprintf(", data='%X'", t.Data)
				}

				config.Base.ConsoleWriter.Println(fmt.Sprintf("ID='%s', symbol='%s', name='%s', token-type='%s', locked='%s'",
					t.ID, t.Symbol, t.Name, t.TypeID, wallet.LockReason(t.LockStatus).String()) + typeName + nftURI + nftData + " (nft)")
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
			return runner(cmd, config, &accountNumber, Any)
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
			return runner(cmd, config, &accountNumber, Fungible)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "non-fungible",
		Short: "lists non-fungible types",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runner(cmd, config, &accountNumber, NonFungible)
		},
	})
	return cmd
}

func execTokenCmdListTypes(cmd *cobra.Command, config *types.WalletConfig, accountNumber *uint64, kind Kind) error {
	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Close()

	printTokenType := func(id basetypes.UnitID, symbol, name string, kind Kind) {
		optionalName := ""
		if name != "" {
			optionalName = fmt.Sprintf(", name=%s", name)
		}
		kindStr := fmt.Sprintf(" (%v)", kind)
		config.Base.ConsoleWriter.Println(fmt.Sprintf("ID=%s, symbol=%s", id, symbol) + optionalName + kindStr)
	}

	if kind == Any || kind == Fungible {
		res, err := tw.ListFungibleTokenTypes(cmd.Context(), *accountNumber)
		if err != nil {
			return err
		}
		for _, tt := range res {
			printTokenType(tt.ID, tt.Symbol, tt.Name, Fungible)
		}
	}
	if kind == Any || kind == NonFungible {
		res, err := tw.ListNonFungibleTokenTypes(cmd.Context(), *accountNumber)
		if err != nil {
			return err
		}
		for _, tt := range res {
			printTokenType(tt.ID, tt.Symbol, tt.Name, NonFungible)
		}
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
	setHexFlag(cmd, cmdFlagTokenID, nil, "token identifier")
	if err := cmd.MarkFlagRequired(cmdFlagTokenID); err != nil {
		panic(err)
	}
	cmd.Flags().String(cmdFlagBearerClauseInput, predicatePtpkh, "input to satisfy the bearer clause. "+helpPredicateArgument)
	return addCommonAccountFlags(cmd)
}

func execTokenCmdLock(cmd *cobra.Command, config *types.WalletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	tokenID, err := getHexFlag(cmd, cmdFlagTokenID)
	if err != nil {
		return err
	}

	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Close()

	ownerPredicateInput, err := readSinglePredicateInput(cmd, cmdFlagBearerClauseInput, accountNumber, tw.GetAccountManager())
	if err != nil {
		return err
	}

	result, err := tw.LockToken(cmd.Context(), accountNumber, tokenID, ownerPredicateInput)
	if err != nil {
		return err
	}
	config.Base.ConsoleWriter.Println(fmt.Sprintf("Sent request to lock token with id=%s", result.GetUnit()))
	if result.FeeSum > 0 {
		config.Base.ConsoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", util.AmountToString(result.FeeSum, 8)))
	}
	if err := saveTxProofs(cmd, result.GetProofs(), config.Base.ConsoleWriter); err != nil {
		return fmt.Errorf("saving transaction proof(s): %w", err)
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
	setHexFlag(cmd, cmdFlagTokenID, nil, "token identifier")
	if err := cmd.MarkFlagRequired(cmdFlagTokenID); err != nil {
		panic(err)
	}
	cmd.Flags().String(cmdFlagBearerClauseInput, predicatePtpkh, "input to satisfy the bearer clause. "+helpPredicateArgument)
	return addCommonAccountFlags(cmd)
}

func execTokenCmdUnlock(cmd *cobra.Command, config *types.WalletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(args.KeyCmdName)
	if err != nil {
		return err
	}
	tokenID, err := getHexFlag(cmd, cmdFlagTokenID)
	if err != nil {
		return err
	}

	tw, err := initTokensWallet(cmd, config)
	if err != nil {
		return err
	}
	defer tw.Close()

	ownerPredicateInput, err := readSinglePredicateInput(cmd, cmdFlagBearerClauseInput, accountNumber, tw.GetAccountManager())
	if err != nil {
		return err
	}

	result, err := tw.UnlockToken(cmd.Context(), accountNumber, tokenID, ownerPredicateInput)
	if err != nil {
		return err
	}
	config.Base.ConsoleWriter.Println(fmt.Sprintf("Sent request to unlock token with id=%s", result.GetUnit()))
	if result.FeeSum > 0 {
		config.Base.ConsoleWriter.Println(fmt.Sprintf("Paid %s fees for transaction(s).", util.AmountToString(result.FeeSum, 8)))
	}
	if err := saveTxProofs(cmd, result.GetProofs(), config.Base.ConsoleWriter); err != nil {
		return fmt.Errorf("saving transaction proof(s): %w", err)
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
	confirmTx, _, err := args.WaitForProofArg(cmd)
	if err != nil {
		return nil, err
	}
	maxFee, err := args.ParseMaxFeeFlag(cmd)
	if err != nil {
		return nil, err
	}
	tokensClient, err := client.NewTokensPartitionClient(cmd.Context(), args.BuildRpcUrl(rpcUrl))
	if err != nil {
		return nil, fmt.Errorf("failed to dial rpc client: %w", err)
	}

	return tokenswallet.New(tokensClient, am, confirmTx, nil, maxFee, config.Base.Logger)
}

func readParentTypeInfo(cmd *cobra.Command, keyNr uint64, am account.Manager) (sdktypes.TokenTypeID, []*tokenswallet.PredicateInput, error) {
	parentType, err := getHexFlag(cmd, cmdFlagParentType)
	if err != nil {
		return nil, nil, err
	}

	if len(parentType) == 0 {
		return nil, []*tokenswallet.PredicateInput{}, nil
	}

	creationInputs, err := readPredicateInputs(cmd, cmdFlagSybTypeClauseInput, keyNr, am)
	if err != nil {
		return nil, nil, err
	}

	return parentType, creationInputs, nil
}

/*
readPredicateInputs reads the flag value and converts it to predicate inputs. Returns a single input with nil argument if the flag is empty.
*/
func readPredicateInputs(cmd *cobra.Command, flag string, keyNr uint64, am account.Manager) ([]*tokenswallet.PredicateInput, error) {
	creationInputStrs, err := cmd.Flags().GetStringSlice(flag)
	if err != nil {
		return nil, err
	}
	if len(creationInputStrs) == 0 {
		key, err := am.GetAccountKey(keyNr)
		if err != nil {
			return nil, err
		}
		return []*tokenswallet.PredicateInput{{Argument: nil, AccountKey: key}}, nil
	}
	return tokenswallet.ParsePredicateArguments(creationInputStrs, keyNr, am)
}

/*
readSinglePredicateInput reads the flag value and converts it to predicate input. Returns nil if the flag is empty.
*/
func readSinglePredicateInput(cmd *cobra.Command, flag string, keyNr uint64, am account.Manager) (*tokenswallet.PredicateInput, error) {
	arg, err := cmd.Flags().GetString(flag)
	if err != nil {
		return nil, err
	}
	if arg == "" {
		return nil, nil
	}
	predicateArgs, err := tokenswallet.ParsePredicateArguments([]string{arg}, keyNr, am)
	if err != nil {
		return nil, err
	}
	if len(predicateArgs) != 1 {
		return nil, fmt.Errorf("expected exactly one argument, got %d", len(predicateArgs))
	}
	return predicateArgs[0], nil
}

/*
parsePredicateClauseCmd reads the "flag" value and converts it to predicate bytes.
The flag's value must use the following format:
  - empty string returns "always true"
  - true
  - false
  - ptpkh
  - ptpkh:n - where n is integer >= 0
  - ptpkh:0x<hex> - where hex value is the hash of a public key
  - @filename - to load the content of given file
*/
func parsePredicateClauseCmd(cmd *cobra.Command, flag string, keyNr uint64, am account.Manager) ([]byte, error) {
	clause, err := cmd.Flags().GetString(flag)
	if err != nil {
		return nil, fmt.Errorf("reading flag %q value: %w", flag, err)
	}
	buf, err := tokenswallet.ParsePredicateClause(clause, keyNr, am)
	if err != nil {
		return nil, fmt.Errorf("parsing flag %q value: %w", flag, err)
	}
	return buf, nil
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

func readIconFile(iconFilePath string) (*tokens.Icon, error) {
	if len(iconFilePath) == 0 {
		return nil, nil
	}
	icon := &tokens.Icon{}

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

/*
saveTxProofs saves the tx proofs into file when the cmd has appropriate flag set.
*/
func saveTxProofs(cmd *cobra.Command, proofs []*basetypes.TxRecordProof, out types.ConsoleWrapper) error {
	_, proofFile, err := args.WaitForProofArg(cmd)
	if err != nil {
		return err
	}
	if proofFile == "" {
		return nil
	}

	w, err := os.Create(proofFile)
	if err != nil {
		return fmt.Errorf("creating file for transaction proofs: %w", err)
	}
	if err := basetypes.Cbor.Encode(w, proofs); err != nil {
		return fmt.Errorf("encoding transaction proofs as CBOR: %w", err)
	}
	out.Println("Transaction proof(s) saved to file:" + proofFile)
	return nil
}

func (kind Kind) String() string {
	switch kind {
	case Any:
		return "all"
	case Fungible:
		return "fungible"
	case NonFungible:
		return "nft"
	}
	return "unknown"
}
