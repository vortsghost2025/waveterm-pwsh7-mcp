package main

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/wavetermdev/waveterm/pkg/wavebase"
	"github.com/wavetermdev/waveterm/pkg/wavejwt"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: jwt-tool <base64-private-key> [socket-path]\n")
		os.Exit(1)
	}

	privKeyB64 := os.Args[1]
	sockPath := ""
	if len(os.Args) >= 3 {
		sockPath = os.Args[2]
	}

	privKeyBytes, err := base64.StdEncoding.DecodeString(privKeyB64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error decoding private key: %v\n", err)
		os.Exit(1)
	}

	err = wavejwt.SetPrivateKey(privKeyBytes)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error setting private key: %v\n", err)
		os.Exit(1)
	}

	if sockPath == "" {
		sockPath = wavebase.GetDomainSocketName()
	}

	claims := &wavejwt.WaveJwtClaims{
		Sock:   sockPath,
		Router: true,
	}

	tokenStr, err := wavejwt.Sign(claims)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error signing JWT: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(tokenStr)
}
