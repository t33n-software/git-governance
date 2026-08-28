// Package tools pins the governed tool and registry modules of this
// repository. The blank import of the shared kernel's registry anchor keeps
// the capability-pack registry in the build list, so the pack resolution
// resolves it at the pinned stand under go mod tidy.
package tools

import _ "github.com/t33n-software/supply-chain-governance/capabilities"
