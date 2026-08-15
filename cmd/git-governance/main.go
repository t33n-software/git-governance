package main

import (
	"context"
	"os"

	"github.com/spf13/cobra"
	"github.com/t33n-software/git-governance/internal/bootstrap"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

var (
	version      = "devel"
	commit       = "unknown"
	date         = "unknown"
	exitProcess  = os.Exit
	buildCommand = newCommand
)

func main() {
	exitProcess(execute(context.Background(), buildCommand()))
}

func execute(ctx context.Context, command *cobra.Command) int {
	if err := command.ExecuteContext(ctx); err != nil {
		bootstrap.RenderError(command, err)
		return problem.ExitCode(err)
	}
	return problem.ExitSuccess
}

func newCommand() *cobra.Command {
	return bootstrap.New(bootstrap.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	})
}
