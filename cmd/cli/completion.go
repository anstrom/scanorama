// Package cli provides command-line interface commands for the Scanorama network scanner.
// This package implements the Cobra-based CLI structure with commands for scanning,
// discovery, host management, scheduling, and daemon operations.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const (
	// Shell completion constants.
	bashCompletionLongDesc = `Generate the autocompletion script for bash.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(scanorama completion bash)

To load completions for every new session, execute once:

#### Linux:

	scanorama completion bash > /etc/bash_completion.d/scanorama

#### macOS:

	scanorama completion bash > $(brew --prefix)/etc/bash_completion.d/scanorama

You will need to start a new shell for this setup to take effect.`

	zshCompletionLongDesc = `Generate the autocompletion script for zsh.

If shell completion is not already enabled in your environment you will need
to enable it. You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(scanorama completion zsh)

To load completions for every new session, execute once:

#### Linux:

	scanorama completion zsh > "${fpath[1]}/_scanorama"

#### macOS:

	scanorama completion zsh > $(brew --prefix)/share/zsh/site-functions/_scanorama

You will need to start a new shell for this setup to take effect.`

	fishCompletionLongDesc = `Generate the autocompletion script for fish.

To load completions in your current shell session:

	scanorama completion fish | source

To load completions for every new session, execute once:

	scanorama completion fish > ~/.config/fish/completions/scanorama.fish

You will need to start a new shell for this setup to take effect.`

	powershellCompletionLongDesc = `Generate the autocompletion script for powershell.

To load completions in your current shell session:

	scanorama completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output of the above command
to your powershell profile.`
)

// completionCmd represents the completion command.
var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for Scanorama.

The completion command generates shell completion scripts for bash, zsh, fish,
and powershell. These scripts enable tab completion for commands, flags, and
arguments including dynamic completion for network names and discovery methods.

Features include:
- Command and subcommand completion
- Flag name and value completion
- Network name completion for network management commands
- Discovery method completion for discovery and network commands
- Host IP completion for host management commands

Choose the appropriate subcommand for your shell.`,
	Example: `  # Generate bash completion
  scanorama completion bash > /etc/bash_completion.d/scanorama

  # Generate zsh completion
  scanorama completion zsh > ~/.zsh/completions/_scanorama

  # Load completion in current session
  source <(scanorama completion bash)`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			if err := cmd.Root().GenBashCompletion(os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Error generating bash completion: %v\n", err)
				os.Exit(1)
			}
		case "zsh":
			if err := cmd.Root().GenZshCompletion(os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Error generating zsh completion: %v\n", err)
				os.Exit(1)
			}
		case "fish":
			if err := cmd.Root().GenFishCompletion(os.Stdout, true); err != nil {
				fmt.Fprintf(os.Stderr, "Error generating fish completion: %v\n", err)
				os.Exit(1)
			}
		case "powershell":
			if err := cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "Error generating powershell completion: %v\n", err)
				os.Exit(1)
			}
		}
	},
}

// bashCompletionCmd generates bash completion script.
var bashCompletionCmd = &cobra.Command{
	Use:                   "bash",
	Short:                 "Generate bash completion script",
	Long:                  bashCompletionLongDesc,
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		if err := cmd.Root().GenBashCompletion(os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating bash completion: %v\n", err)
			os.Exit(1)
		}
	},
}

// zshCompletionCmd generates zsh completion script.
var zshCompletionCmd = &cobra.Command{
	Use:                   "zsh",
	Short:                 "Generate zsh completion script",
	Long:                  zshCompletionLongDesc,
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		if err := cmd.Root().GenZshCompletion(os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating zsh completion: %v\n", err)
			os.Exit(1)
		}
	},
}

// fishCompletionCmd generates fish completion script.
var fishCompletionCmd = &cobra.Command{
	Use:                   "fish",
	Short:                 "Generate fish completion script",
	Long:                  fishCompletionLongDesc,
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		if err := cmd.Root().GenFishCompletion(os.Stdout, true); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating fish completion: %v\n", err)
			os.Exit(1)
		}
	},
}

// powershellCompletionCmd generates powershell completion script.
var powershellCompletionCmd = &cobra.Command{
	Use:                   "powershell",
	Short:                 "Generate powershell completion script",
	Long:                  powershellCompletionLongDesc,
	DisableFlagsInUseLine: true,
	Run: func(cmd *cobra.Command, args []string) {
		if err := cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error generating powershell completion: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
	completionCmd.AddCommand(bashCompletionCmd)
	completionCmd.AddCommand(zshCompletionCmd)
	completionCmd.AddCommand(fishCompletionCmd)
	completionCmd.AddCommand(powershellCompletionCmd)

	// Add shell completion for the completion command itself
	completionCmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return []string{"bash", "zsh", "fish", "powershell"}, cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}
