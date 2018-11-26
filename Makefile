BUILD=go build
CLEAN=go clean
INSTALL=go install
BUILDPATH=./build
PACKAGES=$(shell go list ./... )

.PHONY: clean build all godep install

all: test build

build: dir
	go build -tags openvino -o "$(BUILDPATH)/monitor"

dir:
	mkdir -p $(BUILDPATH)

install:
	$(INSTALL) -tags openvino

clean:
	rm -rf $(BUILDPATH)/*

godep:
	wget -O- https://raw.githubusercontent.com/golang/dep/master/install.sh | sh

dep:
	dep ensure -v

check:
	for pkg in ${PACKAGES}; do \
		go vet $$pkg || exit ; \
		golint $$pkg || exit ; \
	done

test:
	for pkg in ${PACKAGES}; do \
		go test -tags openvino -coverprofile="../../../$$pkg/coverage.txt" -covermode=atomic $$pkg || exit; \
	done
