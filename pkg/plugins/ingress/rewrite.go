package ingress

import (
	"strings"

	"k8s.io/klog"

	admissionV1beta1 "k8s.io/api/admission/v1beta1"
	extensionsV1beta1 "k8s.io/api/extensions/v1beta1"
	// corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"

	"github.com/trilogy-group/k8s-webhooks/pkg/webhooks"
)

type webhookHandler struct{}

func NewWebhookHandler() webhooks.WebhookHandler {
	return &webhookHandler{}
}

func (wh *webhookHandler) Setup(server webhooks.WebhookServer, path string) {
	server.RegisterHandler(path, mutateIngressRewriteTarget)
}

const (
	rewriteTargetAnnotKey = "nginx.ingress.kubernetes.io/rewrite-target"
)

func mutateIngressRewriteTarget(ar *admissionV1beta1.AdmissionReview) *admissionV1beta1.AdmissionResponse {
	var patch []webhooks.PatchOperation
	var value interface{}
	var ing extensionsV1beta1.Ingress

	if err := json.Unmarshal(ar.Request.Object.Raw, &ing); err != nil {
		klog.Errorf("Could not unmarshal raw object: %v", err)
		return &admissionV1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	// core logic
	v, hasRewrite := ing.ObjectMeta.Annotations[rewriteTargetAnnotKey]
	if !hasRewrite {
		// leave it unchanged
		return &admissionV1beta1.AdmissionResponse{Allowed: true}
	}
	if strings.Contains(v, "$") { // not very robust
		// already has regex group reference
		// leave it unchanged
		return &admissionV1beta1.AdmissionResponse{Allowed: true}
	}

	needsDelete := true
	for _, r := range ing.Spec.Rules {
		if r.IngressRuleValue.HTTP == nil {
			needsDelete = needsDelete && true
			continue
		}
		for ip, p := range r.IngressRuleValue.HTTP.Paths {
			if p.Path == v && v == "/" {
				needsDelete = needsDelete && true
				continue
			}
			needsDelete = false
			if !strings.HasSuffix(p.Path, "/") {
				p.Path = p.Path + "/"
			}
			p.Path = p.Path + "?(.*)"
			r.IngressRuleValue.HTTP.Paths[ip] = p
		}
	}

	if needsDelete {
		delete(ing.ObjectMeta.Annotations, rewriteTargetAnnotKey)
		value = ing.ObjectMeta.Annotations
		patch = append(patch, webhooks.PatchOperation{
			Op:    "replace",
			Path:  "/metadata/annotations",
			Value: value,
		})
	} else {
		// needs patch
		if !strings.HasSuffix(v, "/") {
			v = v + "/"
		}
		v = v + "$1"
		ing.ObjectMeta.Annotations[rewriteTargetAnnotKey] = v
		value = ing.ObjectMeta.Annotations
		patch = append(patch, webhooks.PatchOperation{
			Op:    "replace",
			Path:  "/metadata/annotations",
			Value: value,
		})

		value = ing.Spec.Rules
		patch = append(patch, webhooks.PatchOperation{
			Op:    "replace",
			Path:  "/spec/rules",
			Value: value,
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
