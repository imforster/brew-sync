package manifest

// Manifest represents the declarative sync manifest (brew-sync.toml).
type Manifest struct {
	Version  int              `toml:"version"`
	Metadata ManifestMetadata `toml:"metadata"`
	Formulae []PackageEntry   `toml:"formulae"`
	Casks    []PackageEntry   `toml:"casks"`
	Taps     []string         `toml:"taps"`
}

// ManifestMetadata contains information about when and where the manifest was last updated.
type ManifestMetadata struct {
	UpdatedAt string `toml:"updated_at"`
	UpdatedBy string `toml:"updated_by"`
	Machine   string `toml:"machine"`
}

// PackageEntry represents a single package in the manifest with optional machine filters.
type PackageEntry struct {
	Name     string   `toml:"name"`
	Version  string   `toml:"version,omitempty"`
	OnlyOn   []string `toml:"only_on,omitempty"`
	ExceptOn []string `toml:"except_on,omitempty"`
}
