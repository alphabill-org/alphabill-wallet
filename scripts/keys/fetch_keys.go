package main

import (
	"flag"
	"log"

	"github.com/alphabill-org/alphabill-wallet/wallet/account"
)

/*
Example usage

add keys:
./build/abwallet wallet create -l ./testwallet
./build/abwallet wallet -l ./testwallet add-key

fetch keys:
go run scripts/keys/fetch_keys.go --dir /path/to/account.db [--pw password]
*/
func main() {

	dir := flag.String("dir", "", "account.db directory")
	pw := flag.String("pw", "", "db password")
	flag.Parse()

	if *dir == "" {
		log.Fatal("dir is required")
	}

	manager, err := account.NewManager(*dir, *pw, false)
	if err != nil {
		log.Fatal(err)
	}

	keys, err := manager.GetAccountKeys()
	if err != nil {
		log.Fatal(err)
	}

	for idx, key := range keys {
		log.Printf("Account #: %d\n", idx+1)
		log.Printf("Public Key: 0x%X\n", key.PubKey)
		log.Printf("Public Key Hash: 0x%X\n", key.PubKeyHash.Sha256)
		log.Printf("Private Key: 0x%X\n", key.PrivKey)
		log.Printf("Derivation Path: 0x%X\n\n", key.DerivationPath)
	}
}
