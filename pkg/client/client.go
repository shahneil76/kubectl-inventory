package client

import (
	"fmt"
	"os"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// Options controls how the Kubernetes client is configured.
// All fields are optional — sensible defaults are applied automatically.
type Options struct {
	// Kubeconfig is an explicit path to a kubeconfig file.
	// If empty, the standard discovery chain is used:
	//   1. KUBECONFIG env var (supports colon-separated list of files)
	//   2. ~/.kube/config
	//   3. In-cluster service account (when running inside a pod)
	Kubeconfig string

	// Context is the kubeconfig context name to use.
	// If empty, the current-context of the resolved kubeconfig is used.
	Context string
}

type Clients struct {
	Config    *rest.Config
	Dynamic   dynamic.Interface
	Metadata  metadata.Interface
	Discovery discovery.DiscoveryInterface
	Typed     kubernetes.Interface
	RawConfig clientcmdapi.Config
	Context   string
	Namespace string
}

// New builds a fully initialised Clients from the given options.
//
// Kubeconfig resolution order:
//  1. --kubeconfig flag  (opts.Kubeconfig)
//  2. KUBECONFIG env var (may be colon-separated list; files are merged)
//  3. ~/.kube/config
//  4. In-cluster service account (only when none of the above exist)
func New(opts Options) (*Clients, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	// NewDefaultClientConfigLoadingRules already reads KUBECONFIG env var and
	// falls back to ~/.kube/config. We only override ExplicitPath when the
	// caller passed an explicit --kubeconfig flag.
	if opts.Kubeconfig != "" {
		if _, err := os.Stat(opts.Kubeconfig); err != nil {
			return nil, fmt.Errorf("kubeconfig file not found: %s", opts.Kubeconfig)
		}
		loadingRules.ExplicitPath = opts.Kubeconfig
	}

	overrides := &clientcmd.ConfigOverrides{}
	if opts.Context != "" {
		overrides.CurrentContext = opts.Context
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	config, err := clientConfig.ClientConfig()
	if err != nil {
		// Provide a more actionable error message.
		return nil, fmt.Errorf(
			"could not build kubeconfig: %w\n\nHints:\n"+
				"  • Set KUBECONFIG env var:     export KUBECONFIG=~/.kube/config\n"+
				"  • Pass explicit file:         --kubeconfig /path/to/config\n"+
				"  • Switch context:             --context my-context\n"+
				"  • List available contexts:    kubectl config get-contexts",
			err,
		)
	}

	// Raise QPS/burst limits — we make many parallel List calls.
	config.QPS = 50
	config.Burst = 100
	config.WarningHandler = rest.NoWarnings{}

	rawConfig, err := clientConfig.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("load raw kubeconfig: %w", err)
	}

	// Validate requested context exists.
	if opts.Context != "" {
		if _, ok := rawConfig.Contexts[opts.Context]; !ok {
			available := listContextNames(rawConfig)
			return nil, fmt.Errorf(
				"context %q not found in kubeconfig\n\nAvailable contexts:\n%s",
				opts.Context, available,
			)
		}
	}

	// Resolve namespace: flag > context default > "default"
	namespace, _, err := clientConfig.Namespace()
	if err != nil || namespace == "" {
		namespace = "default"
	}

	dyn, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}
	disc, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create discovery client: %w", err)
	}
	typed, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create typed client: %w", err)
	}
	meta, err := metadata.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create metadata client: %w", err)
	}

	contextName := rawConfig.CurrentContext
	if opts.Context != "" {
		contextName = opts.Context
	}

	return &Clients{
		Config:    config,
		Dynamic:   dyn,
		Metadata:  meta,
		Discovery: disc,
		Typed:     typed,
		RawConfig: rawConfig,
		Context:   contextName,
		Namespace: namespace,
	}, nil
}

// ListContexts returns all context names available in the resolved kubeconfig.
// It applies the same resolution chain as New (KUBECONFIG env, ~/.kube/config, explicit path).
func ListContexts(kubeconfigPath string) (current string, contexts []string, err error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		loadingRules.ExplicitPath = kubeconfigPath
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	raw, err := clientConfig.RawConfig()
	if err != nil {
		return "", nil, fmt.Errorf("load kubeconfig: %w", err)
	}

	return raw.CurrentContext, listContextNames(raw), nil
}

// listContextNames returns a sorted, bullet-formatted list of context names.
func listContextNames(raw clientcmdapi.Config) []string {
	names := make([]string, 0, len(raw.Contexts))
	for name := range raw.Contexts {
		marker := "  "
		if name == raw.CurrentContext {
			marker = "* "
		}
		names = append(names, marker+name)
	}
	return names
}
