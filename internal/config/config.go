package config

// Config holds the brew-sync configuration.
type Config struct {
	ManifestPath string     `toml:"manifest_path"`
	MachineTag   string     `toml:"machine_tag"`
	SyncBackend  string     `toml:"sync_backend"`
	Git          GitConfig  `toml:"git"`
	File         FileConfig `toml:"file"`
}

// GitConfig holds configuration for the Git sync backend.
type GitConfig struct {
	RepoURL string `toml:"repo_url"`
	Branch  string `toml:"branch"`
}

// FileConfig holds configuration for the file sync backend.
type FileConfig struct {
	RemotePath string `toml:"remote_path"`
}
