package affinity

import (
	"strconv"

	"k8s.io/klog"

	admissionV1beta1 "k8s.io/api/admission/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"

	"github.com/trilogy-group/k8s-webhooks/pkg/utils"
	"github.com/trilogy-group/k8s-webhooks/pkg/webhooks"
)

const (
	defaultMinimumReplicasForAffinity int    = 3
	defaultWeightForAffinity          int    = 100
	defaultTopologyKey                string = "failure-domain.beta.kubernetes.io/zone"
)

var (
	minimumReplicasForAffinity int    = defaultMinimumReplicasForAffinity
	weightForAffinity          int    = defaultWeightForAffinity
	topologyKeyForAffinity     string = defaultTopologyKey

	replicaSetIndexer, deploymentIndexer cache.Indexer
)

type webhookHandler struct{}

func NewWebhookHandler() webhooks.WebhookHandler {
	return &webhookHandler{}
}

func setVarsOrDefaults(data map[string]string) {
	if val, found := data["minimumReplicasForAffinity"]; found {
		if ival, err := strconv.Atoi(val); err == nil {
			minimumReplicasForAffinity = ival
		}
	} else {
		minimumReplicasForAffinity = defaultMinimumReplicasForAffinity
	}
	if val, found := data["weightForAffinity"]; found {
		if ival, err := strconv.Atoi(val); err == nil {
			weightForAffinity = ival
		}
	} else {
		weightForAffinity = defaultWeightForAffinity
	}
	if val, found := data["topologyKeyForAffinity"]; found {
		topologyKeyForAffinity = val
	} else {
		topologyKeyForAffinity = defaultTopologyKey
	}
}

func (wh *webhookHandler) Setup(server webhooks.WebhookServer, path string) {
	config := server.GetConfig()
	f := server.GetFactory("kubernetes")
	if f == nil {
		cfg := utils.GetClientConfigOrDie(config.Kubeconfig)
		cs := utils.GetClientsetFromConfigOrDie(cfg)
		// get initial values from CM
		if cm, err := cs.CoreV1().ConfigMaps(config.CmNamespace).
			Get(config.CmName, metav1.GetOptions{}); err == nil {
			setVarsOrDefaults(cm.Data)
		}
		f := informers.NewSharedInformerFactory(cs, 0)
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

	f.Apps().V1().ReplicaSets().Informer().AddIndexers(map[string]cache.IndexFunc{
		"uid": utils.GetObjectUIDIndexFunc(),
	})
	f.Apps().V1().Deployments().Informer().AddIndexers(map[string]cache.IndexFunc{
		"uid": utils.GetObjectUIDIndexFunc(),
	})
	replicaSetIndexer = f.Apps().V1().ReplicaSets().Informer().GetIndexer()
	deploymentIndexer = f.Apps().V1().Deployments().Informer().GetIndexer()

	server.RegisterHandler(path, mutateAffinity)
}

func onConfigMapUpdate(old interface{}, new interface{}) {
	if cm, ok := new.(*corev1.ConfigMap); ok {
		setVarsOrDefaults(cm.Data)
	}
}

func getWeightedPodAffinityTerms(labels map[string]string) (ret []corev1.WeightedPodAffinityTerm) {
	ret = append(ret, corev1.WeightedPodAffinityTerm{
		Weight: int32(weightForAffinity),
		PodAffinityTerm: corev1.PodAffinityTerm{
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			TopologyKey: topologyKeyForAffinity,
		},
	})
	return ret
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

	// check if replicas is >= 3 and there is no affinity in Spec
	if *depl.Spec.Replicas < int32(minimumReplicasForAffinity) {
		// leave it unchanged
		return &admissionV1beta1.AdmissionResponse{Allowed: true}
	}
	if depl.Spec.Template.Spec.Affinity != nil {
		// leave it unchanged
		return &admissionV1beta1.AdmissionResponse{Allowed: true}
	}

	// prepare the patch adding affinity podAntiAffinity by AZs
	depl.Spec.Template.Spec.Affinity = &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: getWeightedPodAffinityTerms(depl.Spec.Template.ObjectMeta.Labels),
		},
	}

	value = depl.Spec.Template.Spec.Affinity
	patch = append(patch, webhooks.PatchOperation{
		Op:    "add",
		Path:  "/spec/template/spec/affinity",
		Value: value,
	})

	annotValue := "Deployment Affinity updated to spread across AZs"
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

func getReplicaSetFromPod(pod *corev1.Pod) *appsv1.ReplicaSet {
	if len(pod.ObjectMeta.OwnerReferences) != 1 {
		return nil
	}
	if pod.ObjectMeta.OwnerReferences[0].Kind != "ReplicaSet" {
		return nil
	}

	res, err := replicaSetIndexer.ByIndex(
		"uid", string(pod.ObjectMeta.OwnerReferences[0].UID))
	if err != nil || len(res) != 1 {
		// we cannot manage this, leave it unchanged
		return nil
	}

	return res[0].(*appsv1.ReplicaSet)
}

func getDeploymentFromReplicaSet(rs *appsv1.ReplicaSet) *appsv1.Deployment {
	if len(rs.ObjectMeta.OwnerReferences) != 1 {
		return nil
	}
	if rs.ObjectMeta.OwnerReferences[0].Kind != "Deployment" {
		return nil
	}

	res, err := deploymentIndexer.ByIndex(
		"uid", string(rs.ObjectMeta.OwnerReferences[0].UID))
	if err != nil || len(res) != 1 {
		// we cannot manage this, leave it unchanged
		return nil
	}

	return res[0].(*appsv1.Deployment)
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
	rs := getReplicaSetFromPod(&pod)
	if rs == nil {
		// we cannot manage this, leave it unchanged
		return &admissionV1beta1.AdmissionResponse{Allowed: true}
	}
	depl := getDeploymentFromReplicaSet(rs)
	if depl == nil {
		// we cannot manage this, leave it unchanged
		return &admissionV1beta1.AdmissionResponse{Allowed: true}
	}

	// check if replicas is >= 3 and there is no affinity in Spec
	if *depl.Spec.Replicas < int32(minimumReplicasForAffinity) {
		// leave it unchanged
		return &admissionV1beta1.AdmissionResponse{Allowed: true}
	}
	if pod.Spec.Affinity != nil {
		// leave it unchanged
		return &admissionV1beta1.AdmissionResponse{Allowed: true}
	}

	// prepare the patch adding affinity podAntiAffinity by AZs
	pod.Spec.Affinity = &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: getWeightedPodAffinityTerms(depl.Spec.Template.ObjectMeta.Labels),
		},
	}

	value = pod.Spec.Affinity
	patch = append(patch, webhooks.PatchOperation{
		Op:    "add",
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
