package workspace

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

var ErrTooLarge = errors.New("file is too large")

type Change struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	Added   *int   `json:"added,omitempty"`
	Removed *int   `json:"removed,omitempty"`
}

type ChangesResult struct {
	Branch string   `json:"branch"`
	Target string   `json:"target,omitempty"`
	Files  []Change `json:"files"`
}

// Changes lists staged, unstaged, and untracked files within dir, relative
// to dir. top is the containing Git checkout root.
func Changes(dir, top string) (ChangesResult, error) {
	return changes(dir, top, "")
}

func changes(dir, top, base string) (ChangesResult, error) {
	result := ChangesResult{Files: []Change{}}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return result, err
	}
	root, err := filepath.EvalSymlinks(top)
	if err != nil {
		return result, err
	}
	prefix, err := filepath.Rel(root, resolved)
	if err != nil {
		return result, err
	}
	if prefix != "." {
		prefix += string(filepath.Separator)
	} else {
		prefix = ""
	}
	status, err := run(dir, []string{"GIT_OPTIONAL_LOCKS=0"}, "", 0,
		"-c", "status.relativePaths=false", "status", "--porcelain=v2", "--branch", "-z", "--untracked-files=all", "--no-renames", "--", ".")
	if err != nil {
		return result, err
	}
	var unborn, branchFound bool
	for _, record := range strings.Split(status, "\x00") {
		switch {
		case strings.HasPrefix(record, "# branch.oid "):
			unborn = strings.TrimPrefix(record, "# branch.oid ") == "(initial)"
		case strings.HasPrefix(record, "# branch.head "):
			branchFound = true
			result.Branch = strings.TrimPrefix(record, "# branch.head ")
			if result.Branch == "(detached)" {
				result.Branch = ""
			}
		case strings.HasPrefix(record, "? "):
			path := strings.TrimPrefix(record, "? ")
			if strings.HasPrefix(path, prefix) {
				result.Files = append(result.Files, Change{Path: strings.TrimPrefix(path, prefix), Status: "untracked"})
			}
		}
	}
	if !branchFound {
		return result, fmt.Errorf("git status did not report a branch")
	}
	if base == "" {
		base = "HEAD"
		if unborn {
			base, err = emptyTree(dir)
			if err != nil {
				return result, err
			}
		}
	}
	tracked, err := changedFiles(dir, base, nil, ".")
	result.Files = append(result.Files, tracked...)
	return result, err
}

func changedFiles(dir, base string, env []string, paths ...string) ([]Change, error) {
	// Diff can explicitly refresh the index even with optional locks disabled.
	args := []string{"--literal-pathspecs", "-c", "diff.autoRefreshIndex=false", "diff", base, "--raw", "--numstat", "--no-renames", "--relative", "-z", "--"}
	out, err := run(dir, append([]string{"GIT_OPTIONAL_LOCKS=0"}, env...), "", 0, append(args, paths...)...)
	if err != nil {
		return nil, err
	}
	files := []Change{}
	// Git emits raw records before numstat. Pair by path: without refreshing
	// the index, raw can include stat-only changes that numstat omits.
	records := strings.Split(out, "\x00")
	statuses := map[string]string{}
	i := 0
	for i < len(records) && strings.HasPrefix(records[i], ":") {
		fields := strings.Fields(records[i])
		status := "modified"
		switch fields[len(fields)-1] {
		case "A":
			status = "added"
		case "D":
			status = "deleted"
		}
		statuses[records[i+1]] = status
		i += 2
	}
	for _, record := range records[i:] {
		if record == "" {
			continue
		}
		fields := strings.SplitN(record, "\t", 3)
		file := Change{Path: fields[2], Status: statuses[fields[2]]}
		if fields[0] != "-" && fields[1] != "-" {
			added, _ := strconv.Atoi(fields[0])
			removed, _ := strconv.Atoi(fields[1])
			file.Added, file.Removed = &added, &removed
		}
		files = append(files, file)
	}
	return files, nil
}

func branchBase(dir, checkout string) (base, target string, err error) {
	head, err := Head(checkout)
	if err != nil {
		return "", "", err
	}
	target, err = git(checkout, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", "", err
	}
	if target == "HEAD" {
		target = "detached HEAD"
	}
	base, err = git(dir, "merge-base", "HEAD", head)
	return
}

