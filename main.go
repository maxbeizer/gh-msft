package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/maxbeizer/gh-msft/internal/cli"
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

	rootCmd := cli.NewRootCmd(cli.DefaultFactory)
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		userMessages.Printf("error: %v", err)
		os.Exit(1)
	}
}
