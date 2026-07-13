package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/GreenFuze/MyGamesAnywhere/client/internal/buildinfo"
	"github.com/GreenFuze/MyGamesAnywhere/client/internal/cli"
	"github.com/GreenFuze/MyGamesAnywhere/client/internal/clientapp"
)

func main() {
	info := buildinfo.Current()
	clientService, err := clientapp.New(os.Getenv("MGA_CLIENT_DATA_DIR"), info)
	if err != nil {
		fmt.Fprintf(os.Stderr, "initialize MGA Client runtime: %v\n", err)
		os.Exit(1)
	}
	application, err := cli.NewApplication(cli.Dependencies{
		Out:       os.Stdout,
		Err:       os.Stderr,
		BuildInfo: info,
		Client:    clientService,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "initialize MGA Client: %v\n", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := application.Execute(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "MGA Client: %v\n", err)
		os.Exit(1)
	}
}
