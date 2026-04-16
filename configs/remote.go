package configs

import "context"

// RemoteProvider fetches configuration from a remote source.
// Implement this interface to integrate any remote config system
// (e.g., etcd, Consul, AWS AppConfig, Kubernetes ConfigMaps).
//
// Returned maps should use nested structure matching the YAML layout:
//
//	map[string]any{
//	    "database": map[string]any{
//	        "host": "db.prod.internal",
//	        "port": 5432,
//	    },
//	}
type RemoteProvider interface {
	// Fetch retrieves the current configuration as a map.
	Fetch(ctx context.Context) (map[string]any, error)

	// Watch registers a callback that is called whenever the remote config changes.
	// When fired with a nil error, the loader calls Fetch to retrieve fresh data.
	// Watch must not block; start a goroutine for any long-lived polling.
	Watch(ctx context.Context, onChange func(error))
}

// WithRemote adds a remote config source at the current precedence position.
// Sources are merged in registration order; later sources override earlier ones
// for the same key within the file/remote layer. Env vars and flags always win.
//
//	configs.Load(cfg,
//	    WithConfigFile("base.yaml"),   // lowest among sources
//	    WithRemote(etcdProvider),       // middle
//	    WithConfigFile("local.yaml"),   // highest among sources
//	    WithEnvPrefix("APP"),
//	)
//	// Precedence: flags > env > local.yaml > etcd > base.yaml > defaults
func WithRemote(provider RemoteProvider) Option {
	return func(o *options) {
		o.sources = append(o.sources, configSource{
			kind:     sourceKindRemote,
			provider: provider,
		})
	}
}
