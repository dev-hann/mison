package cli

import (
	"github.com/spf13/cobra"

	"github.com/dev-hann/mison/internal/usecase"
)

func newStatusCmd(f *usecase.Flows) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Compare the declaration with installed tools",
		RunE: func(_ *cobra.Command, _ []string) error {
			return f.RunStatus()
		},
	}
}
