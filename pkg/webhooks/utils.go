package webhooks

import (
	admissionV1beta1 "k8s.io/api/admission/v1beta1"
)

type PatchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func AdmitAlways(*admissionV1beta1.AdmissionReview) *admissionV1beta1.AdmissionResponse {
	return &admissionV1beta1.AdmissionResponse{Allowed: true}
}

func AdmitNever(*admissionV1beta1.AdmissionReview) *admissionV1beta1.AdmissionResponse {
	return &admissionV1beta1.AdmissionResponse{Allowed: false}
}
