// Package licenses provides the supported built-in license set for
// `spin new`. The canonical texts are vendored verbatim from the SPDX
// license-list-data repository (https://spdx.org/licenses/), embedded
// into the binary, and accessed through Known/IsKnown/Render.
//
// This is intentionally a small, curated set -- not a complete SPDX
// catalog and not a license-management system. Licenses outside the
// set are template-owned: a template can use a plain select param or
// ship its own _base/LICENSE file.
package licenses

import (
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed texts/*.txt
var texts embed.FS

// NoneID is the "no license" option offered alongside the built-in
// set. It is not a license: selecting it produces no LICENSE file.
const NoneID = "None"

// ids is the curated set of supported built-in licenses, in the order
// they are offered to the user.
var ids = []string{
	"MIT",
	"Apache-2.0",
	"BSD-3-Clause",
	"BSD-2-Clause",
	"ISC",
	"0BSD",
	"Unlicense",
	"CC0-1.0",
	"MPL-2.0",
	"GPL-2.0-only",
	"GPL-3.0-only",
	"AGPL-3.0-only",
	"LGPL-3.0-only",
}

// Known returns the supported built-in license IDs, sorted.
func Known() []string {
	out := append([]string(nil), ids...)
	sort.Strings(out)
	return out
}

// Options returns the selectable options: the known IDs plus "None"
// last. Used to build the `type = "license"` param's option list.
func Options() []string {
	out := Known()
	return append(out, NoneID)
}

// IsKnown reports whether id is a supported built-in license. "None",
// "Proprietary", and the empty string are never known: they mean "no
// license file". Matching is exact against the SPDX IDs.
func IsKnown(id string) bool {
	if id == "" || strings.EqualFold(id, NoneID) || strings.EqualFold(id, "Proprietary") {
		return false
	}
	for _, k := range ids {
		if k == id {
			return true
		}
	}
	return false
}

// Render returns the canonical text of a built-in license, substituting
// the copyright year and holder into the placeholder tokens the SPDX
// texts carry (`<year>`, `[yyyy]`, `<copyright holders>`, `<owner>`,
// `[name of copyright owner]`, `<name of author>`). A zero year or
// empty holder leaves the corresponding token untouched, so a license
// never claims an ownership it was not given. Render errors on an
// unknown id.
func Render(id, holder string, year int) (string, error) {
	if !IsKnown(id) {
		return "", fmt.Errorf("unknown license %q (supported: %s)", id, strings.Join(Known(), ", "))
	}
	text, err := texts.ReadFile("texts/" + id + ".txt")
	if err != nil {
		return "", err
	}
	s := string(text)
	if year > 0 {
		yy := fmt.Sprintf("%d", year)
		s = strings.ReplaceAll(s, "<year>", yy)
		s = strings.ReplaceAll(s, "[yyyy]", yy)
	}
	if holder != "" {
		s = strings.ReplaceAll(s, "<copyright holders>", holder)
		s = strings.ReplaceAll(s, "<owner>", holder)
		s = strings.ReplaceAll(s, "[name of copyright owner]", holder)
		s = strings.ReplaceAll(s, "<name of author>", holder)
	}
	return s, nil
}
