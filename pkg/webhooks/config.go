package webhooks

import (
	// "flag"
	"github.com/spf13/pflag"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	defaultUseConfigMap       bool   = false
	defaultConfigMapNamespace string = "kube-system"
	defaultConfigMapName      string = "webhooks-manager-config"
	defaultKubeconfig         string = clientcmd.RecommendedConfigPathFlag

	defaultPluginsDir string = "/webhookplugins"
	defaultAdmit      string = "Always"

	defaultPort     int    = 443
	defaultCertFile string = "/etc/webhook/certs/cert.pem"
	defaultKeyFile  string = "/etc/webhook/certs/key.pem"
)

// TODO: drop WhSrvFlags and WhSrvParameters, use directly WebhookServerConfig

type WhSrvFlags struct {
	Port     int    `json:"port"`
	CertFile string `json:"certFile"`
	KeyFile  string `json:"keyFile"`

	UseConfigMap bool   `json:"useConfigMap"`
	Kubeconfig   string `json:"kubeconfig"`
	CmNamespace  string `json:"configMapNamespace"`
	CmName       string `json:"configMapName"`

	PluginsDir         string `json:"pluginsDir"`
	DefaultAdmitPolicy string `json:"defaultAdmitPolicy"`
}

func BindFlags(flags *WhSrvFlags, fs *pflag.FlagSet) {
	fs.IntVar(&flags.Port, "port", defaultPort, "Listen on port. Default: 443")
	fs.StringVar(&flags.CertFile, "tlsCertFile", defaultCertFile, "File containing the x509 Certificate for HTTPS.")
	fs.StringVar(&flags.KeyFile, "tlsKeyFile", defaultKeyFile, "File containing the x509 private key to --tlsCertFile.")

	fs.BoolVar(&flags.UseConfigMap, "use-config-map", defaultUseConfigMap, "Optional absolute path to the kubeconfig file")
	fs.StringVar(&flags.Kubeconfig, "kubeconfig", defaultKubeconfig, "Optional absolute path to the kubeconfig file")
	fs.StringVar(&flags.CmNamespace, "config-map-namespace", defaultConfigMapNamespace, "")
	fs.StringVar(&flags.CmName, "config-map-name", defaultConfigMapName, "")

	fs.StringVar(&flags.PluginsDir, "plugins-dir", defaultPluginsDir, "")
	fs.StringVar(&flags.DefaultAdmitPolicy, "default-admit-policy", defaultAdmit, "")
}

type WhSrvParameters struct {
	Port     int    // webhook server port
	CertFile string // path to the x509 certificate for https
	KeyFile  string // path to the x509 private key matching `CertFile`
}

func NewDefaultWebhookServerParameters() *WhSrvParameters {
	return &WhSrvParameters{
		Port:     defaultPort,
		CertFile: defaultCertFile,
		KeyFile:  defaultKeyFile,
	}
}

type pluggedHandler struct {
	name            string `yaml:"name"`
	filename        string `yaml:"filename"`
	handlerFuncName string `yaml:"handler"`
}

type WebhookServerConfig struct {
	DefaultAdmitPolicy string
	handlersMapYAML    string
	handlersMap        map[string]pluggedHandler

	UseConfigMap bool
	Kubeconfig   string
	CmNamespace  string
	CmName       string
}

func NewDefaultWebhookServerConfig() *WebhookServerConfig {
	return &WebhookServerConfig{
		handlersMapYAML:    `{}`,
		DefaultAdmitPolicy: defaultAdmit,
		UseConfigMap:       false,
		Kubeconfig:         defaultKubeconfig,
		CmNamespace:        defaultConfigMapNamespace,
		CmName:             defaultConfigMapName,
	}
}
