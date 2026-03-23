package buildinfo

type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"builtAt"`
}

var (
	version = "dev"
	commit  = "local"
	builtAt = "unknown"
)

func Get() Info {
	return Info{
		Version: version,
		Commit:  commit,
		BuiltAt: builtAt,
	}
}
