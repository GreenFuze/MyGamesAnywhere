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
	"github.com/GreenFuze/MyGamesAnywhere/client/internal/desktop"
	"github.com/GreenFuze/MyGamesAnywhere/client/internal/launchlog"
)

func main() {
	launchLogger, launchLogErr := launchlog.OpenBesideExecutable()
	if launchLogger != nil {
		defer launchLogger.Close()
	}
	logLaunch := func(format string, values ...any) {
		if launchLogger != nil {
			launchLogger.Printf(format, values...)
		}
	}
	if launchLogErr != nil {
		fmt.Fprintf(os.Stderr, "launch log unavailable: %v\n", launchLogErr)
	}

	executable, _ := os.Executable()
	logLaunch("start pid=%d exe=%s args=%s", os.Getpid(), executable, launchlog.FormatArgs(os.Args))

	info := buildinfo.Current()
	clientService, err := clientapp.New(os.Getenv("MGA_CLIENT_DATA_DIR"), info, launchLogger.Writer())
	if err != nil {
		logLaunch("initialize runtime failed: %v", err)
		fmt.Fprintf(os.Stderr, "initialize MGA Client runtime: %v\n", err)
		os.Exit(1)
	}
	defer clientService.Close()
	application, err := cli.NewApplication(cli.Dependencies{
		Out:       os.Stdout,
		Err:       os.Stderr,
		BuildInfo: info,
		Client:    clientService,
	})
	if err != nil {
		logLaunch("initialize CLI failed: %v", err)
		fmt.Fprintf(os.Stderr, "initialize MGA Client: %v\n", err)
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := application.Execute(ctx, os.Args[1:]); err != nil {
		logLaunch("command failed: %v", err)
		clientService.Logf("command failed: %v", err)
		fmt.Fprintf(os.Stderr, "MGA Client: %v\n", err)
		if len(os.Args) > 1 && os.Args[1] == "protocol" {
			_ = desktop.ShowError("MGA Client", err.Error())
		}
		os.Exit(1)
	}
	logLaunch("command completed ok")
}
