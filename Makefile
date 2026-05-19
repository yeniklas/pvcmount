VERSION := $(shell git describe --tags --always --dirty)

build:
	go build -ldflags "-X main.version=$(VERSION)" -o pvcmount .

install:
	go install -ldflags "-X main.version=$(VERSION)" .

test-e2e:
	go test -tags e2e -v -run TestE2E -timeout 5m ./...
