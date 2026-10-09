// Command bored is a cross-project kanban board for you and your coding agents.
package main

import (
	"os"

	"github.com/Commonwealth-Labs/bored/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
