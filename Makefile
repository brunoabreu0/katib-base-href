HAS_LINT := $(shell command -v golangci-lint;)
HAS_YAMLLINT := $(shell command -v yamllint;)
HAS_SHELLCHECK := $(shell command -v shellcheck;)
HAS_SETUP_ENVTEST := $(shell command -v setup-envtest;)
HAS_MOCKGEN := $(shell command -v mockgen;)

COMMIT := v1beta1-$(shell git rev-parse --short=7 HEAD)
KATIB_REGISTRY := docker.io/kubeflowkatib
CPU_ARCH ?= amd64
ENVTEST_K8S_VERSION ?= 1.26
MOCKGEN_VERSION ?= $(shell grep 'github.com/golang/mock' go.mod | cut -d ' ' -f 2)
GO_VERSION=$(shell grep '^go' go.mod | cut -d ' ' -f 2)

# for pytest
PYTHONPATH := $(PYTHONPATH):$(CURDIR)/pkg/apis/manager/v1beta1/python:$(CURDIR)/pkg/apis/manager/health/python
PYTHONPATH := $(PYTHONPATH):$(CURDIR)/pkg/metricscollector/v1beta1/common:$(CURDIR)/pkg/metricscollector/v1beta1/tfevent-metricscollector
TEST_TENSORFLOW_EVENT_FILE_PATH ?= $(CURDIR)/test/unit/v1beta1/metricscollector/testdata/tfevent-metricscollector/logs

# Run tests
.PHONY: test
test: envtest
	KUBEBUILDER_ASSETS="$(shell setup-envtest --arch=amd64 use $(ENVTEST_K8S_VERSION) -p path)" go test ./pkg/... ./cmd/... -coverprofile coverage.out

envtest:
ifndef HAS_SETUP_ENVTEST
	go install sigs.k8s.io/controller-runtime/tools/setup-envtest@2c3a6fa2996c026b284c7fe2b055274cd9a556bc #v0.14.6
	$(info "setup-envtest has been installed")
endif
	$(info "setup-envtest has already installed")

check: generated-codes go-mod fmt vet lint

fmt:
	hack/verify-gofmt.sh

lint:
ifndef HAS_LINT
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.50.1
	$(info "golangci-lint has been installed")
endif
	hack/verify-golangci-lint.sh

yamllint:
ifndef HAS_YAMLLINT
	pip install --prefer-binary yamllint
	$(info "yamllint has been installed")
endif
	hack/verify-yamllint.sh

vet:
	go vet ./pkg/... ./cmd/...

shellcheck:
ifndef HAS_SHELLCHECK
	bash hack/install-shellcheck.sh
	$(info "shellcheck has been installed")
endif
	hack/verify-shellcheck.sh

update:
	hack/update-gofmt.sh

# Deploy Katib v1beta1 manifests using Kustomize into a k8s cluster.
deploy:
	bash scripts/v1beta1/deploy.sh $(WITH_DATABASE_TYPE)

# Undeploy Katib v1beta1 manifests using Kustomize from a k8s cluster
undeploy:
	bash scripts/v1beta1/undeploy.sh

generated-codes: generate
ifneq ($(shell bash hack/verify-generated-codes.sh '.'; echo $$?),0)
	$(error 'Please run "make generate" to generate codes')
endif

go-mod: sync-go-mod
ifneq ($(shell bash hack/verify-generated-codes.sh 'go.*'; echo $$?),0)
	$(error 'Please run "go mod tidy -go $(GO_VERSION)" to sync Go modules')
endif

sync-go-mod:
	go mod tidy -go $(GO_VERSION)

# Run this if you update any existing controller APIs.
# 1. Generate deepcopy, clientset, listers, informers for the APIs (hack/update-codegen.sh)
# 2. Generate open-api for the APIs (hack/update-openapigen)
# 3. Generate Python SDK for Katib (hack/gen-python-sdk/gen-sdk.sh)
# 4. Generate gRPC manager APIs (pkg/apis/manager/v1beta1/build.sh and pkg/apis/manager/health/build.sh)
# 5. Generate Go mock codes
generate:
ifndef GOPATH
	$(error GOPATH not defined, please define GOPATH. Run "go help gopath" to learn more about GOPATH)
endif
ifndef HAS_MOCKGEN
	go install github.com/golang/mock/mockgen@$(MOCKGEN_VERSION)
	$(info "mockgen has been installed")
endif
	go generate ./pkg/... ./cmd/...
	hack/gen-python-sdk/gen-sdk.sh
	pkg/apis/manager/v1beta1/build.sh
	pkg/apis/manager/health/build.sh
	hack/update-mockgen.sh

