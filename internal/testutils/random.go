package test

import (
	"crypto/rand"
)

func RandomBytes(len int) []byte {
	bytes := make([]byte, len)
	_, err := rand.Read(bytes)
	if err != nil {
		panic(err)
	}
	return bytes
}
