package main

import (
	"os"

	snip "github.com/edouard-claude/snip"
)

func main() {
	os.Exit(snip.Run(os.Args))
}
