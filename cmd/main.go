package main

import (
	"context"
	// "flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/klog"

	"github.com/trilogy-group/k8s-webhooks/pkg/webhooks"
	"github.com/trilogy-group/k8s-webhooks/pkg/webhooks/server"

	"github.com/trilogy-group/k8s-webhooks/pkg/plugins/affinity"
	"github.com/trilogy-group/k8s-webhooks/pkg/plugins/ingress"
	"github.com/trilogy-group/k8s-webhooks/pkg/plugins/jivewebappaffinity"
)

var version string

func main() {
	flags := parseFlags()

	if flags.version {
		fmt.Printf("webhooks-manager version %s\n", version)
		os.Exit(0)
	}

	ws := server.NewWebhookServer(
		&webhooks.WebhookServerConfig{
			DefaultAdmitPolicy: flags.wsFlags.DefaultAdmitPolicy,
			UseConfigMap:       flags.wsFlags.UseConfigMap,
			Kubeconfig:         flags.wsFlags.Kubeconfig,
			CmNamespace:        flags.wsFlags.CmNamespace,
			CmName:             flags.wsFlags.CmName,
		},
		&webhooks.WhSrvParameters{
			Port:     flags.wsFlags.Port,
			CertFile: flags.wsFlags.CertFile,
			KeyFile:  flags.wsFlags.KeyFile,
		})

	if flags.deploymentAffinity {
		wh := affinity.NewWebhookHandler()
		wh.Setup(ws, "/deployment/affinity")
	}

	if flags.ingressRewriteTarget {
		wh := ingress.NewWebhookHandler()
		wh.Setup(ws, "/ingress/rewrite")
	}

	if flags.jiveWebAppsAffinity {
		wh := jivewebappaffinity.NewWebhookHandler()
		wh.Setup(ws, "/jive/webapp")
	}

	go func() {
		if err := ws.Start(); err != http.ErrServerClosed {
			klog.Fatalf("Server Start Failed:%+v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		// extra handling here
		cancel()
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGQUIT, syscall.SIGINT, syscall.SIGTERM)
	<-c

	if err := ws.Shutdown(ctx); err != nil {
		klog.Fatalf("Server Shutdown Failed:%+v", err)
	}
	klog.Info("Server Exited Properly")
}
