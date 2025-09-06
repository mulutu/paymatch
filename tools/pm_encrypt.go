package main

import (
	"fmt"
	"os"
	"paymatch/internal/config"
	"paymatch/internal/crypto"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("usage: go run tools/pm_encrypt.go <plaintext>")
		os.Exit(1)
	}
	cfg := config.Load() // reads AES_256_KEY_BASE64 from env
	enc, err := crypto.EncryptString(cfg.Sec.AESKey, os.Args[1])
	if err != nil {
		panic(err)
	}
	fmt.Println(enc)
}
