package params

import (
	"fmt"

	"github.com/sam0uly/spin/internal/licenses"
)

// LicenseParam is a select-style param over the built-in license list.
// Declaring `type = "license"` in spin.toml is the explicit opt-in
// contract for built-in licensing: the user picks a license at scaffold
// time and spin writes the generated project's LICENSE file from the
// resolved value. Options default to the curated SPDX set plus "None";
// templates can override with their own options.
type LicenseParam struct {
	*SelectParam
}

// NewLicense builds a LicenseParam. Empty options are replaced with the
// built-in set (licenses.Options()).
func NewLicense(name, prompt string, options []string, def string) *LicenseParam {
	if len(options) == 0 {
		options = licenses.Options()
	}
	return &LicenseParam{SelectParam: NewSelect(name, prompt, options, def)}
}

func (p *LicenseParam) Type() Type { return TypeLicense }

func (p *LicenseParam) String() string {
	return fmt.Sprintf("%s (license, default %q, options %v)", p.Name(), p.def, p.options)
}
