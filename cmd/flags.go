package main

import (
	goflag "flag"
	flag "github.com/spf13/pflag"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"

	"github.com/trilogy-group/k8s-webhooks/pkg/webhooks"
)

type Flags struct {
	version   bool
	overrides *clientcmd.ConfigOverrides

	wsFlags *webhooks.WhSrvFlags

	deploymentAffinity   bool
	ingressRewriteTarget bool
	jiveWebAppsAffinity  bool
}

func parseFlags() *Flags {
	flags := &Flags{}
	klog.InitFlags(nil)
	flags.overrides = &clientcmd.ConfigOverrides{}
	// clientcmd.BindOverrideFlags(
	// 	flags.overrides, flag.CommandLine,
	// 	clientcmd.ConfigOverrideFlags{
	// 		CurrentContext: clientcmd.FlagInfo{
	// 			clientcmd.FlagContext, "", "", "The name of the kubeconfig context to use",
	// 		},
	// 	})

	flag.BoolVar(&flags.version, "version", false, "Print version and exit")

	flags.wsFlags = &webhooks.WhSrvFlags{}
	webhooks.BindFlags(flags.wsFlags, flag.CommandLine)

	flag.BoolVar(&flags.deploymentAffinity, "deployment-affinity", false, "Setup deployment affinity webhook")
	flag.BoolVar(&flags.ingressRewriteTarget, "ingress-rewrite-target", false, "Setup ingress rewrite-target webhook")
	flag.BoolVar(&flags.jiveWebAppsAffinity, "jive-webapps-affinity", false, "Setup ingress jive webapp affinity webhook")

	flag.CommandLine.AddGoFlagSet(goflag.CommandLine)
	flag.Parse()
	return flags
}
