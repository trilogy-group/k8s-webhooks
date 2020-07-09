package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"k8s.io/klog"

	admissionV1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	. "github.com/trilogy-group/k8s-webhooks/pkg/webhooks"
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

	return whsrv.server.ListenAndServeTLS("", "")
}

// Serve method for webhook server
func (whsrv *webhookServer) serve(w http.ResponseWriter, r *http.Request) {
	klog.Infof("Start Serving request: %s", r.URL.Path)

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
	}

	if _, err := w.Write(resp); err != nil {
		klog.Errorf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	} else {
		klog.Infof("Response written! (was %v)", admissionReview)
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
	pair, err := tls.LoadX509KeyPair(params.CertFile, params.KeyFile)
	if err != nil {
		klog.Fatalf("Failed to load key pair: %v", err)
	}

	ws := &webhookServer{
		config: config,
		stopCh: make(chan struct{}),
		server: &http.Server{
			Addr:      fmt.Sprintf(":%v", params.Port),
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{pair}},
		},
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
