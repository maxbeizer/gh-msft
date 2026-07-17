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
			providers, cleanup, err := build(cmd, factory)
			if err != nil {
				return err
			}
			defer cleanup()
			events, err := providers.Cal.Upcoming(cmd.Context(), top)
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

// build invokes the factory and returns the providers plus a cleanup func. It
// centralizes the nil-factory guard and the "WorkIQ not available" hinting.
func build(cmd *cobra.Command, factory Factory) (*Providers, func(), error) {
	if factory == nil {
		return nil, func() {}, fmt.Errorf("no provider factory configured")
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	providers, err := factory(ctx)
	if err != nil {
		return nil, func() {}, fmt.Errorf("%w\nEnsure WorkIQ is installed and you are signed in (run `npx -y @microsoft/workiq accept-eula`)", err)
	}
	cleanup := func() {}
	if providers.Close != nil {
		cleanup = providers.Close
	}
	return providers, cleanup, nil
}
