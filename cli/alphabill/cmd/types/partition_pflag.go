package types

import (
	"errors"
)

// PartitionType "partition" cli flag, implements github.com/spf13/pflag/flag.go#Value interface
type PartitionType string

const (
	MoneyType            PartitionType = "money"
	TokensType           PartitionType = "tokens"
	EnterpriseTokensType PartitionType = "enterprise-tokens"
	EvmType              PartitionType = "evm"
)

// String returns string value of given partitionType, used in Printf and help context
func (e *PartitionType) String() string {
	return string(*e)
}

// Set sets the value of this partitionType string
func (e *PartitionType) Set(v string) error {
	switch v {
	case "money", "tokens", "enterprise-tokens", "evm":
		*e = PartitionType(v)
		return nil
	default:
		return errors.New("must be one of [money|tokens|enterprise-tokens|evm]")
	}
}

// Type used to show the type value in the help context
func (e *PartitionType) Type() string {
	return "string"
}
