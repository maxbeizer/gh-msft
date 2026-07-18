package cli

import (
	"github.com/maxbeizer/gh-msft/internal/tui"
	"github.com/spf13/cobra"
)

func newTUICmd(factory Factory) *cobra.Command {
	var top int
	var mailAll bool
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive inbox",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			providers, cleanup, sp, err := build(cmd, factory)
			if err != nil {
				return err
			}
			defer cleanup()
			sp.stopSpinner()
			return tui.Run(providers.Mail, top, mailAll)
		},
	}
	cmd.Flags().IntVar(&top, "top", 50, "maximum number of messages to load")
	cmd.Flags().BoolVar(&mailAll, "mail-all", false, "load all mail instead of only the inbox")
	return cmd
}
