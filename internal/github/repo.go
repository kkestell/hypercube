package github

import (
	"net/url"
	"strings"
)

// parseRepo reads the owner and name from a github.com remote URL.
func parseRepo(remote string) (owner, name string, ok bool) {
	var host, path string
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil {
			return "", "", false
		}
		switch u.Scheme {
		case "https", "http", "ssh", "git":
		default:
			return "", "", false
		}
		host, path = u.Hostname(), u.Path
	} else {
		// scp form: user@host:path
		var rest string
		if _, rest, ok = strings.Cut(remote, "@"); !ok {
			return "", "", false
		}
		host, path, ok = strings.Cut(rest, ":")
		if !ok {
			return "", "", false
		}
	}
	if !strings.EqualFold(host, "github.com") {
		return "", "", false
	}
	path = strings.TrimSuffix(strings.TrimSuffix(path, "/"), ".git")
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
