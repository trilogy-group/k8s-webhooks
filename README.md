# k8s-webhooks
Kubernetes Admission Controller WebHooks mini framework

## Webhooks management ##

This package makes easy to implement webhook servers focusing only on the core loginc of an Admission handler.

A tipical use of this package is:


    import (
	    "github.com/trilogy-group/central-tools/pkg/webhooks"
	    "github.com/trilogy-group/central-tools/pkg/webhooks/server"

    )

    func main () {
      var ws webhooks.WebhookServer
      ws = server.NewWebhookServerWithOptions(
          nil, nil,
          server.WithHandlers(webhooks.HandlersMap{
              "/mutate/deployment": myMutationDeploymentFunction,
              "/mutate/pod": myMutationPodFn,
              "/validate": myValidationFunction,
          }))
    }


Where `myValidationFunction` (as the other 2) is an `AdmissionHandler` that takes an `AdmissionReview` and return an `AdmissionResponse`, implementing the logic of validation or mutation.

### Webhook handlers configuration ###

It's easy to add informers to keep some configuration updateable via ConfigMap or any other resource.
To achieve that we can share `SharedInformerFactory` and install many event handler, so many webhook handlers can reuse the same shared informers.
