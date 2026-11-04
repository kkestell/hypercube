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

// Changes lists all staged, unstaged, and untracked files relative to root.
func Changes(root string) (ChangesResult, error) {
	result := ChangesResult{Files: []Change{}}
	status, err := run(root, nil, "", 0, "status", "--porcelain=v2", "--branch", "--untracked-files=all", "-z")
	if err != nil {
		return result, err
	}
	base := "HEAD"
	for _, record := range strings.Split(status, "\x00") {
		if branch, ok := strings.CutPrefix(record, "# branch.head "); ok && branch != "(detached)" {
			result.Branch = branch
		}
		if record == "# branch.oid (initial)" {
			base, err = emptyTree(root)
			if err != nil {
				return result, err
			}
		}
		if path, ok := strings.CutPrefix(record, "? "); ok {
			result.Files = append(result.Files, Change{Path: path, Status: "untracked"})
		}
	}
	out, err := run(root, nil, "", 0, "diff", base, "--raw", "--numstat", "--no-renames", "-z")
	if err != nil {
		return result, err
	}
	records := strings.Split(out, "\x00")
	var tracked []Change
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
		tracked = append(tracked, Change{Path: records[i+1], Status: status})
		i += 2
	}
	for j := range tracked {
		fields := strings.SplitN(records[i+j], "\t", 3)
		if fields[0] != "-" && fields[1] != "-" {
			added, _ := strconv.Atoi(fields[0])
			removed, _ := strconv.Atoi(fields[1])
			tracked[j].Added, tracked[j].Removed = &added, &removed
		}
	}
	result.Files = append(result.Files, tracked...)
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

// Diff returns Git's whole-file patch for one literal repository-relative path.
func Diff(root, path string, limit int) (string, error) {
	base, err := diffBase(root)
	if err != nil {
		return "", err
	}
	out, err := run(root, nil, "", limit, "--literal-pathspecs", "diff", base, "--no-color", "--no-ext-diff", "--no-renames", "-U1000000", "--", path)
	if err != nil || out != "" {
		return out, err
	}
	untracked, err := git(root, "--literal-pathspecs", "ls-files", "--others", "--exclude-standard", "--", path)
	if err != nil || untracked == "" {
		return "", err
	}
	out, err = run(root, nil, "", limit, "diff", "--no-index", "--no-color", "--no-ext-diff", "-U1000000", "--", "/dev/null", path)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		err = nil
	}
	return out, err
}
