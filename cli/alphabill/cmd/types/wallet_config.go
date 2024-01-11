package types

import "go.opentelemetry.io/otel/trace"

type WalletConfig struct {
	Base            *BaseConfiguration
	WalletHomeDir   string
	PasswordFromArg string
	PromptPassword  bool
}

func (wc *WalletConfig) Tracer() trace.Tracer {
	return wc.Base.Observe.Tracer("main")
}
