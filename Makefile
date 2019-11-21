.PHONY: test
export GOBIN := $(PWD)/bin
export PATH := $(GOBIN):$(PATH)
export INSTALL_FLAG=
export TAG=0.2.0

DOCKER_IMAGE = aws-limits-exporter
DOCKER_REPO = danielfm

# Determine which OS.
OS?=$(shell uname -s | tr A-Z a-z)

default: build

dependencies:
	@go mod tidy -v

dep: dependencies

run-server: build
	$(GOBIN)/aws-limits-exporter

run-linux: build
	$(GOBIN)/aws-limits-exporter

test:
	@go test ./... -timeout 2m -v -race

test-cover:
	@go test ./... -timeout 2m -race -cover

build:
	CGO_ENABLED=0 GOOS=$(OS) go build $(INSTALL_FLAG) -a --ldflags "-X main.VERSION=$(TAG) -w -extldflags '-static'" -tags netgo -o $(GOBIN)/aws-limits-exporter ./cmd

clean:
	@go clean

docker-build:
	docker build -t ${DOCKER_REPO}/$(DOCKER_IMAGE):latest .

docker-deploy:
	docker tag ${DOCKER_REPO}/$(DOCKER_IMAGE):latest ${DOCKER_REPO}/$(DOCKER_IMAGE):$(TAG)
	docker push ${DOCKER_REPO}/$(DOCKER_IMAGE):$(TAG)
	docker push ${DOCKER_REPO}/$(DOCKER_IMAGE):latest
