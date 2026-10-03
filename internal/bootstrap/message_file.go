package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/t33n-software/git-governance/internal/domain/problem"
)

// maxMessageFileBytes is the hard transport ceiling shared by every message
// file flag; the commit-validate message file carries the same bound.
const maxMessageFileBytes = 1 << 20

// resolveMessageFile is the canonical message transport: every multi-line
// message part crosses the CLI boundary exclusively through a file path, and
// this loader is the only implementation of that transport. The path must be
// absolute, reference an existing regular file within the transport ceiling,
// and decode as UTF-8; the content is carried verbatim without any
// normalization. An empty path resolves to an empty message so optional
// flags stay optional, and the mandatory couplings stay endpoint-owned.
// Canonical convention: docs/conventions/cli/message-file-transport.md.
func resolveMessageFile(rawPath, field string) (string, error) {
	if rawPath == "" {
		return "", nil
	}
	if !filepath.IsAbs(rawPath) {
		return "", problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryUsage,
			Field:       field,
			Actual:      rawPath,
			Expected:    "an absolute path",
			Rule:        "message files are passed by absolute path so resolution never depends on the working directory",
			Example:     filepath.Join(os.TempDir(), "message.txt"),
			Remediation: "pass the absolute path of an existing message file",
		})
	}
	info, err := os.Stat(rawPath)
	if err != nil {
		return "", problem.Wrap(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryUsage,
			Field:       field,
			Actual:      rawPath,
			Expected:    "an existing regular file",
			Rule:        "message file existence is resolved at invocation time",
			Remediation: "pass the absolute path of an existing message file",
		}, err)
	}
	if !info.Mode().IsRegular() {
		return "", problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryUsage,
			Field:       field,
			Actual:      rawPath,
			Expected:    "a regular file",
			Rule:        "message files carry plain text, not directories or special files",
			Remediation: "pass the absolute path of an existing message file",
		})
	}
	if info.Size() > maxMessageFileBytes {
		return "", problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryUsage,
			Field:       field,
			Actual:      rawPath,
			Expected:    fmt.Sprintf("at most %d bytes", maxMessageFileBytes),
			Rule:        "message file input is size-bounded",
			Remediation: "reduce the message file size",
		})
	}
	contents, err := os.ReadFile(rawPath)
	if err != nil {
		return "", problem.Wrap(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryUsage,
			Field:       field,
			Actual:      rawPath,
			Expected:    "a readable message file",
			Rule:        "message files are read completely at invocation time",
			Remediation: "check the path and file permissions",
		}, err)
	}
	if !utf8.Valid(contents) {
		return "", problem.New(problem.Details{
			Code:        problem.CodeInvalidInput,
			Category:    problem.CategoryUsage,
			Field:       field,
			Actual:      rawPath,
			Expected:    "a plain UTF-8 text file",
			Rule:        "message files decode as UTF-8 without a silent repair attempt",
			Remediation: "save the message file as UTF-8 text",
		})
	}
	return string(contents), nil
}
