package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

/*
Example usage
go run scripts/templates/create_p2pkh.go --pubkey 0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3
*/
func main() {
	pubKeyHex := flag.String("pubkey", "", "public key of the new unit owner")
	flag.Parse()

	if *pubKeyHex == "" {
		log.Fatal("pubkey is required")
	}

	pubKey, err := hexutil.Decode(*pubKeyHex)
	if err != nil {
		log.Fatal(err)
	}

	predicateBytes := templates.NewP2pkh256BytesFromKey(pubKey)
	fmt.Printf("0x%X\n", predicateBytes)
}
