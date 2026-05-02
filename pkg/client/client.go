package client

import (
	"fmt"
	"path/filepath"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"
)

type Options struct {
	Kubeconfig string
	Context    string
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

func New(opts Options) (*Clients, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if opts.Kubeconfig != "" {
		loadingRules.ExplicitPath = opts.Kubeconfig
	} else if home := homedir.HomeDir(); home != "" {
		loadingRules.ExplicitPath = filepath.Join(home, ".kube", "config")
	}

	overrides := &clientcmd.ConfigOverrides{}
	if opts.Context != "" {
		overrides.CurrentContext = opts.Context
	}

	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	config, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build kubeconfig: %w", err)
	}
	config.QPS = 50
	config.Burst = 100
	config.WarningHandler = rest.NoWarnings{}

	rawConfig, err := clientConfig.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("load raw kubeconfig: %w", err)
	}

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
