package webhooks

import (
	"context"
	admissionV1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/client-go/informers"
)

type FactoriesMap map[string]informers.SharedInformerFactory
type AdmissionHandler func(*admissionV1beta1.AdmissionReview) *admissionV1beta1.AdmissionResponse
type HandlersMap map[string]AdmissionHandler
type WebhookHandler interface {
	Setup(WebhookServer, string)
}

type WebhookServerOption func(WebhookServer) WebhookServer
type WebhookServer interface {
	Start() error
	Shutdown(context.Context) error
	RegisterHandler(path string, handler AdmissionHandler) error
	GetHandlerForPath(path string) AdmissionHandler
	StartFactory(factoryName string) error
	RegisterFactory(factoryName string, f informers.SharedInformerFactory)
	GetFactory(factoryName string) informers.SharedInformerFactory
	GetConfig() *WebhookServerConfig
}

// type WebhookConfigurator interface {
// 	SetupConfigurator()
// }

// type WebhookManager interface {
// 	LoadPlugins()
// }
