package utils

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/apimachinery/pkg/api/meta"
)

func GetClientConfigOutsideCluster(kubeconfig string) (*rest.Config, error) {
	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

func GetClientConfig(kubeconfig string) (*rest.Config, error) {
	var config *rest.Config
	var err error
	config, err = rest.InClusterConfig()
	if err != nil {
		config, err = GetClientConfigOutsideCluster(kubeconfig)
	}
	return config, err
}

func GetClientConfigOrDie(kubeconfig string) *rest.Config {
	// if the token file exists, assume we're running in Cluster
	cfg, err := GetClientConfig(kubeconfig)
	if err != nil {
		panic(err.Error())
	}
	return cfg
}

func GetClientsetFromConfig(config *rest.Config) (*kubernetes.Clientset, error) {
	return kubernetes.NewForConfig(config)
}

func GetClientsetFromConfigOrDie(config *rest.Config) *kubernetes.Clientset {
	return kubernetes.NewForConfigOrDie(config)
}

func GetClientset(kubeconfig string, overrides *clientcmd.ConfigOverrides) (kubernetes.Interface, error) {
	var config *rest.Config
	var err error
	if kubeconfig == "" {
		config, err = rest.InClusterConfig()
		if err == rest.ErrNotInCluster {
			err = nil
			kubeconfig = clientcmd.RecommendedHomeFile
		}
	}

	if err == nil && config == nil {
		config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig},
			overrides,
		).ClientConfig()
	}

	if err != nil {
		return nil, err
	}
	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}

func GetClientsetOrDie(kubeconfig string, overrides *clientcmd.ConfigOverrides) kubernetes.Interface {
	if overrides == nil {
		overrides = &clientcmd.ConfigOverrides{}
	}
	cs, err := GetClientset(kubeconfig, overrides)
	if err != nil {
		panic(err.Error())
	}
	return cs
}

func GetObjectUIDIndexFunc() cache.IndexFunc {
	return cache.IndexFunc(func(obj interface{}) ([]string, error) {
		meta, err := meta.Accessor(obj)
		if err != nil {
			return []string{""}, fmt.Errorf("object has no meta: %v", err)
		}
		return []string{string(meta.GetUID())}, nil
	})
}
