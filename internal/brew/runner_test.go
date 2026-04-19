package brew

import (
	"testing"

	"brew-sync/internal/diff"
)

// Compile-time check: MockBrewRunner must satisfy the BrewRunner interface.
var _ BrewRunner = (*MockBrewRunner)(nil)

// --- parseBrewListOutput tests ---

func TestParseBrewListOutput_Empty(t *testing.T) {
	got := parseBrewListOutput("")
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %v", got)
	}
}

func TestParseBrewListOutput_SinglePackageWithVersion(t *testing.T) {
	got := parseBrewListOutput("git 2.40.0\n")
	want := []diff.Package{{Name: "git", Version: "2.40.0"}}
	if len(got) != 1 {
		t.Fatalf("expected 1 package, got %d", len(got))
	}
	if got[0] != want[0] {
		t.Errorf("got %+v, want %+v", got[0], want[0])
	}
}

func TestParseBrewListOutput_MultiplePackages(t *testing.T) {
	got := parseBrewListOutput("git 2.40.0\ngo 1.23\n")
	if len(got) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(got))
	}
	if got[0].Name != "git" || got[0].Version != "2.40.0" {
		t.Errorf("package 0: got %+v, want {Name:git Version:2.40.0}", got[0])
	}
	if got[1].Name != "go" || got[1].Version != "1.23" {
		t.Errorf("package 1: got %+v, want {Name:go Version:1.23}", got[1])
	}
}

func TestParseBrewListOutput_MultipleVersionsTakesFirst(t *testing.T) {
	got := parseBrewListOutput("python 3.12.0 3.11.0\n")
	if len(got) != 1 {
		t.Fatalf("expected 1 package, got %d", len(got))
	}
	if got[0].Name != "python" || got[0].Version != "3.12.0" {
		t.Errorf("got %+v, want {Name:python Version:3.12.0}", got[0])
	}
}

func TestParseBrewListOutput_PackageNoVersion(t *testing.T) {
	got := parseBrewListOutput("curl\n")
	if len(got) != 1 {
		t.Fatalf("expected 1 package, got %d", len(got))
	}
	if got[0].Name != "curl" || got[0].Version != "" {
		t.Errorf("got %+v, want {Name:curl Version:}", got[0])
	}
}

func TestParseBrewListOutput_BlankLinesAndWhitespace(t *testing.T) {
	input := "\n  git 2.40.0  \n\n  go 1.23\n  \n"
	got := parseBrewListOutput(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 packages, got %d: %v", len(got), got)
	}
	if got[0].Name != "git" || got[0].Version != "2.40.0" {
		t.Errorf("package 0: got %+v", got[0])
	}
	if got[1].Name != "go" || got[1].Version != "1.23" {
		t.Errorf("package 1: got %+v", got[1])
	}
}

func TestParseBrewListOutput_TrailingNewline(t *testing.T) {
	got := parseBrewListOutput("wget 1.21\n")
	if len(got) != 1 {
		t.Fatalf("expected 1 package, got %d", len(got))
	}
	if got[0].Name != "wget" || got[0].Version != "1.21" {
		t.Errorf("got %+v, want {Name:wget Version:1.21}", got[0])
	}
}

// --- parseLines tests ---

func TestParseLines_Empty(t *testing.T) {
	got := parseLines("")
	if len(got) != 0 {
		t.Errorf("expected empty/nil slice, got %v", got)
	}
}

func TestParseLines_MultipleLinesBlankFiltered(t *testing.T) {
	input := "homebrew/core\n\nhomebrew/cask\n\n"
	got := parseLines(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(got), got)
	}
	if got[0] != "homebrew/core" {
		t.Errorf("line 0: got %q, want %q", got[0], "homebrew/core")
	}
	if got[1] != "homebrew/cask" {
		t.Errorf("line 1: got %q, want %q", got[1], "homebrew/cask")
	}
}

func TestParseLines_WhitespaceTrimmed(t *testing.T) {
	input := "  homebrew/core  \n\t homebrew/cask \t\n"
	got := parseLines(input)
	if len(got) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(got), got)
	}
	if got[0] != "homebrew/core" {
		t.Errorf("line 0: got %q, want %q", got[0], "homebrew/core")
	}
	if got[1] != "homebrew/cask" {
		t.Errorf("line 1: got %q, want %q", got[1], "homebrew/cask")
	}
}

// TestLeafNameMatching_TapPrefixStripping verifies that tap-prefixed names from
// `brew leaves` (e.g. "cockroachdb/tap/cockroach") match short names from
// `brew list --formula --versions` (e.g. "cockroach").
func TestLeafNameMatching_TapPrefixStripping(t *testing.T) {
	leavesOutput := "git\ncockroachdb/tap/cockroach\nmongodb/brew/mongodb-community@8.0\n"
	formulaeOutput := "git 2.53.0\ncockroach 26.1.2\nmongodb-community@8.0 8.0.20\nreadline 8.3.3\n"

	// Build leaf name map the same way ListLeaves does
	leafNames := make(map[string]bool)
	for _, name := range parseLines(leavesOutput) {
		leafNames[name] = true
		if i := len(name) - 1; i >= 0 {
			for j := i; j >= 0; j-- {
				if name[j] == '/' {
					leafNames[name[j+1:]] = true
					break
				}
			}
		}
	}

	allFormulae := parseBrewListOutput(formulaeOutput)

	var leaves []diff.Package
	for _, pkg := range allFormulae {
		if leafNames[pkg.Name] {
			leaves = append(leaves, pkg)
		}
	}

	// Should include git, cockroach, and mongodb-community@8.0 but NOT readline
	if len(leaves) != 3 {
		t.Fatalf("expected 3 leaves, got %d: %v", len(leaves), leaves)
	}

	want := map[string]string{
		"git":                    "2.53.0",
		"cockroach":              "26.1.2",
		"mongodb-community@8.0": "8.0.20",
	}
	for _, pkg := range leaves {
		if wantVer, ok := want[pkg.Name]; !ok {
			t.Errorf("unexpected leaf: %s", pkg.Name)
		} else if pkg.Version != wantVer {
			t.Errorf("leaf %s: got version %s, want %s", pkg.Name, pkg.Version, wantVer)
		}
	}
}