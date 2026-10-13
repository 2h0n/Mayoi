package main

import (
	"fmt"
	"mayoi/internal/git"
	"os"
)

// Opening an existing checkout never changes the branch in either worktree.
func (a *app) askOpenWorktree(branch, path string) {
	r := a.git.repo
	open := func(newWindow bool) {
		if a.git.repo != r {
			return
		}
		a.gitDo("Opening worktree…", func(_ *git.Repo) error {
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			if !info.IsDir() {
				return fmt.Errorf("worktree is not a folder: %s", path)
			}
			return nil
		}, func() {
			if newWindow {
				a.host.newWindow(path)
			} else {
				a.switchFolder(path)
			}
		})
	}
	a.confirm = &confirm{
		title:        "Open Existing Worktree?",
		message:      fmt.Sprintf("The branch %q is already checked out in:\n\n%s\n\nChoose where to open this worktree.", branch, path),
		ok:           "Open in Current Window",
		run:          func() { open(false) },
		alternate:    "Open in New Window",
		runAlternate: func() { open(true) },
	}
}
