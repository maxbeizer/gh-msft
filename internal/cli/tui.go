package cli

import (
	"github.com/maxbeizer/gh-msft/internal/tui"
	"github.com/spf13/cobra"
)

func newTUICmd(factory Factory) *cobra.Command {
	var top int
	var startCal bool
	var mailAll bool
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive inbox and calendar",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			providers, cleanup, sp, err := build(cmd, factory)
			if err != nil {
				return err
			}
			defer cleanup()
			sp.stopSpinner()
			return tui.Run(providers.Mail, providers.Cal, top, mailAll, startCal)
		},
	}
	cmd.Flags().IntVar(&top, "top", 50, "maximum number of items to load")
	cmd.Flags().BoolVar(&startCal, "cal", false, "start in calendar mode instead of mail mode")
	cmd.Flags().BoolVar(&mailAll, "mail-all", false, "load all mail instead of only the inbox")
	return cmd
}
