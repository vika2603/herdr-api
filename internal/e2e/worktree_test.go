//go:build e2e

package e2e

import (
	"path/filepath"
	"testing"

	"github.com/vika2603/herdr-client"
)

// stageWorktree works on the git repository the harness created. Every call
// passes the checkout path explicitly: without it the server places new
// checkouts under the caller's home directory, outside the temporary root.
func stageWorktree(t *testing.T, h *harness, _ *state) {
	list, err := h.client.WorktreeList(h.ctx(t), herdr.WorktreeListParams{
		Cwd:             ptr(h.repo),
		TrustRepository: ptr(true),
	})
	if h.cover(t, herdr.MethodWorktreeList, list, err) {
		if list.Source.RepoRoot != h.repo {
			t.Errorf("worktree.list reports repository %s, asked about %s", list.Source.RepoRoot, h.repo)
		}
		if len(list.Worktrees) == 0 {
			t.Errorf("worktree.list reports no worktree for %s", h.repo)
		}
	}

	checkout := filepath.Join(h.root, "wt-created")
	created, err := h.client.WorktreeCreate(h.ctx(t), herdr.WorktreeCreateParams{
		Cwd:             ptr(h.repo),
		Branch:          ptr("e2e-created"),
		Path:            ptr(checkout),
		TrustRepository: ptr(true),
		Focus:           ptr(false),
	})
	if h.cover(t, herdr.MethodWorktreeCreate, created, err) {
		if created.Worktree.Path != checkout {
			t.Errorf("worktree.create checked out at %s, asked for %s", created.Worktree.Path, checkout)
		}
		if created.Workspace.Worktree == nil {
			t.Errorf("worktree.create returned a workspace without worktree information")
		}

		removed, err := h.client.WorktreeRemove(h.ctx(t), herdr.WorktreeRemoveParams{
			WorkspaceID:     created.Workspace.WorkspaceID,
			Force:           ptr(true),
			TrustRepository: ptr(true),
		})
		if h.cover(t, herdr.MethodWorktreeRemove, removed, err) && removed.Path != checkout {
			t.Errorf("worktree.remove removed %s, expected %s", removed.Path, checkout)
		}
	}

	// worktree.open adopts a checkout that already exists, so git makes one.
	adopted := filepath.Join(h.root, "wt-open")
	if err := h.addWorktree(adopted, "e2e-open"); err != nil {
		t.Fatalf("prepare the checkout for worktree.open: %v", err)
	}
	opened, err := h.client.WorktreeOpen(h.ctx(t), herdr.WorktreeOpenParams{
		Cwd:             ptr(h.repo),
		Path:            ptr(adopted),
		TrustRepository: ptr(true),
		Focus:           ptr(false),
	})
	if !h.cover(t, herdr.MethodWorktreeOpen, opened, err) {
		return
	}
	if opened.Worktree.Path != adopted {
		t.Errorf("worktree.open opened %s, asked for %s", opened.Worktree.Path, adopted)
	}
	if _, err := h.client.WorkspaceClose(h.ctx(t), herdr.WorkspaceCloseParams{
		WorkspaceID: opened.Workspace.WorkspaceID,
	}); err != nil {
		t.Errorf("close the workspace worktree.open created: %v", err)
	}
}
