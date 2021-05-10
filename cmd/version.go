package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	Revision = "dev"
	Version  = "dev"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Version of rtty",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf(`Version: %s
Revision: %s
OS: %s
Arch: %s
`, Version, Revision, runtime.GOOS, runtime.GOARCH)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
