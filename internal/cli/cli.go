// Package cli wires the gh-msft cobra command tree on top of the mail and calendar
// providers. Providers are constructed lazily via a Factory so that command
// wiring and output formatting can be unit-tested without launching WorkIQ.
package cli

import (
	"context"

	"github.com/maxbeizer/gh-msft/internal/calendar"
	"github.com/maxbeizer/gh-msft/internal/mail"
	"github.com/maxbeizer/gh-msft/internal/tui"
	"github.com/maxbeizer/gh-msft/internal/workiq"
	"github.com/spf13/cobra"
)

// Providers bundles the data providers plus a cleanup func.
type Providers struct {
	Mail  mail.Provider
	Cal   calendar.Provider
	EULA  EULAAccepter
	Close func()
}

// EULAAccepter accepts the WorkIQ End User License Agreement.
type EULAAccepter interface {
	AcceptEULA(ctx context.Context) error
}

// Factory builds providers on demand (e.g. spawning WorkIQ). It is only called
// when a command actually needs data.
type Factory func(ctx context.Context) (*Providers, error)

type tuiRunner func(mail.Provider, calendar.Provider, int, bool, bool) error

// DefaultFactory spawns a WorkIQ client and builds WorkIQ-backed providers.
func DefaultFactory(ctx context.Context) (*Providers, error) {
	c, err := workiq.New(ctx)
	if err != nil {
		return nil, err
	}
	return &Providers{
		Mail:  mail.NewWorkIQProvider(c),
		Cal:   calendar.NewWorkIQProvider(c),
		EULA:  c,
		Close: func() { _ = c.Close() },
	}, nil
}

// NewRootCmd builds the root command. factory may be nil for `--help`-only use.
func NewRootCmd(factory Factory) *cobra.Command {
	return newRootCmd(factory, tui.Run)
}

func newRootCmd(factory Factory, runTUI tuiRunner) *cobra.Command {
	root := &cobra.Command{
		Use:   "gh-msft",
		Short: "Read and triage your Microsoft 365 mail and calendar from the terminal",
		Long: "gh-msft reads your Microsoft 365 inbox and calendar via WorkIQ.\n" +
			"Authentication is handled entirely by WorkIQ (no credentials are stored by this tool).",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newMailCmd(factory))
	root.AddCommand(newCalCmd(factory))
	root.AddCommand(newEULACmd(factory))
	root.AddCommand(newDemoCmd(runTUI))
	tuiCmd := newTUICmd(factory, runTUI)
	root.AddCommand(tuiCmd)
	root.RunE = func(cmd *cobra.Command, args []string) error {
		return tuiCmd.RunE(cmd, args)
	}
	return root
}
