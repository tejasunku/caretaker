package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (a *App) scanCmd() *cobra.Command {
	var imageRef, output string
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Generate a CycloneDX SBOM for an image with Trivy",
		RunE: func(cmd *cobra.Command, args []string) error {
			if imageRef == "" {
				return errString("--image is required")
			}
			if output == "" {
				output = "sbom.cdx.json"
			}
			if err := a.patcher(patchOptions{}).Scan(cmd.Context(), imageRef, output); err != nil {
				return err
			}
			fmt.Fprintf(a.Out, "wrote %s\n", output)
			return nil
		},
	}
	cmd.Flags().StringVar(&imageRef, "image", "", "image reference to scan (required)")
	cmd.Flags().StringVar(&output, "output", "sbom.cdx.json", "output file")
	return cmd
}
