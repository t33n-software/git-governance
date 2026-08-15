package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/commitmsg"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

func TestResolveCommitMessageInputRepresentations(t *testing.T) {
	t.Parallel()

	feature, err := branch.ParseName("feature/ABC-123-add-export")
	if err != nil {
		t.Fatal(err)
	}
	application := newCommitCommandApplication(newCommitCommandGit(t, feature.String()), nil)

	t.Run("accepts a complete compatibility message", func(t *testing.T) {
		message, err := application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:          feature,
			CompleteMessage: "feat(ABC-123): add export",
		})
		if err != nil || message.Header().String() != "feat(ABC-123): add export" {
			t.Fatalf("resolve complete message = (%q, %v)", message.String(), err)
		}

		_, err = application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:          feature,
			CompleteMessage: "feat(ABC-124): wrong ticket",
		})
		assertProblemCode(t, err, problem.CodeCommitTicketMismatch)
	})

	t.Run("rejects mixed complete and structured input", func(t *testing.T) {
		_, err := application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:          feature,
			CompleteMessage: "feat(ABC-123): add export",
			Family:          "feat",
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("uses the branch-derived default only when allowed", func(t *testing.T) {
		message, err := application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:      feature,
			Description: "add export",
		})
		if err != nil || message.Header().Type() != commitmsg.TypeFeat {
			t.Fatalf("default structured message = (%q, %v)", message.String(), err)
		}

		_, err = application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:        feature,
			Description:   "add export",
			RequireFamily: true,
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)
	})

	t.Run("validates descriptions, breaking details, footers, and custom rules", func(t *testing.T) {
		_, err := application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:           feature,
			Family:           "feat",
			RequireFamily:    true,
			DescriptionLabel: "Commit description",
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)

		message, err := application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:               feature,
			Family:               "feat",
			Description:          "replace export contract",
			Breaking:             true,
			BreakingDescription:  "Clients must use the new export endpoint.",
			FooterSpecifications: []string{"Refs=#123"},
			RequireFamily:        true,
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
			Branch:        feature,
			Family:        "feat",
			Description:   "replace export contract",
			Breaking:      true,
			RequireFamily: true,
		})
		assertProblemCode(t, err, problem.CodeInvalidInput)

		_, err = application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:        feature,
			Family:        "revert",
			Description:   "revert export",
			RequireFamily: true,
		})
		assertProblemCode(t, err, problem.CodeCommitDescriptionInvalid)

		_, err = application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:               feature,
			Family:               "feat",
			Description:          "add export",
			FooterSpecifications: []string{"invalid"},
			RequireFamily:        true,
		})
		assertProblemCode(t, err, problem.CodeCommitDescriptionInvalid)

		_, err = application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:              feature,
			Family:              "feat",
			Description:         "replace export contract",
			Breaking:            true,
			BreakingDescription: " ",
			RequireFamily:       true,
		})
		assertProblemCode(t, err, problem.CodeCommitDescriptionInvalid)

		revert, err := application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:        feature,
			Family:        "revert",
			Description:   "revert export",
			Body:          "Reverts 0123456789abcdef.",
			RequireFamily: true,
		})
		if err != nil || revert.Header().Type() != commitmsg.TypeRevert {
			t.Fatalf("valid revert = (%#v, %v)", revert, err)
		}

		_, err = application.resolveCommitMessage(context.Background(), commitMessageInput{
			Branch:        feature,
			Family:        "feat",
			Description:   "add export",
			Body:          "invalid\x00body",
			RequireFamily: true,
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
		Branch:        feature,
		RequireFamily: true,
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
		Branch:        feature,
		RequireFamily: true,
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
