IMG ?= ghcr.io/unict-cclab/cluster-lens:latest

.PHONY: run test build build-image push-image

run:
	cd backend && go run .

test:
	cd backend && go test ./...

build:
	cd backend && go build -buildvcs=false

build-image:
	docker build -t ${IMG} .

push-image:
	docker push ${IMG}
