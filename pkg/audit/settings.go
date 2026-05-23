package audit

// Config holds user preferences for filtering audit results.
type Config struct {
	IgnoredNamespaces []string `json:"ignoredNamespaces"`
	DisabledChecks    []string `json:"disabledChecks"`
}

// DefaultConfig returns the default audit filter settings.
func DefaultConfig() Config {
	return Config{
		IgnoredNamespaces: []string{"kube-system", "kube-node-lease", "kube-public", "*-system"},
	}
}
