package server

import (
	"errors"
	"fmt"

	"k8s.io/client-go/informers"

	. "github.com/trilogy-group/k8s-webhooks/pkg/webhooks"
)

func (whsrv *webhookServer) StartFactory(factoryName string) error {
	f, ok := whsrv.factories[factoryName]
	if !ok {
		return errors.New(fmt.Sprintf("Unknown factory for name: %s", factoryName))
	}
	f.Start(whsrv.stopCh)
	for _, ok = range f.WaitForCacheSync(whsrv.stopCh) {
		if !ok {
			return errors.New(fmt.Sprintf("failed to wait for caches to sync (factory name: %s)", factoryName))
		}
	}
	return nil
}

func (whsrv *webhookServer) RegisterFactory(name string, factory informers.SharedInformerFactory) {
	if whsrv.factories == nil {
		whsrv.factories = make(FactoriesMap)
	}
	if _, alreadyExists := whsrv.factories[name]; !alreadyExists {
		whsrv.factories[name] = factory
	}
}

func (whsrv *webhookServer) GetFactory(name string) informers.SharedInformerFactory {
	if f, ok := whsrv.factories[name]; ok {
		return f
	}
	return nil
}

func WithFactories(factoriesMap FactoriesMap) WebhookServerOption {
	return func(whsrv WebhookServer) WebhookServer {
		for path, handler := range factoriesMap {
			whsrv.RegisterFactory(path, handler)
		}
		return whsrv
	}
}
