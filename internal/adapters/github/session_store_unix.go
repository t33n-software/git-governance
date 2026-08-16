//go:build linux || darwin

package github

// activeScopePointerAccount is the well-known account label of the
// host-level record that names the client ID of the currently active
// session. The native-tool platform stores (Linux Secret Service, macOS
// Keychain) cannot enumerate scopes and resolve the active session through
// this pointer. The Windows DPAPI document store resolves it through its
// document fields instead, so this constant is compiled only for the
// native-tool platforms.
const activeScopePointerAccount = "__git_governance_active_scope__"
