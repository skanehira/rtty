package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use: "rtty",
}

func exitError(msg any) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

func Execute() {
	rootCmd.Run = func(cmd *cobra.Command, args []string) {
		_ = rootCmd.Help()
	}

	if err := rootCmd.Execute(); err != nil {
		exitError(err)
	}
}
