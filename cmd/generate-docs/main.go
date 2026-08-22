// Command generate-docs writes release-ready shell completions and manpages.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra/doc"
	"github.com/t33n-software/git-governance/internal/bootstrap"
)

var (
	exitProcess   = os.Exit
	commandArgs   = os.Args
	generateFiles = generate
	generateMan   = doc.GenManTree
	version       = "devel"
)

func main() {
	exitProcess(run(commandArgs[1:], os.Stdout, os.Stderr))
}

func run(arguments []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("generate-docs", flag.ContinueOnError)
	flags.SetOutput(stderr)
	output := flags.String("out", ".build/generated", "output directory for completions and manpages")
	showVersion := flags.Bool("version", false, "print the tool version and exit")
	if err := flags.Parse(arguments); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "generate-docs %s\n", version)
		return 0
	}
	if err := generateFiles(*output); err != nil {
		fmt.Fprintln(stderr, "generate documentation:", err)
		return 1
	}
	return 0
}

func generate(output string) error {
	if output == "" {
		return fmt.Errorf("output directory is required")
	}
	completionDirectory := filepath.Join(output, "completions")
	manDirectory := filepath.Join(output, "man")
	if err := os.MkdirAll(completionDirectory, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(manDirectory, 0o755); err != nil {
		return err
	}

	command := bootstrap.New(bootstrap.BuildInfo{
		Version: "generated",
		Commit:  "generated",
		Date:    "generated",
	})
	generators := []struct {
		name string
		run  func() error
	}{
		{
			name: "bash",
			run: func() error {
				file, err := os.Create(filepath.Join(completionDirectory, "git-governance.bash"))
				if err != nil {
					return err
				}
				defer file.Close()
				return command.GenBashCompletion(file)
			},
		},
		{
			name: "zsh",
			run: func() error {
				file, err := os.Create(filepath.Join(completionDirectory, "_git-governance"))
				if err != nil {
					return err
				}
				defer file.Close()
				return command.GenZshCompletion(file)
			},
		},
		{
			name: "fish",
			run: func() error {
				file, err := os.Create(filepath.Join(completionDirectory, "git-governance.fish"))
				if err != nil {
					return err
				}
				defer file.Close()
				return command.GenFishCompletion(file, true)
			},
		},
		{
			name: "powershell",
			run: func() error {
				file, err := os.Create(filepath.Join(completionDirectory, "git-governance.ps1"))
				if err != nil {
					return err
				}
				defer file.Close()
				return command.GenPowerShellCompletionWithDesc(file)
			},
		},
	}
	for _, generator := range generators {
		if err := generator.run(); err != nil {
			return fmt.Errorf("generate %s completion: %w", generator.name, err)
		}
	}

	header := &doc.GenManHeader{
		Title:   "GIT-GOVERNANCE",
		Section: "1",
		Source:  "git-governance",
		Manual:  "git-governance manual",
	}
	if err := generateMan(command, header, manDirectory); err != nil {
		return fmt.Errorf("generate manpages: %w", err)
	}
	return nil
}
