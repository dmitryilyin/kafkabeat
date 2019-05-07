package main

import (
	"os"

	"github.com/dmitryilyin/kafkabeat/cmd"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
