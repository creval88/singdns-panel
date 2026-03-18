package auth

import (
	"fmt"
	"os"
)

func RunHashCLI() int {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: singdns-panel hash-password <password>")
		return 1
	}
	hash, err := HashPassword(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(hash)
	return 0
}
