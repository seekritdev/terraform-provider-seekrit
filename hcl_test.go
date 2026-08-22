package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
)

// The Terraform Registry namespace this provider publishes under. It appears in
// four places that must agree: the provider server's Address (main.go), every
// module's required_providers, every example's required_providers, and the
// documentation. A mismatch is not a compile error — it surfaces as
// "provider not found" at someone's first `terraform init`.
const (
	registryNamespace = "seekritdev"
	registrySource    = registryNamespace + "/seekrit"
)

// tfFiles walks the committed .tf files: the modules and the examples. These are
// shipped artifacts — the examples are what the Registry renders on the
// provider's documentation pages — so a syntax error in one is a user-visible
// bug, and nothing else in the Go test suite would catch it.
func tfFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, root := range []string{"modules", "examples"} {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".tf") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	if len(files) == 0 {
		t.Fatal("no .tf files found — this test is not checking anything")
	}
	return files
}

// TestTerraformFilesParse is a syntax gate over the shipped HCL. It is not a
// `terraform validate`: no provider schema is consulted and no references are
// resolved, so it catches typos and unbalanced braces rather than a wrong
// attribute name. That is still the class of error most likely to reach a user,
// because these files are never applied in CI.
func TestTerraformFilesParse(t *testing.T) {
	for _, path := range tfFiles(t) {
		parser := hclparse.NewParser()
		if _, diags := parser.ParseHCLFile(path); diags.HasErrors() {
			t.Errorf("%s: %s", path, diags.Error())
		}
	}
}

// TestRegistryNamespaceIsConsistent pins the namespace across every file that
// names it. Renaming the namespace has to be a deliberate, complete change.
func TestRegistryNamespaceIsConsistent(t *testing.T) {
	// The provider server's advertised address.
	mainGo, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	wantAddress := `"registry.terraform.io/` + registrySource + `"`
	if !strings.Contains(string(mainGo), wantAddress) {
		t.Errorf("main.go does not advertise %s — Terraform would not match it to the "+
			"`source` in users' required_providers blocks", wantAddress)
	}

	// Every required_providers block in the modules and examples.
	checked := 0
	for _, path := range tfFiles(t) {
		parser := hclparse.NewParser()
		file, diags := parser.ParseHCLFile(path)
		if diags.HasErrors() {
			continue // reported by TestTerraformFilesParse
		}
		for _, source := range providerSources(file) {
			checked++
			if source != registrySource {
				t.Errorf("%s: required_providers source is %q, want %q",
					path, source, registrySource)
			}
		}
	}
	if checked == 0 {
		t.Error("found no required_providers source attributes — this test is not " +
			"checking anything; did the modules stop declaring the provider?")
	}
}

// providerSources pulls every `source` string out of the terraform >
// required_providers blocks in one file.
func providerSources(file *hcl.File) []string {
	content, _, _ := file.Body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: "terraform"}},
	})
	var sources []string
	for _, tfBlock := range content.Blocks {
		inner, _, _ := tfBlock.Body.PartialContent(&hcl.BodySchema{
			Blocks: []hcl.BlockHeaderSchema{{Type: "required_providers"}},
		})
		for _, reqBlock := range inner.Blocks {
			attrs, _ := reqBlock.Body.JustAttributes()
			for _, attr := range attrs {
				value, diags := attr.Expr.Value(nil)
				if diags.HasErrors() || !value.Type().IsObjectType() {
					continue
				}
				if !value.Type().HasAttribute("source") {
					continue
				}
				sources = append(sources, value.GetAttr("source").AsString())
			}
		}
	}
	return sources
}

// moduleRefPattern matches the pinned tag in a module git source, the
// `ref` query parameter on a `git::` URL. Every occurrence in the shipped text
// is something a reader is meant to copy, so every one has to name the current
// release. (Written without a literal example on purpose — this test scans its
// own file too, and an illustrative tag in a comment would fail it.)
var moduleRefPattern = regexp.MustCompile(`\?ref=v(\d+\.\d+\.\d+)`)

// TestPinnedModuleRefsMatchTheRelease is the backstop behind release-please.
//
// The `?ref=` tags are rewritten on each release by the `extra-files` entries in
// release-please-config.json, which only reach the files listed there. Adding a
// sixth snippet somewhere and forgetting to list it is silent: the docs keep
// advertising an old tag, which still resolves, so nothing breaks loudly — it
// just quietly hands people stale modules. This finds that.
//
// Skipped when the monorepo is not around, which is how it behaves on the public
// mirror: the mirror gets only this directory, so there is no manifest to check
// against and the check has already run in the monorepo that produced the sync.
func TestPinnedModuleRefsMatchTheRelease(t *testing.T) {
	const manifestPath = "../../.release-please-manifest.json"
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Skipf("no monorepo manifest at %s (expected on the public mirror): %v", manifestPath, err)
	}
	var manifest map[string]string
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parse %s: %v", manifestPath, err)
	}
	want, ok := manifest["apps/terraform-provider-seekrit"]
	if !ok {
		t.Fatal("the provider has no entry in .release-please-manifest.json")
	}

	// This directory, plus the one documentation page that lives outside it.
	roots := []string{".", "../../apps/site/src/app/docs/guides/terraform"}
	checked := 0
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				// The changelog is a historical record; old tags belong in it.
				if name := d.Name(); name == ".git" || name == "dist" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(path, "CHANGELOG.md") {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, match := range moduleRefPattern.FindAllStringSubmatch(string(body), -1) {
				checked++
				if match[1] != want {
					t.Errorf("%s pins module ref v%s, but the released version is v%s — "+
						"add this file to the provider's `extra-files` in "+
						"release-please-config.json so the tag is rewritten on release",
						path, match[1], want)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	if checked == 0 {
		t.Error("found no `?ref=vX.Y.Z` module sources — this test is not checking anything")
	}
}
