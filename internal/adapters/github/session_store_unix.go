//go:build linux || darwin

package github

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
)

// activeScopePointerAccount is the well-known account label of the
// host-level record that names the client ID of the most recently used
// session. The native-tool platform stores (Linux Secret Service, macOS
// Keychain) cannot enumerate scopes and resolve the recency pointer through
// this record. The Windows DPAPI document store resolves it through its
// document fields instead, so this constant is compiled only for the
// native-tool platforms.
const activeScopePointerAccount = "__git_governance_active_scope__"

// scopeIndexAccount is the well-known account label of the host-level record
// that stores the JSON-encoded list of known client IDs for the host. The
// native-tool platform stores cannot enumerate items, so the index is the
// discovery source for capability-based session selection.
const scopeIndexAccount = "__git_governance_scopes__"

// repositoryBindingAccountPrefix prefixes the account label of the
// host-level records that bind a canonical repository identity to a client
// ID. The full label carries the SHA-256 digest of the repository scope key.
const repositoryBindingAccountPrefix = "__git_governance_repository__"

// repositoryBindingAccount encodes the canonical repository identity into a
// single account label suitable for the native-tool stores.
func repositoryBindingAccount(host, owner, repository string) string {
	digest := sha256.Sum256([]byte(repositoryScopeKey(host, owner, repository)))
	return repositoryBindingAccountPrefix + hex.EncodeToString(digest[:])
}

// addClientIDToScopeIndex returns the index with the client ID added exactly
// once. The second result reports whether the index changed.
func addClientIDToScopeIndex(index []string, clientID string) ([]string, bool) {
	for _, existing := range index {
		if existing == clientID {
			return index, false
		}
	}
	return append(index, clientID), true
}

// removeClientIDFromScopeIndex returns the index without the client ID.
func removeClientIDFromScopeIndex(index []string, clientID string) []string {
	result := make([]string, 0, len(index))
	for _, existing := range index {
		if existing != clientID {
			result = append(result, existing)
		}
	}
	return result
}

// parseScopeIndex decodes the stored scope index record.
func parseScopeIndex(raw []byte) ([]string, error) {
	var clientIDs []string
	if err := json.Unmarshal(raw, &clientIDs); err != nil {
		return nil, errors.New("native GitHub App session scope index has an invalid format")
	}
	sort.Strings(clientIDs)
	return clientIDs, nil
}
