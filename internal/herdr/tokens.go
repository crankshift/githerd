package herdr

import (
	"fmt"
	"strings"
)

// tokenNames are the workspace tokens githerd reports. Every report names
// each one: it sets those that apply and clears the others.
var tokenNames = []string{"repos", "changes", "stale"}

// rootTokens are the tokens of a root workspace: how many of the root's
// repos need attention, of how many.
func rootTokens(root Root) map[string]string {
	attention := 0
	for _, r := range root.Repos {
		if r.NeedsAttention() {
			attention++
		}
	}
	toks := map[string]string{"repos": fmt.Sprintf("✓%d", len(root.Repos))}
	if attention > 0 {
		toks["repos"] = fmt.Sprintf("%d/%d", attention, len(root.Repos))
	}
	return toks
}

// repoTokens are the tokens of a workspace mapped to repo: its uncommitted
// count, and ∅ when it has no upstream.
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
	return toks
}