# Build images for the Katib v1beta1 components.
build: generate
ifeq ($(and $(REGISTRY),$(TAG),$(CPU_ARCH)),)
	$(error REGISTRY and TAG must be set. Usage: make build REGISTRY=<registry> TAG=<tag> CPU_ARCH=<cpu-architecture>)
endif
	bash scripts/v1beta1/build.sh $(REGISTRY) $(TAG) $(CPU_ARCH)

# Build and push Katib images from the latest master commit.
push-latest: generate
	bash scripts/v1beta1/build.sh $(KATIB_REGISTRY) latest $(CPU_ARCH)
	bash scripts/v1beta1/build.sh $(KATIB_REGISTRY) $(COMMIT) $(CPU_ARCH)
	bash scripts/v1beta1/push.sh $(KATIB_REGISTRY) latest
	bash scripts/v1beta1/push.sh $(KATIB_REGISTRY) $(COMMIT)

# Build and push Katib images for the given tag.
push-tag: generate
ifeq ($(TAG),)
	$(error TAG must be set. Usage: make push-tag TAG=<release-tag>)
endif
	bash scripts/v1beta1/build.sh $(KATIB_REGISTRY) $(TAG) $(CPU_ARCH)
	bash scripts/v1beta1/build.sh $(KATIB_REGISTRY) $(COMMIT) $(CPU_ARCH)
	bash scripts/v1beta1/push.sh $(KATIB_REGISTRY) $(TAG)
	bash scripts/v1beta1/push.sh $(KATIB_REGISTRY) $(COMMIT)

# Release a new version of Katib.
release:
ifeq ($(and $(BRANCH),$(TAG)),)
	$(error BRANCH and TAG must be set. Usage: make release BRANCH=<branch> TAG=<tag>)
endif
	bash scripts/v1beta1/release.sh $(BRANCH) $(TAG)

# Update all Katib images.
update-images:
ifeq ($(and $(OLD_PREFIX),$(NEW_PREFIX),$(TAG)),)
	$(error OLD_PREFIX, NEW_PREFIX, and TAG must be set. \
	Usage: make update-images OLD_PREFIX=<old-prefix> NEW_PREFIX=<new-prefix> TAG=<tag> \
	For more information, check this file: scripts/v1beta1/update-images.sh)
endif
	bash scripts/v1beta1/update-images.sh $(OLD_PREFIX) $(NEW_PREFIX) $(TAG)

# Prettier UI format check for Katib v1beta1.
prettier-check:
	npm run format:check --prefix pkg/ui/v1beta1/frontend

# Update boilerplate for the source code.
update-boilerplate:
	./hack/boilerplate/update-boilerplate.sh

prepare-pytest:
	pip install --prefer-binary -r test/unit/v1beta1/requirements.txt
	pip install --prefer-binary -r cmd/suggestion/hyperopt/v1beta1/requirements.txt
	pip install --prefer-binary -r cmd/suggestion/skopt/v1beta1/requirements.txt
	pip install --prefer-binary -r cmd/suggestion/optuna/v1beta1/requirements.txt
	pip install --prefer-binary -r cmd/suggestion/hyperband/v1beta1/requirements.txt
	pip install --prefer-binary -r cmd/suggestion/nas/enas/v1beta1/requirements.txt
	pip install --prefer-binary -r cmd/suggestion/nas/darts/v1beta1/requirements.txt
	pip install --prefer-binary -r cmd/suggestion/pbt/v1beta1/requirements.txt
	pip install --prefer-binary -r cmd/earlystopping/medianstop/v1beta1/requirements.txt
	pip install --prefer-binary -r cmd/metricscollector/v1beta1/tfevent-metricscollector/requirements.txt

prepare-pytest-testdata:
ifeq ("$(wildcard $(TEST_TENSORFLOW_EVENT_FILE_PATH))", "")
	python examples/v1beta1/trial-images/tf-mnist-with-summaries/mnist.py --epochs 5 --batch-size 200 --log-path $(TEST_TENSORFLOW_EVENT_FILE_PATH)
endif

pytest: prepare-pytest prepare-pytest-testdata
	PYTHONPATH=$(PYTHONPATH) pytest ./test/unit/v1beta1/suggestion
	PYTHONPATH=$(PYTHONPATH) pytest ./test/unit/v1beta1/earlystopping
	PYTHONPATH=$(PYTHONPATH) pytest ./test/unit/v1beta1/metricscollector
