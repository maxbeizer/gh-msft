package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/maxbeizer/gh-msft/internal/cli"
	"github.com/maxbeizer/gh-msft/internal/workiq"
)

func main() {
	userMessages := log.New(os.Stderr, "", 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer func() {
		signal.Stop(c)
		cancel()
	}()

	go func() {
		for sig := range c {
			userMessages.Printf("received signal %v", sig)
			cancel()
		}
	}()

	if len(os.Args) > 1 && os.Args[1] == "__broker" {
		if err := workiq.RunBroker(ctx); err != nil {
			userMessages.Printf("broker error: %v", err)
			os.Exit(1)
		}
		return
	}

	rootCmd := cli.NewRootCmd(cli.DefaultFactory)
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		userMessages.Printf("error: %v", err)
		if errors.Is(err, workiq.ErrEULANotAccepted) {
			userMessages.Print("run `gh msft accept-eula` to accept it")
		}
		os.Exit(1)
	}
}
