package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

func newCalCmd(factory Factory) *cobra.Command {
	var top int
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "cal",
		Short: "List upcoming calendar events",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			providers, cleanup, sp, err := build(cmd, factory)
			if err != nil {
				return err
			}
			defer cleanup()
			sp.setMessage("Loading calendar…")
			events, err := providers.Cal.Upcoming(cmd.Context(), top)
			sp.stopSpinner()
			if err != nil {
				return fmt.Errorf("listing calendar: %w", err)
			}
			return writeEvents(cmd.OutOrStdout(), events, asJSON)
		},
	}
	cmd.Flags().IntVar(&top, "top", 25, "maximum number of events to list")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}

// build invokes the factory and returns the providers, a cleanup func, and a
// running spinner. It centralizes the nil-factory guard and the "WorkIQ not
// available" hinting. The spinner reports startup progress on stderr (a no-op when
// stderr isn't a terminal); callers should update its message for subsequent slow
// steps and call stopSpinner before writing output.
func build(cmd *cobra.Command, factory Factory) (*Providers, func(), *spinner, error) {
	sp := newSpinner(cmd.ErrOrStderr(), "Starting WorkIQ (first launch can take a few seconds)…")
	if factory == nil {
		return nil, func() {}, sp, fmt.Errorf("no provider factory configured")
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	sp.start()
	providers, err := factory(ctx)
	if err != nil {
		sp.stopSpinner()
		return nil, func() {}, sp, fmt.Errorf("%w\nEnsure WorkIQ is installed and you are signed in (run `npx -y @microsoft/workiq accept-eula`)", err)
	}
	cleanup := func() {}
	if providers.Close != nil {
		cleanup = providers.Close
	}
	return providers, cleanup, sp, nil
}
