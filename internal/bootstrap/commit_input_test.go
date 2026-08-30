package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
	"github.com/t33n-software/git-governance/internal/domain/ticket"
)

func TestResolveCommitMessageInputRepresentations(t *testing.T) {
	t.Parallel()

	feature, err := branch.ParseName("feature/ABC-123-add-export")
	if err != nil {
		t.Fatal(err)
	}
	application := newCommitCommandApplication(newCommitCommandGit(t, feature.String()), nil)

	t.Run("requires an explicit family in non-interactive mode", func(t *testing.T) {
		_, err := application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:      feature,
			Description: "add export",
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("rejects an envelope in the subject", func(t *testing.T) {
		_, err := application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:      feature,
			Family:      "feat",
			Description: "feat(ABC-123): add export",
		})
		assertProblemCode(t, err, problem.CodeCommitDescriptionInvalid)
	})

	t.Run("validates descriptions, breaking details, footers, and custom rules", func(t *testing.T) {
		_, err := application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:           feature,
			Family:           "feat",
			DescriptionLabel: "Commit description",
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)

		message, err := application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:               feature,
			Family:               "feat",
			Description:          "replace export contract",
			Breaking:             true,
			BreakingDescription:  "Clients must use the new export endpoint.",
			Body:                 "## Motivation\n\nThe export contract changed.",
			FooterSpecifications: []string{"Refs=#123"},
		})
		if err != nil || !message.IsBreaking() || len(message.Footers()) != 2 {
			t.Fatalf("breaking structured message = (%#v, %v)", message, err)
		}

		validationErr := errors.New("additional validation failed")
		_, err = application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:      feature,
			Family:      "feat",
			Description: "add export",
			Validate: func(commitmsg.Message) error {
				return validationErr
			},
		})
		if !errors.Is(err, validationErr) {
			t.Fatalf("custom validation error = %v, want %v", err, validationErr)
		}

		_, err = application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:      feature,
			Family:      "feat",
			Description: "replace export contract",
			Breaking:    true,
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)

		_, err = application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:      feature,
			Family:      "revert",
			Description: "revert export",
		})
		assertProblemCode(t, err, problem.CodeCommitDescriptionInvalid)

		_, err = application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:               feature,
			Family:               "feat",
			Description:          "add export",
			FooterSpecifications: []string{"invalid"},
		})
		assertProblemCode(t, err, problem.CodeCommitDescriptionInvalid)

		_, err = application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:              feature,
			Family:              "feat",
			Description:         "replace export contract",
			Breaking:            true,
			BreakingDescription: " ",
		})
		assertProblemCode(t, err, problem.CodeCommitDescriptionInvalid)

		revert, err := application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:      feature,
			Family:      "revert",
			Description: "revert export",
			Body:        "Reverts 0123456789abcdef.",
		})
		if err != nil || revert.Header().Type() != commitmsg.TypeRevert {
			t.Fatalf("valid revert = (%#v, %v)", revert, err)
		}

		_, err = application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:      feature,
			Family:      "feat",
			Description: "add export",
			Body:        "invalid\x00body",
		})
		assertProblemCode(t, err, problem.CodeCommitDescriptionInvalid)
	})

	t.Run("rejects an unscoped branch before prompting", func(t *testing.T) {
		main, err := branch.ParseName("main")
		if err != nil {
			t.Fatal(err)
		}
		_, err = application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:      main,
			Family:      "feat",
			Description: "add export",
		})
		assertProblemCode(t, err, problem.CodeSharedLineMutationForbidden)
	})

	t.Run("rejects a mismatched message ticket at the delivery seam", func(t *testing.T) {
		other, err := ticket.ParseID("ABC-124")
		if err != nil {
			t.Fatal(err)
		}
		header, err := commitmsg.NewHeader(commitmsg.TypeFeat, other, "add export", false)
		if err != nil {
			t.Fatal(err)
		}
		mismatched, err := commitmsg.NewMessage(header, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = validateResolvedCommitMessage(commitMessageInput{Branch: feature}, mismatched)
		assertProblemCode(t, err, problem.CodeCommitTicketMismatch)
	})
}

func TestResolveCommitMessageInputPromptErrors(t *testing.T) {
	t.Parallel()

	feature, err := branch.ParseName("feature/ABC-123-add-export")
	if err != nil {
		t.Fatal(err)
	}

	selectErr := errors.New("selection unavailable")
	prompt := &commitCommandPrompt{
		selects: []commitStringReply{{err: selectErr}},
	}
	application := newCommitCommandApplication(newCommitCommandGit(t, feature.String()), prompt)
	enableCommitPrompt(application, prompt)
	_, err = application.resolveCommitMessage(context.Background(), commitMessageInput{
		Branch: feature,
	})
	if !errors.Is(err, selectErr) {
		t.Fatalf("family selection error = %v, want %v", err, selectErr)
	}

	descriptionErr := errors.New("description unavailable")
	prompt = &commitCommandPrompt{
		selects: []commitStringReply{{value: "feat"}},
		inputs:  []commitStringReply{{err: descriptionErr}},
	}
	application = newCommitCommandApplication(newCommitCommandGit(t, feature.String()), prompt)
	enableCommitPrompt(application, prompt)
	_, err = application.resolveCommitMessage(context.Background(), commitMessageInput{
		Branch: feature,
	})
	if !errors.Is(err, descriptionErr) {
		t.Fatalf("description input error = %v, want %v", err, descriptionErr)
	}

	if defaultCommitFamily(commitMessageInput{Branch: feature, DefaultFamily: commitmsg.TypeFix}) != commitmsg.TypeFix {
		t.Fatalf("configured default family was ignored")
	}
	if defaultCommitFamily(commitMessageInput{Branch: feature, DefaultFamily: commitmsg.Type("unknown")}) != commitmsg.TypeFeat {
		t.Fatalf("invalid configured default family was accepted")
	}
}
