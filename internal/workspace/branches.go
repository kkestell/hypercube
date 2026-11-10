package workspace

import (
	"fmt"
	"strings"
)

// CheckNewBranch returns an error unless name is a valid branch name that
// does not exist in root's repository.
func CheckNewBranch(root, name string) error {
	// check-ref-format expands @{-1} to the previous branch's name.
	if got, err := git(root, "check-ref-format", "--branch", name); err != nil || got != name {
		return fmt.Errorf("%q is not a valid branch name", name)
	}
	if exists(root, name) {
		return fmt.Errorf("a branch named %s already exists", name)
	}
	return nil
}

// Branches lists root's local branches and remote-tracking branches by short
// name, most recently committed first, without symbolic refs such as
// origin/HEAD.
func Branches(root string) (local, remote []string, err error) {
	out, err := git(root, "for-each-ref", "--sort=-committerdate", "--format=%(refname)%00%(symref)", "refs/heads", "refs/remotes")
	if err != nil {
		return nil, nil, err
	}
	local, remote = []string{}, []string{}
	for line := range strings.Lines(out) {
		ref, symref, _ := strings.Cut(strings.TrimSpace(line), "\x00")
		if symref != "" {
			continue
		}
		if name, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
			local = append(local, name)
		} else if name, ok := strings.CutPrefix(ref, "refs/remotes/"); ok {
			remote = append(remote, name)
		}
	}
	return local, remote, nil
}

// FreeBranch returns name when root's repository has no local branch of that
// name, else the first of name-2, name-3, … that it lacks.
func FreeBranch(root, name string) string {
	free := name
	for n := 2; exists(root, free); n++ {
		free = fmt.Sprintf("%s-%d", name, n)
	}
	return free
}
