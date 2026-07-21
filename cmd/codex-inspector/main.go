package main

import (
	"github.com/dylanjbarth/codex-inspector/internal/cli"
	"os"
)

func main() { os.Exit(cli.Main(os.Args[1:], cli.IO{In: os.Stdin, Out: os.Stdout, Err: os.Stderr})) }
