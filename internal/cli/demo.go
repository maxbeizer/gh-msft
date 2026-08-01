package cli

import (
	"github.com/maxbeizer/gh-msft/internal/demo"
	"github.com/spf13/cobra"
)

func newDemoCmd(runTUI tuiRunner) *cobra.Command {
	var startCal bool
	cmd := &cobra.Command{
		Use:   "demo",
		Short: "Launch the interactive UI with synthetic data",
		Long:  "Launch the interactive inbox and calendar using deterministic synthetic data. No Microsoft 365 account or WorkIQ setup is required.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runTUI(demo.MailProvider{}, demo.CalendarProvider{}, 50, false, startCal)
		},
	}
	cmd.Flags().BoolVar(&startCal, "cal", false, "start in calendar mode instead of mail mode")
	return cmd
}
