package server

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	. "github.com/trilogy-group/k8s-webhooks/pkg/webhooks"
)

func (whsrv *webhookServer) RegisterHandler(path string, h AdmissionHandler) error {
	if whsrv.handlers == nil {
		whsrv.handlers = make(HandlersMap)
	}
	if _, alreadyExists := whsrv.handlers[path]; alreadyExists {
		return errors.New(fmt.Sprintf("Handler for path: %s already exists", path))
	}
	whsrv.handlers[path] = h
	return nil
}

func (whsrv *webhookServer) GetHandlerForPath(path string) AdmissionHandler {
	// try exact path match (faster)
	if h, ok := whsrv.handlers[path]; ok {
		return h
	}
	// try with prefix match, longer is better
	paths := make([]string, 0, len(whsrv.handlers))
	for hPath, _ := range whsrv.handlers {
		paths = append(paths, hPath)
	}
	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) > len(paths[j]) })
	for _, hPath := range paths {
		if strings.HasPrefix(path, hPath) {
			return whsrv.handlers[hPath]
		}
	}

	if whsrv.config.DefaultAdmitPolicy == "Never" {
		return AdmitNever
	}
	return AdmitAlways
}

func WithHandlers(handlersMap HandlersMap) WebhookServerOption {
	return func(whsrv WebhookServer) WebhookServer {
		for path, handler := range handlersMap {
			whsrv.RegisterHandler(path, handler)
		}
		return whsrv
	}
}
