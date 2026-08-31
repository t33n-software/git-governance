// Package cliparam is the canonical value-domain register of the CLI: every
// flag and positional-argument concept is described exactly once here, on the
// application layer, and every consumption channel — static help (K1),
// interactive prompts (K2), error contracts (K3), shell completion (K4), and
// the machine-readable discovery endpoint (K5) — renders from these
// descriptors, never from duplicated literals. Value lists are derived from
// the domain registries at call time, and endpoint subsets are produced
// through declared filters on the same sources, never through hand copies.
// Canonical conventions: docs/conventions/cli/value-domain-model.md and
// docs/conventions/cli/single-source-of-truth.md.
package cliparam

import "strings"

// Class names the value class of a flag or positional argument. The class
// determines the binding help duty of the descriptor. Canonical convention:
// docs/conventions/cli/value-domain-model.md.
type Class string

const (
	// ClassClosedEnum is a finite, fixed value set: the help shows the
	// complete list of values accepted by the endpoint.
	ClassClosedEnum Class = "closed-enum"
	// ClassShaped is a grammar template with a fixed skeleton or prefix: the
	// help shows the grammar, a canonical example, and subset rules.
	ClassShaped Class = "shaped"
	// ClassFreeConstrained is free text with validation rules: the help shows
	// the compact rule set including the forbidden and an example.
	ClassFreeConstrained Class = "free-constrained"
	// ClassStructuralReference references paths, refs, or identifiers whose
	// validity is decidable only at runtime: the help shows the form and the
	// resolution rule without promising full prevention.
	ClassStructuralReference Class = "structural-reference"
	// ClassScalarBounded is a number, duration, or size: the help shows the
	// type, unit, value range, and default.
	ClassScalarBounded Class = "scalar-bounded"
	// ClassBooleanSwitch is a switch without a value: the help shows the
	// effect and the default.
	ClassBooleanSwitch Class = "boolean-switch"
	// ClassCompositeToken is a repeatable TOKEN=VALUE form: the help shows the
	// transport form, the token grammar, and an example.
	ClassCompositeToken Class = "composite-token"
	// ClassSecretReference references a secret: secrets are never accepted as
	// value arguments, only as references.
	ClassSecretReference Class = "secret-reference"
)

// Domain is the single canonical description of one CLI value domain.
//
// Values carries the complete accepted set of a closed-enum domain in
// canonical order. Rule carries the validation-enforced constraint text (hard
// rejection); Convention carries the content contract that is not
// machine-enforced. The label law requires both to stay distinguishable for
// the consumer, so HelpText renders them with explicit labels. Prefixes
// carries the static grammar skeleton forms used for shell completion of
// shaped and composite-token domains.
type Domain struct {
	Concept    string
	Class      Class
	Values     []string
	Rule       string
	Convention string
	Example    string
	Prefixes   []string
}

// ValueList renders the complete accepted value set in canonical order using
// the canonical enumeration form "a, b, c, or d".
func (domain Domain) ValueList() string {
	switch len(domain.Values) {
	case 0:
		return ""
	case 1:
		return domain.Values[0]
	case 2:
		return domain.Values[0] + " or " + domain.Values[1]
	default:
		joined := strings.Join(domain.Values[:len(domain.Values)-1], ", ")
		return joined + ", or " + domain.Values[len(domain.Values)-1]
	}
}

// HelpText renders the canonical help body for the domain. The optional
// context extends the concept lead-in for endpoint-specific variants, so call
// sites never copy the rule text itself.
func (domain Domain) HelpText(context string) string {
	head := domain.Concept
	if context != "" {
		head += " " + context
	}
	if domain.Class == ClassClosedEnum {
		head += ": " + domain.ValueList()
	} else if domain.Rule != "" {
		head += ": " + domain.Rule
	}
	sections := []string{head}
	if domain.Class == ClassClosedEnum && domain.Rule != "" {
		sections = append(sections, domain.Rule)
	}
	if domain.Convention != "" {
		sections = append(sections, "convention-violating: "+domain.Convention)
	}
	if domain.Example != "" {
		sections = append(sections, "example: "+domain.Example)
	}
	return strings.Join(sections, "; ")
}

// Complete returns the completion candidates for the domain: the accepted
// values of closed-enum domains and the static prefix forms of shaped or
// composite-token domains, filtered by the typed prefix.
func (domain Domain) Complete(toComplete string) []string {
	combined := make([]string, 0, len(domain.Values)+len(domain.Prefixes))
	combined = append(combined, domain.Values...)
	combined = append(combined, domain.Prefixes...)
	candidates := make([]string, 0, len(combined))
	for _, candidate := range combined {
		if strings.HasPrefix(candidate, toComplete) {
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}
