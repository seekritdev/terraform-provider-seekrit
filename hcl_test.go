package main

import (
	"os"
	"path/filepath"
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
