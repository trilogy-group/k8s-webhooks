package main

import (
	"k8s.io/klog"

	admissionV1beta1 "k8s.io/api/admission/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/informers"
	// "k8s.io/client-go/kubernetes"
	// // appsv1listers "k8s.io/client-go/listers/apps/v1"
	// "k8s.io/client-go/rest"

	"github.com/trilogy-group/k8s-webhooks/pkg/utils"
	"github.com/trilogy-group/k8s-webhooks/pkg/webhooks"
	// "github.com/trilogy-group/k8s-webhooks/pkg/webhooks/server"
)

var (
	minimumReplicasForAffinity int = 3
	weightForAZAffinity        int = 100
)

func LoadWebhookPlugin(ws webhooks.WebhookServer) {
	f := ws.GetFactory("kubernets")
	if f == nil {
		f = informers.NewSharedInformerFactory(utils.GetClientsetFromConfigOrDie(utils.GetClientConfigOrDie), 0)
	}
}

func getWeightedPodAffinityTerms() (ret []corev1.WeightedPodAffinityTerm) {
	ret = append(ret, corev1.WeightedPodAffinityTerm{
		Weight: weightForAZAffinity,
		PodAffinityTerm: corev1.PodAffinityTerm{
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: depl.Spec.Template.ObjectMeta.Labels,
			},
			TopologyKey: "failure-domain.beta.kubernetes.io/zone",
		},
	})
	return ret
}

func Admit(ar *admissionV1beta1.AdmissionReview) *admissionV1beta1.AdmissionResponse {
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

	// Operate only on creation
	if ar.Request.Operation != admissionV1beta1.Create {
		// leave it unchanged
		return &admissionV1beta1.AdmissionResponse{Allowed: true}
	}

	// check if replicas is >= 3 and there is no affinity in Spec
	if *depl.Spec.Replicas < 3 {
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
			PreferredDuringSchedulingIgnoredDuringExecution: getWeightedPodAffinityTerms(),
		},
	}

	value = depl.Spec.Template.Spec.Affinity
	patch = append(patch, webhooks.PatchOperation{
		Op:    "add",
		Path:  "/spec/template/spec/affinity",
		Value: value,
	})

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
