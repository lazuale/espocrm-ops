package cli

import "github.com/spf13/cobra"

func newBackupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "backup",
		Short: "Backup operations",
	}
}
