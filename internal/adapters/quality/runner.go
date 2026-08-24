// Package quality runs explicitly configured repository-local quality gates.
package quality

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/t33n-software/git-governance/internal/application/port"
	"github.com/t33n-software/git-governance/internal/domain/branch"
	"github.com/t33n-software/git-governance/internal/domain/problem"
)

const (
	defaultConfigName = "git-governance.quality.json"
	currentSchema     = 4
	maxConfigBytes    = 1 << 20
	maxGateCount      = 32
	maxArgumentCount  = 64
	maxBinaryCount    = 32
	maxSmokeCount     = 16
	maxFuzzCount      = 64
	maxExtendsCount   = 32
	defaultTimeout    = 5 * time.Minute
)

var (
	gateNamePattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	languagePattern       = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	versionPattern        = regexp.MustCompile(`^[0-9]+\.[0-9]+(\.[0-9]+)?$`)
	packReferencePattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*@[0-9]+$`)
	projectPackagePattern = regexp.MustCompile(`^\.?/[A-Za-z0-9_./-]+$`)
	fuzzTargetPattern     = regexp.MustCompile(`^Fuzz[A-Za-z0-9_]*$`)
	defaultFamilies       = []branch.Family{
		branch.FamilyFeature,
		branch.FamilyFix,
		branch.FamilyDocs,
		branch.FamilyRefactor,
		branch.FamilyChore,
		branch.FamilyTest,
		branch.FamilyPerf,
		branch.FamilyHotfix,
	}
)

type commandRunner func(ctx context.Context, directory, executable string, arguments ...string) error

// Options configures a generic quality runner. It deliberately accepts command
// arrays, never shell command strings.
type Options struct {
	Path           string
	DefaultTimeout time.Duration
	ReadFile       func(string) ([]byte, error)
	Run            commandRunner
	Diagnostic     io.Writer
}

// Runner executes a trusted repository's explicitly declared local quality
// gates. Repositories with no config are reported as unconfigured, not passed.
type Runner struct {
	path           string
	defaultTimeout time.Duration
	readFile       func(string) ([]byte, error)
	run            commandRunner
}

// New creates a repository-local quality runner.
func New(options Options) *Runner {
	timeout := options.DefaultTimeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	readFile := options.ReadFile
	if readFile == nil {
		readFile = os.ReadFile
	}
	run := options.Run
	if run == nil {
		diagnostic := options.Diagnostic
		if diagnostic == nil {
			diagnostic = os.Stderr
		}
		run = func(ctx context.Context, directory, executable string, arguments ...string) error {
			return runCommand(diagnostic, ctx, directory, executable, arguments...)
		}
	}
	return &Runner{
		path:           options.Path,
		defaultTimeout: timeout,
		readFile:       readFile,
		run:            run,
	}
}

// Run loads and executes gates selected by the request's branch families. The
// config file is an explicit trust boundary: invoking project tooling may
// execute arbitrary project code, so the runner neither guesses commands nor
// interprets shell syntax.
func (runner *Runner) Run(
	ctx context.Context,
	repository port.RepositoryIdentity,
	request port.QualityRequest,
) (port.QualityResult, error) {
	result, _, err := runner.RunWithFingerprint(ctx, repository, request)
	return result, err
}

// RunWithFingerprint executes one quality configuration snapshot and returns
// the fingerprint of that exact snapshot for revision-bound publish evidence.
func (runner *Runner) RunWithFingerprint(
	ctx context.Context,
	repository port.RepositoryIdentity,
	request port.QualityRequest,
) (port.QualityResult, port.QualityFingerprint, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	configuration, contents, requestedFamilies, configured, err := runner.selectedConfiguration(ctx, repository, request)
	if err != nil {
		return port.QualityResult{}, port.QualityFingerprint{}, err
	}
	if !configured {
		return port.QualityResult{
				Status: port.QualityUnconfigured,
				Detail: "no repository-local quality configuration is present",
			},
			port.QualityFingerprint{Toolchain: qualityToolchainIdentity()},
			nil
	}
	selected := selectedGates(configuration, requestedFamilies)
	fingerprint := fingerprintFor(contents, selected)
	result := port.QualityResult{
		Status: port.QualityPassed,
		Detail: "all applicable repository-local quality gates passed",
		Gates:  make([]port.QualityGateResult, 0, len(selected)),
	}
	for _, gate := range selected {
		// decode has already validated both values. Re-evaluating the same
		// deterministic helpers for the actual repository root cannot fail.
		directory, _ := resolveWorkingDirectory(repository.Root, gate.WorkingDirectory)
		timeout, _ := gateTimeout(gate.Timeout, runner.defaultTimeout)
		gateContext, cancel := context.WithTimeout(ctx, timeout)
		err = runner.run(gateContext, directory, gate.Command, gate.Args...)
		cancel()
		if err != nil {
			return port.QualityResult{}, port.QualityFingerprint{}, problem.Wrap(problem.Details{
				Code:        problem.CodeExternalCommandFailed,
				Category:    problem.CategoryExternal,
				Field:       "quality gate",
				Actual:      gate.Name,
				Expected:    "a successful configured quality command",
				Rule:        "each configured quality gate must pass before publication-affecting work continues",
				Example:     `{"name":"unit-tests","command":"go","args":["test","./..."],"timeout":"2m"}`,
				Remediation: "fix the reported project quality failure, adjust the trusted configuration, or use an explicitly documented skip policy",
			}, err)
		}
		result.Gates = append(result.Gates, port.QualityGateResult{Name: gate.Name})
	}
	if len(result.Gates) == 0 {
		return port.QualityResult{
				Status: port.QualitySkipped,
				Detail: "no configured quality gates apply to the selected branch families",
			},
			fingerprint,
			nil
	}
	return result, fingerprint, nil
}

// Fingerprint reads the current selected quality configuration without running
// commands so a pre-push check can reject stale local evidence.
func (runner *Runner) Fingerprint(
	ctx context.Context,
	repository port.RepositoryIdentity,
	request port.QualityRequest,
) (port.QualityFingerprint, error) {
	configuration, contents, requestedFamilies, configured, err := runner.selectedConfiguration(ctx, repository, request)
	if err != nil {
		return port.QualityFingerprint{}, err
	}
	if !configured {
		return port.QualityFingerprint{Toolchain: qualityToolchainIdentity()}, nil
	}
	return fingerprintFor(contents, selectedGates(configuration, requestedFamilies)), nil
}

func (runner *Runner) selectedConfiguration(
	ctx context.Context,
	repository port.RepositoryIdentity,
	request port.QualityRequest,
) (config, []byte, []branch.Family, bool, error) {
	if repository.Root == "" {
		return config{}, nil, nil, false, problem.New(problem.Details{
			Code:        problem.CodeRepositoryNotFound,
			Category:    problem.CategoryRepository,
			Field:       "repository",
			Expected:    "a discovered repository root for quality configuration",
			Rule:        "quality gates run only inside an explicit repository",
			Remediation: "run from a Git repository or pass --repo",
		})
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return config{}, nil, nil, false, cancelled(err)
	}
	requestedFamilies, err := normalizeRequestedFamilies(request.Families)
	if err != nil {
		return config{}, nil, nil, false, err
	}

	path := runner.configPath(repository.Root)
	contents, err := runner.readFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return config{}, nil, requestedFamilies, false, nil
	}
	if err != nil {
		return config{}, nil, nil, false, unavailable(path, "read quality configuration", err)
	}
	if len(contents) > maxConfigBytes {
		return config{}, nil, nil, false, invalid(path, "quality configuration must not exceed 1 MiB", nil)
	}

	configuration, err := decode(path, contents)
	if err != nil {
		return config{}, nil, nil, false, err
	}
	return configuration, contents, requestedFamilies, true, nil
}

func selectedGates(configuration config, requestedFamilies []branch.Family) []gate {
	selected := make([]gate, 0, len(configuration.Gates))
	for _, gate := range configuration.Gates {
		if gateApplies(configuration.Defaults, gate, requestedFamilies) {
			selected = append(selected, gate)
		}
	}
	return selected
}

func fingerprintFor(contents []byte, selected []gate) port.QualityFingerprint {
	sum := sha256.Sum256(contents)
	gates := make([]string, 0, len(selected))
	for _, gate := range selected {
		gates = append(gates, gate.Name)
	}
	return port.QualityFingerprint{
		ConfigurationDigest: hex.EncodeToString(sum[:]),
		Gates:               gates,
		Toolchain:           qualityToolchainIdentity(),
	}
}

func qualityToolchainIdentity() string {
	return runtime.Version() + "/" + runtime.GOOS + "/" + runtime.GOARCH
}

func (runner *Runner) configPath(root string) string {
	if runner.path == "" {
		return filepath.Join(root, defaultConfigName)
	}
	if filepath.IsAbs(runner.path) {
		return filepath.Clean(runner.path)
	}
	return filepath.Join(root, filepath.Clean(runner.path))
}

type config struct {
	SchemaVersion int          `json:"schemaVersion"`
	Toolchain     toolchain    `json:"toolchain"`
	Extends       []string     `json:"extends,omitempty"`
	Defaults      familyScope  `json:"defaults,omitempty"`
	Gates         []gate       `json:"gates"`
	Project       projectScope `json:"project,omitempty"`
}

type toolchain struct {
	Language string `json:"language"`
	Version  string `json:"version"`
}

type projectScope struct {
	Binaries []projectBinary `json:"binaries,omitempty"`
	Fuzz     []projectFuzz   `json:"fuzz,omitempty"`
}

type projectBinary struct {
	Package string   `json:"package"`
	Smoke   []string `json:"smoke,omitempty"`
}

type projectFuzz struct {
	Package string `json:"package"`
	Target  string `json:"target"`
	Time    string `json:"time"`
}

type familyScope struct {
	IncludeFamilies []branch.Family `json:"includeFamilies,omitempty"`
	ExcludeFamilies []branch.Family `json:"excludeFamilies,omitempty"`
}

type gate struct {
	Name             string          `json:"name"`
	Command          string          `json:"command"`
	Args             []string        `json:"args"`
	Timeout          string          `json:"timeout"`
	WorkingDirectory string          `json:"workingDirectory,omitempty"`
	IncludeFamilies  []branch.Family `json:"includeFamilies,omitempty"`
	ExcludeFamilies  []branch.Family `json:"excludeFamilies,omitempty"`
}

func decode(path string, contents []byte) (config, error) {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var value config
	if err := decoder.Decode(&value); err != nil {
		return config{}, invalid(path, "quality configuration must contain valid JSON with known fields", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return config{}, invalid(path, "quality configuration must contain exactly one JSON document", nil)
	}
	if value.SchemaVersion != currentSchema {
		return config{}, invalid(path, "schemaVersion must equal 4", nil)
	}
	if !languagePattern.MatchString(value.Toolchain.Language) {
		return config{}, invalid(path, "toolchain language must be a lowercase language identifier such as go", nil)
	}
	if !versionPattern.MatchString(value.Toolchain.Version) {
		return config{}, invalid(path, "toolchain version must be a pinned version such as 1.26.6", nil)
	}
	if err := validateExtends(path, value.Extends); err != nil {
		return config{}, err
	}
	if len(value.Gates) == 0 || len(value.Gates) > maxGateCount {
		return config{}, invalid(path, "gates must contain between 1 and 32 entries", nil)
	}
	if err := validateScope(path, "defaults", value.Defaults); err != nil {
		return config{}, err
	}
	seen := make(map[string]struct{}, len(value.Gates))
	for _, gate := range value.Gates {
		if !gateNamePattern.MatchString(gate.Name) {
			return config{}, invalid(path, "gate names must use lowercase letters, digits, hyphens, or underscores", nil)
		}
		if _, found := seen[gate.Name]; found {
			return config{}, invalid(path, "gate names must be unique", nil)
		}
		seen[gate.Name] = struct{}{}
		if strings.TrimSpace(gate.Command) == "" || strings.ContainsAny(gate.Command, "\r\n") {
			return config{}, invalid(path, "each gate command must be a non-empty executable name or path", nil)
		}
		if len(gate.Args) > maxArgumentCount {
			return config{}, invalid(path, "each gate may contain at most 64 arguments", nil)
		}
		for _, argument := range gate.Args {
			if strings.ContainsAny(argument, "\x00\r\n") {
				return config{}, invalid(path, "gate arguments cannot contain NUL or line-control characters", nil)
			}
		}
		if _, err := resolveWorkingDirectory(".", gate.WorkingDirectory); err != nil {
			return config{}, err
		}
		if _, err := gateTimeout(gate.Timeout, defaultTimeout); err != nil {
			return config{}, invalid(path, "gate "+gate.Name+" has an invalid timeout", err)
		}
		if err := validateScope(path, "gate "+gate.Name, gate.scope()); err != nil {
			return config{}, err
		}
	}
	if err := validateProject(path, value.Project); err != nil {
		return config{}, err
	}
	return value, nil
}

func (gate gate) scope() familyScope {
	return familyScope{
		IncludeFamilies: gate.IncludeFamilies,
		ExcludeFamilies: gate.ExcludeFamilies,
	}
}

func validateScope(path, label string, scope familyScope) error {
	included := make(map[branch.Family]struct{}, len(scope.IncludeFamilies))
	for _, family := range scope.IncludeFamilies {
		if !family.IsKnown() {
			return invalid(path, label+" includes an unknown branch family "+family.String(), nil)
		}
		if _, found := included[family]; found {
			return invalid(path, label+" cannot include the same branch family more than once", nil)
		}
		included[family] = struct{}{}
	}

	excluded := make(map[branch.Family]struct{}, len(scope.ExcludeFamilies))
	for _, family := range scope.ExcludeFamilies {
		if !family.IsKnown() {
			return invalid(path, label+" excludes an unknown branch family "+family.String(), nil)
		}
		if _, found := excluded[family]; found {
			return invalid(path, label+" cannot exclude the same branch family more than once", nil)
		}
		if _, found := included[family]; found {
			return invalid(path, label+" cannot both include and exclude "+family.String(), nil)
		}
		excluded[family] = struct{}{}
	}
	return nil
}

// validateExtends checks the capability pack declaration form. The runner
// validates the reference grammar only; resolving a pack against the registry
// is the quality-gate toolchain's responsibility.
func validateExtends(path string, references []string) error {
	if len(references) > maxExtendsCount {
		return invalid(path, "extends must contain at most 32 capability pack references", nil)
	}
	seen := make(map[string]struct{}, len(references))
	for _, reference := range references {
		if !packReferencePattern.MatchString(reference) {
			return invalid(path, "extends entries must use the <capability>@<major> form such as opentofu@1", nil)
		}
		if _, found := seen[reference]; found {
			return invalid(path, "extends entries must be unique", nil)
		}
		seen[reference] = struct{}{}
	}
	return nil
}

func validateProject(path string, project projectScope) error {
	if len(project.Binaries) > maxBinaryCount {
		return invalid(path, "project binaries must contain at most 32 entries", nil)
	}
	seenBinaries := make(map[string]struct{}, len(project.Binaries))
	for _, binary := range project.Binaries {
		if !projectPackagePattern.MatchString(binary.Package) {
			return invalid(path, "project binaries package must be a repository-relative package path", nil)
		}
		if _, found := seenBinaries[binary.Package]; found {
			return invalid(path, "project binaries package must be unique", nil)
		}
		seenBinaries[binary.Package] = struct{}{}
		if len(binary.Smoke) > maxSmokeCount {
			return invalid(path, "project binaries may contain at most 16 smoke arguments", nil)
		}
		for _, argument := range binary.Smoke {
			if strings.ContainsAny(argument, "\x00\r\n") {
				return invalid(path, "project binary smoke arguments cannot contain NUL or line-control characters", nil)
			}
		}
	}
	if len(project.Fuzz) > maxFuzzCount {
		return invalid(path, "project fuzz must contain at most 64 entries", nil)
	}
	seenFuzz := make(map[string]struct{}, len(project.Fuzz))
	for _, target := range project.Fuzz {
		if !projectPackagePattern.MatchString(target.Package) {
			return invalid(path, "project fuzz package must be a repository-relative package path", nil)
		}
		if !fuzzTargetPattern.MatchString(target.Target) {
			return invalid(path, "project fuzz target must be a Fuzz function name", nil)
		}
		key := target.Package + "|" + target.Target
		if _, found := seenFuzz[key]; found {
			return invalid(path, "project fuzz target must be unique", nil)
		}
		seenFuzz[key] = struct{}{}
		if err := fuzzTime(target.Time); err != nil {
			return invalid(path, "project fuzz "+target.Target+" has an invalid time budget", err)
		}
	}
	return nil
}

func fuzzTime(raw string) error {
	if raw == "" {
		return errors.New("fuzz time budget must not be empty")
	}
	if strings.HasSuffix(raw, "x") {
		count := strings.TrimSuffix(raw, "x")
		if count == "" {
			return errors.New("fuzz execution count must not be empty")
		}
		for _, r := range count {
			if r < '0' || r > '9' {
				return fmt.Errorf("fuzz execution count must be numeric: %q", raw)
			}
		}
		return nil
	}
	if _, err := time.ParseDuration(raw); err != nil {
		return fmt.Errorf("fuzz time budget must be a positive Go duration or an execution count: %q", raw)
	}
	return nil
}

func normalizeRequestedFamilies(families []branch.Family) ([]branch.Family, error) {
	result := make([]branch.Family, 0, len(families))
	seen := make(map[branch.Family]struct{}, len(families))
	for _, family := range families {
		if !family.IsKnown() {
			return nil, problem.New(problem.Details{
				Code:        problem.CodeInvalidInput,
				Category:    problem.CategoryGovernance,
				Field:       "quality branch family",
				Actual:      family.String(),
				Expected:    "a supported branch family",
				Rule:        "quality-gate selection uses canonical branch families",
				Example:     "feature",
				Remediation: "pass branch families parsed from a governed branch name",
			})
		}
		if _, found := seen[family]; found {
			continue
		}
		seen[family] = struct{}{}
		result = append(result, family)
	}
	return result, nil
}

func gateApplies(defaults familyScope, gate gate, requested []branch.Family) bool {
	if len(requested) == 0 {
		return false
	}
	eligible := effectiveFamilies(defaults, gate.scope())
	for _, requestedFamily := range requested {
		for _, eligibleFamily := range eligible {
			if requestedFamily == eligibleFamily {
				return true
			}
		}
	}
	return false
}

func effectiveFamilies(defaults, override familyScope) []branch.Family {
	included := defaults.IncludeFamilies
	if len(included) == 0 {
		included = defaultFamilies
	}
	if len(override.IncludeFamilies) > 0 {
		included = override.IncludeFamilies
	}

	excluded := make(map[branch.Family]struct{}, len(defaults.ExcludeFamilies)+len(override.ExcludeFamilies))
	for _, family := range defaults.ExcludeFamilies {
		excluded[family] = struct{}{}
	}
	for _, family := range override.ExcludeFamilies {
		excluded[family] = struct{}{}
	}

	result := make([]branch.Family, 0, len(included))
	for _, family := range included {
		if _, found := excluded[family]; !found {
			result = append(result, family)
		}
	}
	return result
}

func resolveWorkingDirectory(root, relative string) (string, error) {
	if relative == "" {
		relative = "."
	}
	if filepath.IsAbs(relative) {
		return "", problem.New(problem.Details{
			Code:        problem.CodeConfigurationInvalid,
			Category:    problem.CategoryConfig,
			Field:       "quality gate workingDirectory",
			Actual:      relative,
			Expected:    "a path relative to the repository root",
			Rule:        "quality gate working directories cannot escape the selected repository",
			Remediation: "use . or a relative descendant path",
		})
	}
	clean := filepath.Clean(relative)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", problem.New(problem.Details{
			Code:        problem.CodeConfigurationInvalid,
			Category:    problem.CategoryConfig,
			Field:       "quality gate workingDirectory",
			Actual:      relative,
			Expected:    "a path inside the repository root",
			Rule:        "quality gate working directories cannot escape the selected repository",
			Remediation: "use . or a relative descendant path",
		})
	}
	return filepath.Join(root, clean), nil
}

func gateTimeout(raw string, fallback time.Duration) (time.Duration, error) {
	if raw == "" {
		return fallback, nil
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout <= 0 {
		return 0, errors.New("timeout must be a positive Go duration")
	}
	return timeout, nil
}

func runCommand(diagnostic io.Writer, ctx context.Context, directory, executable string, arguments ...string) error {
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Dir = directory
	command.Stdout = diagnostic
	command.Stderr = diagnostic
	return command.Run()
}

func cancelled(cause error) error {
	return problem.Wrap(problem.Details{
		Code:        problem.CodeOperationCancelled,
		Category:    problem.CategoryCancelled,
		Field:       "quality gates",
		Expected:    "an active context",
		Rule:        "quality gate execution stops when the caller cancels its context",
		Remediation: "retry with an active context",
	}, cause)
}

func unavailable(path, action string, cause error) error {
	return problem.Wrap(problem.Details{
		Code:        problem.CodeConfigurationUnavailable,
		Category:    problem.CategoryConfig,
		Field:       "quality configuration",
		Actual:      path,
		Expected:    "an accessible repository-local quality configuration",
		Rule:        action,
		Remediation: "check the configuration path and filesystem permissions",
	}, cause)
}

func invalid(path, rule string, cause error) error {
	return problem.Wrap(problem.Details{
		Code:        problem.CodeConfigurationInvalid,
		Category:    problem.CategoryConfig,
		Field:       "quality configuration",
		Actual:      path,
		Expected:    "a valid git-governance.quality.json document",
		Rule:        rule,
		Example:     `{"schemaVersion":4,"toolchain":{"language":"go","version":"1.26.6"},"gates":[{"name":"unit-tests","command":"go","args":["test","./..."],"timeout":"2m"}]}`,
		Remediation: "correct the repository-local quality configuration",
	}, cause)
}

var _ port.QualityRunner = (*Runner)(nil)
var _ port.QualityEvidenceRunner = (*Runner)(nil)
