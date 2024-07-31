PROJECT = node-resource-exporter
IMAGE_VER := 0.3
IMAGE_NAME := ${PROJECT}:${IMAGE_VER}
IMAGE_URL := docker.io/dmitsh/${IMAGE_NAME}
NAMESPACE ?= monitoring

.PHONY: build
build:
	mkdir -p bin
	CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o ./bin/${PROJECT} ./cmd/${PROJECT}/

.PHONY: clean
clean:
	rm -f ./bin/${PROJECT}

.PHONY: image-build
image-build: build
	docker build -t ${IMAGE_URL} .

.PHONY: image-push
image-push: image-build
	docker push ${IMAGE_URL}

.PHONY: helm-install
helm-install:
	helm upgrade --install node-resource-exporter \
	--namespace ${NAMESPACE} --wait --timeout=300s \
	./deployments/node-resource-exporter
