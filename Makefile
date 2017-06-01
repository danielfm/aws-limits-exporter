GO=CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go
TAG=0.1.1
BIN=kube-aws-limits-monitor
IMAGE=danielfm/$(BIN)

.PHONY: build
build:
	$(GO) build -a --ldflags "-X main.VERSION=$(TAG) -w -extldflags '-static'" -tags netgo -o bin/$(BIN) ./cmd

.PHONY: image
image: build
	docker build -t $(IMAGE):$(TAG) .

.PHONY: push
push: image
	docker push $(IMAGE):$(TAG)

.PHONY: push-latest
push-latest: image
	docker tag $(IMAGE):$(TAG) $(IMAGE):latest
	docker push $(IMAGE):latest
