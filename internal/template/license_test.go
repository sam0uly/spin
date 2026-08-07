package template

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sam0uly/spin/internal/params"
)

func testTemplateWithBase(t *testing.T, files map[string]string) *Template {
	t.Helper()
	base := t.TempDir()
	for rel, body := range files {
		full := filepath.Join(base, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &Template{
		BaseDir: base,
		SpinToml: &SpinToml{
			Params: map[string]params.Spec{
				"license":          {Type: params.TypeLicense},
				"copyright_holder": {Type: params.TypeText},
			},
		},
	}
}

// TestRender_GeneratesLicense verifies the core built-in licensing
// path: a known license value plus a copyright holder produce a
// LICENSE file with the year and holder substituted.
func TestRender_GeneratesLicense(t *testing.T) {
	tpl := testTemplateWithBase(t, map[string]string{"main.txt": "hello"})
	out, err := tpl.Render(map[string]any{
		"license":          "MIT",
		"copyright_holder": "Jane Doe",
	})
	if err != nil {
		t.Fatal(err)
	}
	lic, ok := out["LICENSE"]
	if !ok {
		t.Fatalf("expected generated LICENSE; files: %v", keysOf(out))
	}
	text := string(lic)
	if !strings.Contains(text, "MIT License") {
		t.Errorf("LICENSE should be the MIT text; got:\n%s", text)
	}
	wantLine := "Copyright (c) " + strconv.Itoa(time.Now().Year()) + " Jane Doe"
	if !strings.Contains(text, wantLine) {
		t.Errorf("LICENSE should contain %q; got:\n%s", wantLine, text)
	}
	if strings.Contains(text, "<year>") || strings.Contains(text, "<copyright holders>") {
		t.Errorf("placeholder tokens must be substituted; got:\n%s", text)
	}
}

// TestRender_MissingHolderKeepsToken verifies generation never invents
// an owner: with no copyright_holder value the SPDX token stays.
func TestRender_MissingHolderKeepsToken(t *testing.T) {
	tpl := testTemplateWithBase(t, nil)
	out, err := tpl.Render(map[string]any{"license": "MIT"})
	if err != nil {
		t.Fatal(err)
	}
	text := string(out["LICENSE"])
	if !strings.Contains(text, "<copyright holders>") {
		t.Errorf("missing holder must leave the token in place; got:\n%s", text)
	}
}

func TestRender_NoLicenseMeansNoFile(t *testing.T) {
	cases := map[string]map[string]any{
		"empty value":     {"license": ""},
		"None":            {"license": "None"},
		"none lowercase":  {"license": "none"},
		"Proprietary":     {"license": "Proprietary"},
		"unknown license": {"license": "MadeUp"},
		"no value at all": {},
	}
	for name, values := range cases {
		t.Run(name, func(t *testing.T) {
			tpl := testTemplateWithBase(t, nil)
			out, err := tpl.Render(values)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if _, ok := out["LICENSE"]; ok {
				t.Errorf("LICENSE must not be generated for %v", values)
			}
		})
	}
}

// TestRender_TemplateLicenseWins verifies an existing LICENSE in the
// template's _base/ is never overwritten by generation.
func TestRender_TemplateLicenseWins(t *testing.T) {
	custom := "Custom license text\n"
	tpl := testTemplateWithBase(t, map[string]string{"LICENSE": custom})
	out, err := tpl.Render(map[string]any{
		"license":          "MIT",
		"copyright_holder": "Jane Doe",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out["LICENSE"]); got != custom {
		t.Errorf("template's own LICENSE must win; got:\n%s", got)
	}
}

// TestRender_LicenseVariantsAreRespected covers the other filenames
// that count as "the template already ships a license file".
func TestRender_LicenseVariantsAreRespected(t *testing.T) {
	for _, name := range []string{"LICENSE.txt", "LICENSE.md", "COPYING", "COPYING.txt", "license"} {
		t.Run(name, func(t *testing.T) {
			tpl := testTemplateWithBase(t, map[string]string{name: "keep me"})
			out, err := tpl.Render(map[string]any{"license": "MIT"})
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := out["LICENSE"]; ok {
				t.Errorf("generated LICENSE must not overwrite %s", name)
			}
			if got := string(out[name]); got != "keep me" {
				t.Errorf("%s content changed: %q", name, got)
			}
		})
	}
}

// TestRender_NoLicenseParamNeverGenerates verifies templates without
// the license param are completely unaffected by the feature.
func TestRender_NoLicenseParamNeverGenerates(t *testing.T) {
	tpl := &Template{BaseDir: t.TempDir(), SpinToml: &SpinToml{Params: map[string]params.Spec{}}}
	out, err := tpl.Render(map[string]any{"name": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out["LICENSE"]; ok {
		t.Error("template without license param must not get a LICENSE")
	}
}
