package cmd

import (
	"encoding/hex"
	"strings"
)

// bytesHex cobra cli hex value flag that accepts any hex string with or without 0x prefix,
// implements github.com/spf13/pflag/flag.go#Value interface
type bytesHex []byte

// String returns string value of given hexVal, used in Printf and help context
func (h *bytesHex) String() string {
	return hex.EncodeToString(*h)
}

// Set sets the value of this bytesHex string
func (h *bytesHex) Set(v string) error {
	if strings.HasPrefix(v, "0x") {
		v = v[2:]
	}
	b, err := hex.DecodeString(v)
	if err != nil {
		return err
	}
	*h = b
	return nil
}

// Type used to show the type value in the help context
func (h *bytesHex) Type() string {
	return "hex"
}
