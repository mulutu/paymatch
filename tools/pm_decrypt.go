package main

import (
	"encoding/base64"
	"fmt"
	"os"

	"paymatch/internal/crypto"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("usage: go run tools/pm_decrypt.go <ciphertext>")
		os.Exit(1)
	}
	keyB64 := os.Getenv("AES_256_KEY_BASE64")
	if keyB64 == "" {
		fmt.Println("AES_256_KEY_BASE64 is not set")
		os.Exit(1)
	}
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil || len(key) != 32 {
		fmt.Println("AES_256_KEY_BASE64 must be valid base64 of 32 bytes")
		os.Exit(1)
	}

	pt, err := crypto.DecryptString(key, os.Args[1])
	if err != nil {
		panic(err)
	}
	fmt.Println(pt)
}
