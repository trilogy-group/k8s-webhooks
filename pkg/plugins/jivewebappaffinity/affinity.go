package jivewebappaffinity

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/klog"

	admissionV1beta1 "k8s.io/api/admission/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	l_autoscalingv1 "k8s.io/client-go/listers/autoscaling/v1"
	l_corev1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/trilogy-group/k8s-webhooks/pkg/utils"
	"github.com/trilogy-group/k8s-webhooks/pkg/webhooks"
)

const (
	configMapKey string = "jiveWebAppsAffinity"

	defaultMaximumHpaReplicas  int    = 10
	defaultHpaName             string = "webapp-hpa"
	defaultPodLabelForAffinity string = "jcx.inst.uri"
	defaultTopologyKey         string = "kubernetes.io/hostname"
	defaultNsLabelSelStr       string = "jcx.customer.id,jcx.environment,jcx.inst.uri,jcx.name,jcx.suspended=false"
	defaultHpaLabelSelStr      string = "jcx.environment"
	defaultNsPrefix            string = ""

	// factoryNS  string = "jivejcxwebappsnamespaces"
	// factoryHPA string = "jivejcxwebappshorizontalpodautoscalers"
)

var (
	maximumHpaReplicas  int    = defaultMaximumHpaReplicas
	hpaName             string = defaultHpaName
	podLabelForAffinity string = defaultPodLabelForAffinity
	topologyKey         string = defaultTopologyKey
	nsLabelSelStr       string = defaultNsLabelSelStr
	hpaLabelSelStr      string = defaultHpaLabelSelStr
	nsPrefix            string = defaultNsPrefix

	nsLabelSel, hpaLabelSel labels.Selector

	nsLister  l_corev1.NamespaceLister
	hpaLister l_autoscalingv1.HorizontalPodAutoscalerLister
)

type webhookHandler struct{}

func NewWebhookHandler() webhooks.WebhookHandler {
	return &webhookHandler{}
}

func (wh *webhookHandler) Setup(server webhooks.WebhookServer, path string) {
	var cs kubernetes.Interface
	var err error

	config := server.GetConfig()

	// Dynamic configuration management
	f := server.GetFactory("kubernetes")
	if f == nil {
		if cs == nil {
			cs = utils.GetClientsetOrDie(config.Kubeconfig, nil)
		}
		// get initial values from CM
		if cm, err := cs.CoreV1().ConfigMaps(config.CmNamespace).
			Get(config.CmName, metav1.GetOptions{}); err == nil {
			if _, ok := cm.Data[configMapKey]; ok {
				setVarsFromYAMLString(cm.Data[configMapKey])
			}
		}
		f := informers.NewSharedInformerFactory(cs, 300*time.Second)
		server.RegisterFactory("kubernetes", f)
	}
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
				AddFunc: func(obj interface{}) {
					onConfigMapUpdate(nil, obj)
				},
				UpdateFunc: onConfigMapUpdate,
			},
		})

	nsLister = f.Core().V1().Namespaces().Lister()
	hpaLister = f.Autoscaling().V1().HorizontalPodAutoscalers().Lister()

	// setup label selector for NSa and HPAs
	if nsLabelSel, err = labels.Parse(nsLabelSelStr); err != nil {
		klog.Fatalf("Invalid NS labels string: %s: %+v", nsLabelSelStr, err)
	}
	if hpaLabelSel, err = labels.Parse(hpaLabelSelStr); err != nil {
		klog.Fatalf("Invalid NS labels string: %s: %+v", hpaLabelSelStr, err)
	}

	server.RegisterHandler(path, mutateAffinity)
}

func getHardPodAntiAffinityTerm(labels map[string]string) corev1.PodAffinityTerm {
	return corev1.PodAffinityTerm{
		LabelSelector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
		TopologyKey: topologyKey,
	}
}

func isExistingPodAntiAffinityOk(terms []corev1.PodAffinityTerm) bool {
	for _, term := range terms {
		if term.LabelSelector != nil {
			if _, ok := term.LabelSelector.MatchLabels[podLabelForAffinity]; ok {
				return true
			}
		}
	}
	return false
}

