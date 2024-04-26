package types

type WalletConfig struct {
	Base            *BaseConfiguration
	WalletHomeDir   string
	PasswordFromArg string
	PromptPassword  bool
}
