package herdr

import (
	"fmt"
	"strings"
)

// tokenNames are the workspace tokens githerd reports. Every report names
// each one: it sets those that apply and clears the others.
var tokenNames = []string{"repos", "changes", "stale"}

// hasAny reports whether toks holds any of names.
func hasAny(toks map[string]string, names []string) bool {
	for _, name := range names {
		if _, ok := toks[name]; ok {
			return true
		}
	}
	return false
}

// rootTokens are the tokens of a root workspace: how many of the root's
// repos need attention, of how many, and how many are stale.
func rootTokens(root Root) map[string]string {
	attention, stale := 0, 0
	for _, r := range root.Repos {
		if r.NeedsAttention() {
			attention++
		}
		if r.Stale != nil {
			stale++
		}
	}
	toks := map[string]string{"repos": fmt.Sprintf("✓%d", len(root.Repos))}
	if attention > 0 {
		toks["repos"] = fmt.Sprintf("%d/%d", attention, len(root.Repos))
	}
	if stale > 0 {
		toks["stale"] = fmt.Sprintf("⚠%d", stale)
	}
	return toks
}

// repoTokens are the tokens of a workspace mapped to repo: its uncommitted
// count, ∅ when it has no upstream, and whether it is stale.
func repoTokens(repo Repo) map[string]string {
	var changes []string
	if repo.Uncommitted > 0 {
		changes = append(changes, fmt.Sprintf("●%d", repo.Uncommitted))
	}
	if repo.NoUpstream() {
		changes = append(changes, "∅")
	}
	toks := map[string]string{}
	if len(changes) > 0 {
		toks["changes"] = strings.Join(changes, " ")
	}
	if repo.Stale != nil {
		toks["stale"] = "⚠"
	}
	return toks
}
