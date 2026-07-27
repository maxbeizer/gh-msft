package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newEULACmd(factory Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "accept-eula",
		Short: "Accept the WorkIQ End User License Agreement",
		Long: "Accept the WorkIQ End User License Agreement (EULA), required once\n" +
			"before mail and calendar commands will run.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			providers, cleanup, sp, err := build(cmd, factory)
			if err != nil {
				return err
			}
			defer cleanup()
			sp.setMessage("Accepting EULA…")
			err = providers.EULA.AcceptEULA(cmd.Context())
			sp.stopSpinner()
			if err != nil {
				return fmt.Errorf("accepting EULA: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "EULA accepted")
			return nil
		},
	}
}
