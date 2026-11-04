package workspace

import (
	"errors"
	"os/exec"
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
	Files  []Change `json:"files"`
}

// Changes lists the staged, unstaged, and untracked files within dir, by path
// relative to dir, and the branch checked out there.
func Changes(dir string) (ChangesResult, error) {
	result := ChangesResult{Files: []Change{}}
	branch, err := Branch(dir)
	if err != nil {
		return result, err
	}
	result.Branch = branch
	untracked, err := run(dir, []string{"GIT_OPTIONAL_LOCKS=0"}, "", 0, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return result, err
	}
	for _, path := range strings.Split(untracked, "\x00") {
		if path != "" {
			result.Files = append(result.Files, Change{Path: path, Status: "untracked"})
		}
	}
	base, err := diffBase(dir)
	if err != nil {
		return result, err
	}
	// Diff can explicitly refresh the index even with optional locks disabled.
	out, err := run(dir, []string{"GIT_OPTIONAL_LOCKS=0"}, "", 0, "-c", "diff.autoRefreshIndex=false", "diff", base, "--raw", "--numstat", "--no-renames", "--relative", "-z")
	if err != nil {
		return result, err
	}
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
		result.Files = append(result.Files, file)
	}
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
	out, err := run(dir, []string{"GIT_OPTIONAL_LOCKS=0"}, "", limit, "--literal-pathspecs", "-c", "diff.autoRefreshIndex=false", "diff", base, "--no-color", "--no-ext-diff", "--no-renames", "-U1000000", "--", path)
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