// this is the implementation of the core logic
// the params can be from pod or deployment.spec.template
// this is getting pointers to be able to modify the structures as side-effect
// and fill with the rigth pod anti-affinity
// When this returning non-nil error means that the modification cannot be done, so
// the webhook should leave the obcjec unchanged.
// the return value in case of success is the first patch to the affinity attribute Op and Value,
// basically the "add" or "replcace" and the updated Affinity attribute
func checkAndUpdateAffinity(namespace string, metadata *metav1.ObjectMeta, spec *corev1.PodSpec) (string, interface{}, error) {
	klog.V(5).Infof("checkAndUpdateAffinity (in ns %s) on metadata: %+v -- spec: %+v", namespace, metadata, spec)
	{
		if list, err := nsLister.List(nsLabelSel); err != nil {
			klog.V(6).Infof("Listing cached NSs err: %+v", err)
		} else {
			klog.V(6).Infof("Listing cached NSs: %+v", list)
		}

		if list, err := hpaLister.List(hpaLabelSel); err != nil {
			klog.V(6).Infof("Listing cached HPAs err: %+v", err)
		} else {
			klog.V(6).Infof("Listing cached HPAs: %+v", list)
		}
	}

	// check for namespace prefix if we have to
	if len(nsPrefix) > 0 && !strings.HasPrefix(namespace, nsPrefix) {
		return "", nil, fmt.Errorf("Namespace %s has not prefix %s", namespace, nsPrefix)
	}

	// check for the label we want to use in pod anti-affinity
	if _, ok := metadata.Labels[podLabelForAffinity]; !ok {
		return "", nil, fmt.Errorf("Failed retrieving %s label on %s/%s",
			podLabelForAffinity, namespace, metadata.Name)
	}
	labelsForAffinity := make(map[string]string)
	labelsForAffinity[podLabelForAffinity] = metadata.Labels[podLabelForAffinity]

	// check if the Namespace is a jive jcx installation one
	ns, err := nsLister.Get(namespace)
	if err != nil {
		return "", nil, fmt.Errorf("Failed retrieving %s: %+v", namespace, err)
	}

	if !nsLabelSel.Matches(labels.Set(ns.ObjectMeta.Labels)) {
		// leave it unchanged
		return "", nil, fmt.Errorf("Namespace %s doesn't match labels", namespace)
	}

	// try to get the WebApp HPA in this NS
	hpa, err := hpaLister.HorizontalPodAutoscalers(ns.ObjectMeta.Name).Get(hpaName)
	if err != nil {
		// leave it unchanged
		return "", nil, fmt.Errorf("Failed retrieving %s/%s: %+v", ns.ObjectMeta.Name, hpaName, err)
	}

	if !hpaLabelSel.Matches(labels.Set(hpa.ObjectMeta.Labels)) {
		// leave it unchanged
		return "", nil, fmt.Errorf("HPA does't match labels")
	}

	// check if maxReplicas in this HPA is ok to set affinity
	if hpa.Spec.MaxReplicas > int32(maximumHpaReplicas) {
		// leave it unchanged
		return "", nil, fmt.Errorf("too much HPA maxReplicas")
	}

	affinityPatchOp := "replace"
	if spec.Affinity == nil {
		spec.Affinity = &corev1.Affinity{}
		affinityPatchOp = "add"
	}

	if spec.Affinity.PodAntiAffinity == nil {
		spec.Affinity.PodAntiAffinity = &corev1.PodAntiAffinity{}
	} else if isExistingPodAntiAffinityOk(
		spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution) {
		// leave it unchanged
		return "", nil, fmt.Errorf("No need to patch")
	}

	terms := spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	terms = append(terms, getHardPodAntiAffinityTerm(labelsForAffinity))
	spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution = terms

	return affinityPatchOp, spec.Affinity, nil

}

