// Package spec classifies template source specs. Both the template
// loader and the registry need to tell a local path, a git URL, and a
// "user/repo" (or "alias/id") shorthand apart, so the logic lives here.
package spec

import "strings"

// IsLocalPath reports whether s looks like a local filesystem path.
func IsLocalPath(s string) bool {
	return len(s) > 0 && (s[0] == '/' || s[0] == '.' || s[0] == '~')
}

// IsGitURL reports whether s looks like a git remote URL.
func IsGitURL(s string) bool {
	for _, prefix := range []string{"http://", "https://", "git@", "git://", "ssh://"} {
		if len(s) > len(prefix) && strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// IsShorthand reports whether s looks like a two-segment `a/b`
// shorthand such as "user/repo" or "alias/id".
func IsShorthand(s string) bool {
	if s == "" || IsLocalPath(s) || IsGitURL(s) {
		return false
	}
	first := strings.IndexByte(s, '/')
	if first <= 0 || first == len(s)-1 {
		return false
	}
	return strings.IndexByte(s[first+1:], '/') < 0
}
