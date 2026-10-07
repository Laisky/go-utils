// Package main gutils' command line tool
package main

import (
	"github.com/Laisky/go-utils/v6/cmd"
)

// main is the entry point of the gutils command line tool. It takes no parameters and returns nothing; it
// delegates to cmd.Execute, which parses the command line arguments and runs the selected subcommand.
func main() {
	cmd.Execute()
}
