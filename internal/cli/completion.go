package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// completionCmd generates shell completion scripts for Raven.
var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for Raven.

To install completions:

  Bash (Linux):
    raven completion bash | sudo tee /etc/bash_completion.d/raven > /dev/null

  Bash (macOS with Homebrew):
    raven completion bash > $(brew --prefix)/etc/bash_completion.d/raven

  Zsh:
    raven completion zsh > "${fpath[1]}/_raven"
    # or
    raven completion zsh > ~/.zsh/completions/_raven

  Fish:
    raven completion fish > ~/.config/fish/completions/raven.fish

  PowerShell:
    raven completion powershell > raven.ps1
    # Then add ". raven.ps1" to your PowerShell profile`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletionV2(os.Stdout, true)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		default:
			return fmt.Errorf("unsupported shell: %s", args[0])
		}
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
