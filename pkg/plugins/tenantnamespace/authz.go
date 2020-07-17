package tenantnamespace

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	"k8s.io/klog"

	admissionV1beta1 "k8s.io/api/admission/v1beta1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	l_corev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/trilogy-group/k8s-webhooks/pkg/utils"
	"github.com/trilogy-group/k8s-webhooks/pkg/webhooks"
)

const (
	configMapKey string = "tenantNamespacesAuthz"

	saUserPrefix string = "system:serviceaccounts:"
)

type serviceAccountAuthzRules struct {
	// validation
	AllowedNamespacePrefix      string            `yaml:"allowedNamespacePrefix"`
	DisallowedNamespacePrefix   string            `yaml:"disallowedNamespacePrefix"`
	RequiredNamespaceLabelsMap  map[string]string `yaml:"requiredNamespaceLabelsMap"`
	RequiredNamespaceLabelsKeys []string          `yaml:"requiredNamespaceLabelsKeys"`
	DeniedNamespaceMatchLabels  map[string]string `yaml:"deniedNamespaceMatchLabels"`

	// mutation
	AdditionalLabels      map[string]string `yaml:"additionalLabels"`
	AdditionalAnnotations map[string]string `yaml:"additionalAnnotations"`
}

type authzConfig struct {
	sync.RWMutex

	Rules map[string]serviceAccountAuthzRules `yaml:"rules"`
}

func (ac *authzConfig) isServiceAccountUser(ui authenticationv1.UserInfo) bool {
	return strings.HasPrefix(ui.Username, saUserPrefix) && len(strings.Split(ui.Username, ":")) == 4
}

func (ac *authzConfig) getServiceAccountKeyFromUser(ui authenticationv1.UserInfo) strings {
	return strings.Join(strings.Split(ui.Username, ":")[2:], "/")
}

func (ac *authzConfig) Parse(data string) err {
	ac.Lock()
	defer ac.Unlock()
	ac.Rules = nil
	return yaml.Unmarshal([]byte(data), ac)
}

func (ac *authzConfig) canUserOperateOn(ui authenticationv1.UserInfo, ns *corev1.Namespace) bool {
	if !ac.isServiceAccountUser(ui) {
		return true
	}
	saKey := getServiceAccountKeyFromUser(ui)
	ac.RLock()
	defer ac.RUnlock()

	if _, found := ac.Rules[saKey]; !found {
		return false
	}
	saRules := ac.Rules[saKey]
	if !strings.HasPrefix(ns.Object.Meta.Name, saRules.AllowedNamespacePrefix) {
		return false
	}

	return true
}

func NewAuthzConfig(data string) (*authzConfig, err) {
	ac := &authzConfig{}
	if err := ac.Parse(data); err != nil {
		return nil, err
	}
	return ac, nil
}

/// from playground
// package main

// import (
// 	"fmt"
// 	"gopkg.in/yaml.v3"
// )

// type serviceAccountAuthzRules struct {
// 	// validation
// 	AllowedNamespacePrefix      string            `yaml:"allowedNamespacePrefix"`
// 	DisllowedNamespacePrefix    string            `yaml:"disllowedNamespacePrefix"`
// 	RequiredNamespaceLabelsMap  map[string]string `yaml:"requiredNamespaceLabelsMap"`
// 	RequiredNamespaceLabelsKeys []string          `yaml:"requiredNamespaceLabelsKeys"`

// 	// mutation
// 	AdditionalLabels      map[string]string `yaml:"additionalLabels"`
// 	AdditionalAnnotations map[string]string `yaml:"additionalAnnotations"`
// }

// type authzConfig struct {
// 	Rules map[string]serviceAccountAuthzRules `yaml:"rules"`
// }

// var data string = `
// rules:
//   ns1/sa1:
//     allowedNamespacePrefix: sa-one-
//     additionalAnnotations:
//       annot1: value_annot_1
// `

// func main() {
// 	ac := authzConfig{}
// 	fmt.Printf("Hello, playground: %+v\n", ac)
// 	yaml.Unmarshal([]byte(data), &ac)
// 	fmt.Printf("Hello, playground: %+v\n", ac.Rules["ns1/sa1"].AllowedNamespacePrefix)
// }
