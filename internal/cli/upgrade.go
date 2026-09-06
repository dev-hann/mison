package cli

import (
	"github.com/spf13/cobra"

	"github.com/dev-hann/mison/internal/usecase"
)

func newUpgradeCmd(f *usecase.Flows, version string) *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade the binaries mison depends on: mison + mise",
		RunE: func(_ *cobra.Command, _ []string) error {
			return f.RunUpgrade(version)
		},
	}
}
