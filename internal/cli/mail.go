package cli

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/maxbeizer/gh-msft/internal/mail"
	"github.com/spf13/cobra"
)

func newMailCmd(factory Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mail",
		Short: "Read and triage inbox mail",
	}
	cmd.AddCommand(newMailListCmd(factory))
	cmd.AddCommand(newMailViewCmd(factory))
	cmd.AddCommand(newMailArchiveCmd(factory))
	return cmd
}

func newMailListCmd(factory Factory) *cobra.Command {
	var top int
	var asJSON bool
	var all bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent inbox messages",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			providers, cleanup, sp, err := build(cmd, factory)
			if err != nil {
				return err
			}
			defer cleanup()
			sp.setMessages(inboxLoadingMessages...)
			msgs, err := providers.Mail.ListInbox(cmd.Context(), top, all)
			sp.stopSpinner()
			if err != nil {
				return fmt.Errorf("listing inbox: %w", err)
			}
			return writeMessages(cmd.OutOrStdout(), msgs, asJSON)
		},
	}
	cmd.Flags().IntVar(&top, "top", 25, "maximum number of messages to list")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	cmd.Flags().BoolVar(&all, "all", false, "list all mail instead of only the inbox")
	return cmd
}

func newMailViewCmd(factory Factory) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "view <message-id>",
		Short: "Show message metadata and body",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			providers, cleanup, sp, err := build(cmd, factory)
			if err != nil {
				return err
			}
			defer cleanup()
			sp.setMessages("Fetching message…")
			defer sp.stopSpinner()

			message, err := providers.Mail.GetMessage(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("getting message %s: %w", args[0], err)
			}
			body, err := providers.Mail.Body(cmd.Context(), args[0])
			if err != nil {
				return fmt.Errorf("reading body for message %s: %w", args[0], err)
			}
			return writeMessageDetail(cmd.OutOrStdout(), mail.NewDetail(message, body), asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}

func newMailArchiveCmd(factory Factory) *cobra.Command {
	var fromStdin bool
	cmd := &cobra.Command{
		Use:   "archive [message-id...]",
		Short: "Archive one or more messages by id",
		RunE: func(cmd *cobra.Command, args []string) error {
			ids := append([]string{}, args...)
			if fromStdin {
				sc := bufio.NewScanner(cmd.InOrStdin())
				for sc.Scan() {
					if id := strings.TrimSpace(sc.Text()); id != "" {
						ids = append(ids, id)
					}
				}
				if err := sc.Err(); err != nil {
					return fmt.Errorf("reading ids from stdin: %w", err)
				}
			}
			if len(ids) == 0 {
				return fmt.Errorf("no message ids provided (pass ids as args or use --stdin)")
			}
			providers, cleanup, sp, err := build(cmd, factory)
			if err != nil {
				return err
			}
			defer cleanup()
			sp.setMessages("Archiving…")
			for _, id := range ids {
				if err := providers.Mail.Archive(cmd.Context(), id); err != nil {
					sp.stopSpinner()
					return fmt.Errorf("archiving %s: %w", id, err)
				}
			}
			sp.stopSpinner()
			for _, id := range ids {
				fmt.Fprintf(cmd.OutOrStdout(), "archived %s\n", id)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fromStdin, "stdin", false, "read message ids from stdin (one per line)")
	return cmd
}
