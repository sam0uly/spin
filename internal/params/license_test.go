package params

import (
	"slices"
	"strings"
	"testing"

	"github.com/sam0uly/spin/internal/licenses"
)

func TestParse_LicenseParam(t *testing.T) {
	p, err := ParseOne("license", Spec{Type: TypeLicense, Prompt: "License"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Type() != TypeLicense {
		t.Errorf("Type() = %q, want %q", p.Type(), TypeLicense)
	}
	lp, ok := p.(*LicenseParam)
	if !ok {
		t.Fatalf("ParseOne returned %T, want *LicenseParam", p)
	}
	// Options auto-filled from the built-in set plus "None".
	if len(lp.options) != len(licenses.Options()) {
		t.Errorf("options not auto-filled: got %d, want %d", len(lp.options), len(licenses.Options()))
	}
	if !slices.Contains(lp.options, "MIT") || !slices.Contains(lp.options, "None") {
		t.Errorf("options should include MIT and None; got %v", lp.options)
	}
}

func TestLicenseParam_BehavesLikeSelect(t *testing.T) {
	p := NewLicense("license", "License", nil, "MIT")
	p.SetDefault(map[string]any{})
	if got := p.Value(); got.String != "MIT" {
		t.Errorf("default not applied: got %q, want %q", got.String, "MIT")
	}
	p.Apply(Value{Kind: TypeSelect, String: "Apache-2.0"})
	if got := p.Value().String; got != "Apache-2.0" {
		t.Errorf("Apply not honored: got %q, want %q", got, "Apache-2.0")
	}
	if !strings.Contains(p.String(), "license") {
		t.Errorf("String() should name the param type; got %q", p.String())
	}
}

func TestLicenseParam_CustomOptionsKept(t *testing.T) {
	p := NewLicense("license", "License", []string{"MIT", "None"}, "")
	if len(p.options) != 2 {
		t.Errorf("explicit options must be kept, got %v", p.options)
	}
}

func TestValidateDefaults_LicenseParam(t *testing.T) {
	good := NewLicense("license", "License", nil, "MIT")
	good.SetDefault(map[string]any{})
	if err := ValidateDefaults([]Param{good}); err != nil {
		t.Errorf("valid license value rejected: %v", err)
	}
	bad := NewLicense("license", "License", nil, "")
	bad.Apply(Value{Kind: TypeSelect, String: "MadeUp"})
	if err := ValidateDefaults([]Param{bad}); err == nil {
		t.Error("out-of-options license value must be rejected")
	}
}
