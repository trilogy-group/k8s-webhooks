package server

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	"github.com/trilogy-group/k8s-webhooks/pkg/utils"
)

// for now manage only default admit policy
func (ws *webhookServer) setupConfigMap() {
	config := ws.GetConfig()
	cfg := utils.GetClientConfigOrDie(config.Kubeconfig)
	cs := utils.GetClientsetFromConfigOrDie(cfg)
	// get initial values from CM
	cm, err := cs.CoreV1().ConfigMaps(config.CmNamespace).
		Get(config.CmName, metav1.GetOptions{})
	if err != nil && apierrors.IsNotFound(err) {
		// we have to create the ConfigMap
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: config.CmNamespace,
				Name:      config.CmName,
			},
			Data: map[string]string{
				"DefaultAdmitPolicy": config.DefaultAdmitPolicy,
			},
		}
		cs.CoreV1().ConfigMaps(config.CmNamespace).Create(cm)
	}
	if policy, found := cm.Data["DefaultAdmitPolicy"]; found {
		config.DefaultAdmitPolicy = policy
	} else {
		// we should update the ConfigMap for the default admit policy
		if cm.Data == nil {
			cm.Data = make(map[string]string)
		}
		cm.Data["DefaultAdmitPolicy"] = config.DefaultAdmitPolicy
		cs.CoreV1().ConfigMaps(config.CmNamespace).Update(cm)
	}
	f := informers.NewSharedInformerFactory(cs, 10*time.Minute)
	ws.RegisterFactory("kubernetes", f)
	f.Core().V1().ConfigMaps().Informer().AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				cm, ok := obj.(*corev1.ConfigMap)
				if ok &&
					cm.ObjectMeta.Namespace == config.CmNamespace &&
					cm.ObjectMeta.Name == config.CmName {
					return true
				}
				return false
			},
			Handler: cache.ResourceEventHandlerFuncs{
				UpdateFunc: func(old interface{}, new interface{}) {
					cm := new.(*corev1.ConfigMap)
					if policy, found := cm.Data["DefaultAdmitPolicy"]; found {
						config.DefaultAdmitPolicy = policy
					} else {
						cm.Data["DefaultAdmitPolicy"] = config.DefaultAdmitPolicy
						cs.CoreV1().ConfigMaps(config.CmNamespace).Update(cm)
					}
				},
			},
		})
}
