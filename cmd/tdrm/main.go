package main

import (
	"context"
	"log"
	"os"

	"github.com/mizzy/tdrm"
)

// version is set by goreleaser via ldflags.
var version = "current"

func main() {
	ctx := context.Background()
	app, err := tdrm.New(ctx, "")
	if err != nil {
		log.Fatal(err)
	}
	cli := app.NewCLI()
	cli.Version = version
	if err := cli.RunContext(ctx, os.Args); err != nil {
		log.Fatal(err)
	}
}