// baseIndex compares recreated untracked files with their old tree entries,
// without refreshing the real index or writing objects.
func baseIndex(dir, base string) (env []string, cleanup func(), err error) {
	folder, err := os.MkdirTemp("", "hypercube-branch-index-")
	if err != nil {
		return nil, nil, err
	}
	cleanup = func() { os.RemoveAll(folder) }
	env = []string{"GIT_OPTIONAL_LOCKS=0", "GIT_INDEX_FILE=" + filepath.Join(folder, "index")}
	_, err = run(dir, env, "", 0, "read-tree", base)
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return
}

// BranchChanges lists net working-file changes since the live merge-base.
func BranchChanges(dir, top, checkout string) (ChangesResult, error) {
	base, target, err := branchBase(dir, checkout)
	if err != nil {
		return ChangesResult{}, err
	}
	result, err := changes(dir, top, base)
	if err != nil {
		return result, err
	}
	result.Target = target
	untracked, tracked := map[string]bool{}, map[string]bool{}
	for _, file := range result.Files {
		if file.Status == "untracked" {
			untracked[file.Path] = true
		} else {
			tracked[file.Path] = true
		}
	}
	var overlaps []string
	for path := range untracked {
		if tracked[path] {
			overlaps = append(overlaps, path)
		}
	}
	if len(overlaps) == 0 {
		return result, nil
	}
	env, cleanup, err := baseIndex(dir, base)
	if err != nil {
		return result, err
	}
	defer cleanup()
	replacements, err := changedFiles(dir, base, env, overlaps...)
	if err != nil {
		return result, err
	}
	files := result.Files[:0]
	for _, file := range result.Files {
		if !(untracked[file.Path] && tracked[file.Path]) {
			files = append(files, file)
		}
	}
	result.Files = append(files, replacements...)
	return result, nil
}

func emptyTree(root string) (string, error) {
	return git(root, "hash-object", "-t", "tree", "/dev/null")
}

func diffBase(root string) (string, error) {
	if _, err := Head(root); err == nil {
		return "HEAD", nil
	}
	return emptyTree(root)
}

// Diff returns Git's whole-file patch for one literal path relative to dir.
func Diff(dir, path string, limit int) (string, error) {
	base, err := diffBase(dir)
	if err != nil {
		return "", err
	}
	return diff(dir, base, path, limit)
}

// BranchDiff uses the same live base and recreated-file treatment as BranchChanges.
func BranchDiff(dir, checkout, path string, limit int) (string, error) {
	base, _, err := branchBase(dir, checkout)
	if err != nil {
		return "", err
	}
	untracked, err := gitWith(dir, []string{"GIT_OPTIONAL_LOCKS=0"}, "", "--literal-pathspecs", "ls-files", "--others", "--exclude-standard", "--", path)
	if err != nil {
		return "", err
	}
	if untracked != "" {
		files, err := changedFiles(dir, base, nil, path)
		if err != nil {
			return "", err
		}
		if len(files) > 0 {
			env, cleanup, err := baseIndex(dir, base)
			if err != nil {
				return "", err
			}
			defer cleanup()
			// The alternate index already represents this file's base entry;
			// an empty patch means identical recreation, not a new file.
			return trackedDiff(dir, base, path, limit, env)
		}
	}
	return diff(dir, base, path, limit)
}

func trackedDiff(dir, base, path string, limit int, env []string) (string, error) {
	return run(dir, append([]string{"GIT_OPTIONAL_LOCKS=0"}, env...), "", limit, "--literal-pathspecs", "-c", "diff.autoRefreshIndex=false", "diff", base, "--no-color", "--no-ext-diff", "--no-renames", "-U1000000", "--", path)
}

func diff(dir, base, path string, limit int) (string, error) {
	out, err := trackedDiff(dir, base, path, limit, nil)
	if err != nil || out != "" {
		return out, err
	}
	untracked, err := gitWith(dir, []string{"GIT_OPTIONAL_LOCKS=0"}, "", "--literal-pathspecs", "ls-files", "--others", "--exclude-standard", "--", path)
	if err != nil || untracked == "" {
		return "", err
	}
	out, err = run(dir, []string{"GIT_OPTIONAL_LOCKS=0"}, "", limit, "-c", "diff.autoRefreshIndex=false", "diff", "--no-index", "--no-color", "--no-ext-diff", "-U1000000", "--", "/dev/null", path)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		err = nil
	}
	return out, err
}
