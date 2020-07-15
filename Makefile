VERSION = 0.1
REGISTRY ?= registry2.swarm.devfactory.com/central
FLAGS =
ENVVAR = CGO_ENABLED=0
GOOS ?= linux
LDFLAGS ?=
COMPONENT = webhooks-manager

DOCKER_IMAGE = "${REGISTRY}/${COMPONENT}:${VERSION}"

K8S_VERSION = 1.15.10
GNOSTIC_VERSION = 0.4.0

.PHONY: build static install_deps deps clean

golang:
	@echo "--> Go Version"
	@go version

install_deps:
	go get -d \
		k8s.io/client-go@kubernetes-${K8S_VERSION} \
		k8s.io/apimachinery@kubernetes-${K8S_VERSION} \
		github.com/googleapis/gnostic@v${GNOSTIC_VERSION}

deps:
	go mod verify && go mod tidy -v && go mod vendor -v

clean:
	rm -f ${COMPONENT}

clean-all: clean
	rm -rf vendor

build: golang
	@echo "--> Compiling the project"
	#$(ENVVAR) GOOS=$(GOOS) go install $(LDFLAGS) -v ./pkg/...
	$(ENVVAR) GOOS=$(GOOS) go build -mod=vendor $(LDFLAGS) \
				-ldflags "-w -X main.version=${VERSION}" -v -o ${COMPONENT} ./cmd/...

static: golang
	@echo "--> Compiling the static binary"
	#$(ENVVAR) GOOS=$(GOOS) go install $(LDFLAGS) -v ./pkg/...
	$(ENVVAR) GOARCH=amd64 GOOS=$(GOOS) \
		go build -mod=vendor -a -tags netgo \
			-ldflags "-w -X main.version=${VERSION}" -v -o ${COMPONENT} ./cmd/...

test:
	$(ENVVAR) GOOS=$(GOOS) go test -v ./...

docker: install_deps deps static
	docker build -t ${DOCKER_IMAGE} .
