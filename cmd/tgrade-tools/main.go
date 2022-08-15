package main

import (
	"fmt"
	"os"

	"github.com/cosmos/cosmos-sdk/server"

	"github.com/confio/tgrade/app"
)

func main() {
	rootCmd, _ := NewRootCmd()

	if err := Execute(rootCmd, app.DefaultNodeHome); err != nil {
		fmt.Printf("%s", err)
		switch e := err.(type) {
		case server.ErrorCode:
			os.Exit(e.Code)

		default:
			os.Exit(1)
		}
	}
}
