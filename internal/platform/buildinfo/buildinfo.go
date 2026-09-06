// Package buildinfo exposes non-sensitive build metadata injected by the build.
package buildinfo

var (
	Version = "dev"
	Commit  = "unknown"
	BuiltAt = "unknown"
	Dirty   = "unknown"
)

// Info is safe to expose through the operational version endpoint.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"builtAt"`
	Dirty   string `json:"dirty"`
}

// Current returns the metadata compiled into the process.
func Current() Info { return Info{Version: Version, Commit: Commit, BuiltAt: BuiltAt, Dirty: Dirty} }
