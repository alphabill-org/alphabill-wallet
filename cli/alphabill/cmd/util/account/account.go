package account

import (
	"errors"
	"syscall"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"golang.org/x/term"

	"github.com/alphabill-org/alphabill-wallet/cli/alphabill/cmd/types"
	"github.com/alphabill-org/alphabill-wallet/wallet/account"
)

func LoadExistingAccountManager(config *types.WalletConfig) (account.Manager, error) {
	pw, err := GetPassphrase(config, "Enter passphrase: ")
	if err != nil {
		return nil, err
	}
	am, err := account.NewManager(config.WalletHomeDir, pw, false)
	if err != nil {
		return nil, err
	}
	return am, nil
}

func ReadPassword(consoleWriter types.ConsoleWrapper, promptMessage string) (string, error) {
	consoleWriter.Print(promptMessage)
	passwordBytes, err := term.ReadPassword(syscall.Stdin)
	if err != nil {
		return "", err
	}
	consoleWriter.Println("") // line break after reading password
	return string(passwordBytes), nil
}

func GetPassphrase(config *types.WalletConfig, promptMessage string) (string, error) {
	if config.PasswordFromArg != "" {
		return config.PasswordFromArg, nil
	}
	if !config.PromptPassword {
		return "", nil
	}
	return ReadPassword(config.Base.ConsoleWriter, promptMessage)
}

func CreatePassphrase(config *types.WalletConfig) (string, error) {
	if config.PasswordFromArg != "" {
		return config.PasswordFromArg, nil
	}
	if !config.PromptPassword {
		return "", nil
	}
	p1, err := ReadPassword(config.Base.ConsoleWriter, "Create new passphrase: ")
	if err != nil {
		return "", err
	}
	p2, err := ReadPassword(config.Base.ConsoleWriter, "Confirm passphrase: ")
	if err != nil {
		return "", err
	}
	if p1 != p2 {
		return "", errors.New("passphrases do not match")
	}
	return p1, nil
}

func PubKeyHexToBytes(s string) ([]byte, bool) {
	if len(s) != 68 {
		return nil, false
	}
	pubKeyBytes, err := hexutil.Decode(s)
	if err != nil {
		return nil, false
	}
	return pubKeyBytes, true
}
