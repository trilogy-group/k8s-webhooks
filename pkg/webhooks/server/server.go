package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"k8s.io/klog"

	. "github.com/trilogy-group/k8s-webhooks/pkg/webhooks"
	admissionV1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"

	"github.com/trilogy-group/k8s-webhooks/pkg/utils"
)

const (
	admissionDenied  string = "AdmissionDenied"
	admissionMutated string = "AdmissionMutated"
)

var (
	runtimeScheme *runtime.Scheme
	codecs        serializer.CodecFactory
	deserializer  runtime.Decoder
)

func init() {
	runtimeScheme = runtime.NewScheme()
	codecs = serializer.NewCodecFactory(runtimeScheme)
	deserializer = codecs.UniversalDeserializer()
}

type webhookServer struct {
	server    *http.Server
	config    *WebhookServerConfig
	handlers  HandlersMap
	stopCh    chan struct{}
	factories FactoriesMap
	cw        *CertWatcher
	recorder  record.EventRecorder
}

func (whsrv *webhookServer) GetConfig() *WebhookServerConfig {
	return whsrv.config
}

func (whsrv *webhookServer) Shutdown(ctxt context.Context) error {
	close(whsrv.stopCh)
	return whsrv.server.Shutdown(ctxt)
}

func (whsrv *webhookServer) Start() error {
	for fn, _ := range whsrv.factories {
		if err := whsrv.StartFactory(fn); err != nil {
			return err
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", whsrv.serve)
	whsrv.server.Handler = mux

	// start the certWatcher
	if whsrv.cw != nil {
		go func() {
			if err := whsrv.cw.Start(whsrv.stopCh); err != nil {
				klog.Errorf("certificate watcher error: %v", err)
			}
		}()
	}

	return whsrv.server.ListenAndServeTLS("", "")
}

// Serve method for webhook server
func (whsrv *webhookServer) serve(w http.ResponseWriter, r *http.Request) {
	klog.V(4).Infof("Start Serving request: %s", r.URL.Path)

	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}
	if len(body) == 0 {
		klog.Error("empty body")
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		klog.Errorf("Content-Type=%s, expect application/json", contentType)
		http.Error(w, "invalid Content-Type, expect `application/json`", http.StatusUnsupportedMediaType)
		return
	}

	var admissionResponse *admissionV1beta1.AdmissionResponse
	ar := admissionV1beta1.AdmissionReview{}
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		klog.Errorf("Can't decode body: %v", err)
		admissionResponse = &admissionV1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	if klog.V(5) {
		logUserInfo(&ar)
	}

	// try handlers
	handler := whsrv.GetHandlerForPath(r.URL.Path)
	admissionResponse = handler(&ar)

	admissionReview := admissionV1beta1.AdmissionReview{}
	if admissionResponse != nil {
		admissionReview.Response = admissionResponse
		if ar.Request != nil {
			admissionReview.Response.UID = ar.Request.UID
		}
	}

	resp, err := json.Marshal(admissionReview)
	if err != nil {
		klog.Errorf("Can't encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
		if ar.Request.Object.Object != nil {
			whsrv.recorder.Eventf(ar.Request.Object.Object, corev1.EventTypeWarning, "ErrorEncodingResponse", "could not encode response: %v", err)
		}
	}

	if _, err := w.Write(resp); err != nil {
		klog.Errorf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
		if ar.Request.Object.Object != nil {
			whsrv.recorder.Eventf(ar.Request.Object.Object, corev1.EventTypeWarning, "ErrorEncodingResponse", "could not write response: %v", err)
		}
	} else {
		klog.V(5).Infof("Response written! (was %v)", admissionReview)
		if ar.Request.Object.Object != nil && admissionResponse != nil {
			if admissionResponse.Result != nil && admissionResponse.Result.Message != "" {
				whsrv.recorder.Eventf(ar.Request.Object.Object, corev1.EventTypeWarning, "AdmissionFailed", admissionResponse.Result.Message)
			} else if !admissionResponse.Allowed {
				whsrv.recorder.Eventf(ar.Request.Object.Object, corev1.EventTypeNormal, admissionDenied,
					getEventMessage(&ar, admissionDenied, r.URL.Path))
			} else if len(admissionResponse.Patch) > 0 {
				whsrv.recorder.Eventf(ar.Request.Object.Object, corev1.EventTypeNormal, admissionMutated,
					getEventMessage(&ar, admissionMutated, r.URL.Path))
			}
		}
	}
}

var _ WebhookServer = &webhookServer{}

func NewWebhookServer(config *WebhookServerConfig, params *WhSrvParameters) WebhookServer {
	if config == nil {
		config = NewDefaultWebhookServerConfig()
	}
	if params == nil {
		params = NewDefaultWebhookServerParameters()
	}

	// Be aware of certificate changes, caused by renewal for ie
	// use certwatcher from controller-runtime
	//
	// pair, err := tls.LoadX509KeyPair(params.CertFile, params.KeyFile)
	// if err != nil {
	// 	klog.Fatalf("Failed to load key pair: %v", err)
	// }

	cw, err := NewCertWatcher(params.CertFile, params.KeyFile)
	if err != nil {
		klog.Fatalf("Failed to load key pair: %v", err)
	}

	ws := &webhookServer{
		config: config,
		stopCh: make(chan struct{}),
		server: &http.Server{
			Addr: fmt.Sprintf(":%v", params.Port),
			TLSConfig: &tls.Config{
				NextProtos:     []string{"h2"},
				GetCertificate: cw.GetCertificate,
			},
		},
		cw:       cw,
		recorder: createEventRecorder(config),
	}

	if config.UseConfigMap {
		ws.setupConfigMap()
	}

	return ws
}

func NewWebhookServerWithOptions(config *WebhookServerConfig, params *WhSrvParameters, options ...WebhookServerOption) WebhookServer {
	whsrv := NewWebhookServer(config, params)
	for _, opt := range options {
		whsrv = opt(whsrv)
	}
	return whsrv
}

// copied from cluster-autoscaler
func createEventRecorder(config *WebhookServerConfig) record.EventRecorder {
	cfg := utils.GetClientConfigOrDie(config.Kubeconfig)
	cs := utils.GetClientsetFromConfigOrDie(cfg)
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.V(4).Infof)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: v1core.New(cs.CoreV1().RESTClient()).Events("")})
	return eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "webhooks-manager"})
}