func mutateAffinity(ar *admissionV1beta1.AdmissionReview) *admissionV1beta1.AdmissionResponse {
	switch ar.Request.Kind.Kind {
	case "Deployment":
		return mutateDeploymentAffinity(ar)
	case "Pod":
		return mutatePodAffinity(ar)
	}
	return &admissionV1beta1.AdmissionResponse{Allowed: true}
}

func mutateDeploymentAffinity(ar *admissionV1beta1.AdmissionReview) *admissionV1beta1.AdmissionResponse {
	var patch []webhooks.PatchOperation
	var value interface{}
	var depl appsv1.Deployment

	if err := json.Unmarshal(ar.Request.Object.Raw, &depl); err != nil {
		klog.Errorf("Could not unmarshal raw object: %v", err)
		return &admissionV1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	// core logic
	affinityPatchOp, value, err := checkAndUpdateAffinity(
		ar.Request.Namespace,
		&depl.Spec.Template.ObjectMeta,
		&depl.Spec.Template.Spec)

	if err != nil {
		klog.Errorf("%v", err)
		// leave it unchanged
		return &admissionV1beta1.AdmissionResponse{Allowed: true}
	}

	patch = append(patch, webhooks.PatchOperation{
		Op:    affinityPatchOp,
		Path:  "/spec/template/spec/affinity",
		Value: value,
	})

	annotValue := "Deployment Affinity updated to spread across Nodes"
	if depl.Annotations == nil {
		patch = append(patch, webhooks.PatchOperation{
			Op:    "add",
			Path:  "/metadata/annotations",
			Value: map[string]string{"mutatingWebookAffinity": annotValue},
		})
	} else {
		patch = append(patch, webhooks.PatchOperation{
			Op:    "add",
			Path:  "/metadata/annotations/mutatingWebookAffinity",
			Value: annotValue,
		})
	}

	// end core logic

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		klog.Errorf("Could not marshal patch: %v", patch)
		return &admissionV1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	klog.Infof("AdmissionResponse: patch=%v\n", string(patchBytes))
	return &admissionV1beta1.AdmissionResponse{
		Allowed: true,
		Patch:   patchBytes,
		PatchType: func() *admissionV1beta1.PatchType {
			pt := admissionV1beta1.PatchTypeJSONPatch
			return &pt
		}(),
	}
}

func mutatePodAffinity(ar *admissionV1beta1.AdmissionReview) *admissionV1beta1.AdmissionResponse {
	var patch []webhooks.PatchOperation
	var value interface{}
	var pod corev1.Pod

	if err := json.Unmarshal(ar.Request.Object.Raw, &pod); err != nil {
		klog.Errorf("Could not unmarshal raw object: %v", err)
		return &admissionV1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	// core logic

	affinityPatchOp, value, err := checkAndUpdateAffinity(ar.Request.Namespace, &pod.ObjectMeta, &pod.Spec)
	if err != nil {
		klog.Errorf("%v", err)
		// leave it unchanged
		return &admissionV1beta1.AdmissionResponse{Allowed: true}
	}

	patch = append(patch, webhooks.PatchOperation{
		Op:    affinityPatchOp,
		Path:  "/spec/affinity",
		Value: value,
	})

	annotValue := "Pod Affinity updated to spread across AZs"
	if pod.Annotations == nil {
		patch = append(patch, webhooks.PatchOperation{
			Op:    "add",
			Path:  "/metadata/annotations",
			Value: map[string]string{"mutatingWebookAffinity": annotValue},
		})
	} else {
		patch = append(patch, webhooks.PatchOperation{
			Op:    "add",
			Path:  "/metadata/annotations/mutatingWebookAffinity",
			Value: annotValue,
		})
	}

	// end core logic

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		klog.Errorf("Could not marshal patch: %v", patch)
		return &admissionV1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	klog.Infof("AdmissionResponse: patch=%v\n", string(patchBytes))
	return &admissionV1beta1.AdmissionResponse{
		Allowed: true,
		Patch:   patchBytes,
		PatchType: func() *admissionV1beta1.PatchType {
			pt := admissionV1beta1.PatchTypeJSONPatch
			return &pt
		}(),
	}
}
