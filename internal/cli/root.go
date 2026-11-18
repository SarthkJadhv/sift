package cli

import "github.com/spf13/cobra"

func Execute() error {

	rootCmd := &cobra.Command{
		Use:   "sift",
		Short: "Context intelligence and token economics for AI coding agents",
		Long: `SIFT is a local-first developer tool that analyzes
AI coding agent activity, measures token consumption,
and provides context optimization recommendations.`,
	}

	rootCmd.AddCommand(newVersionCommand())
	rootCmd.AddCommand(newInitCommand())
	rootCmd.AddCommand(newListCommand())
	rootCmd.AddCommand(newDoctorCommand())
	rootCmd.AddCommand(newCodexCommand())
	return rootCmd.Execute()

}