func getEventMessage(ar *admissionV1beta1.AdmissionReview, eventName, path string) string {
	msg := fmt.Sprintf("Handler for %s", path)

	switch eventName {
	case admissionDenied:
		msg = fmt.Sprintf("%s %s the %s operation for a %s", msg, "denied", ar.Request.Operation, ar.Request.Kind.Kind)
	case admissionMutated:
		msg = fmt.Sprintf("%s %s a %s", msg, "patched", ar.Request.Kind.Kind)
	}

	if ar.Request.Name != "" {
		msg = fmt.Sprintf("%s named %s", msg, ar.Request.Name)
	}
	if ar.Request.Namespace != "" {
		msg = fmt.Sprintf("%s in %s namespace", msg, ar.Request.Namespace)
	} else {
		msg = fmt.Sprintf("%s (cluster-scoped)", msg)
	}

	return msg
}

func logUserInfo(ar *admissionV1beta1.AdmissionReview) {
	ns := "(cluster-scoped)"
	if ar.Request.Namespace != "" {
		ns = fmt.Sprintf("in %s", ar.Request.Namespace)
	}
	named := ""
	if ar.Request.Name != "" {
		named = fmt.Sprintf(" named %s", ar.Request.Name)
	}
	klog.Infof("UserInfo: %+v requested to %s a %s%s %s",
		ar.Request.UserInfo, ar.Request.Operation, ar.Request.Kind.Kind, named, ns)
}
