VERSION =  $(shell git describe --tags --exact-match 2>/dev/null || git branch  --show-current )
REVISION = $(shell git rev-parse HEAD)
BRANCH = $(shell git branch  --show-current)
COMPILE_TIME= $(shell date +"%Y-%m-%d %H:%M:%S")
USER = $(shell  git log -1 --pretty=format:"%an")
FLAGS = -ldflags "-s -w \
	-X 'github.com/prometheus/common/version.Version=${VERSION}' \
	-X 'github.com/prometheus/common/version.Revision=${REVISION}' \
	-X 'github.com/prometheus/common/version.Branch=${BRANCH}' \
	-X 'github.com/prometheus/common/version.BuildUser=${USER}' \
	-X 'github.com/prometheus/common/version.BuildDate=${COMPILE_TIME}'"


.PHONY: bpf
bpf:
	cd bpf

.PHONY: build
build:
	make bpf
	CGO_ENABLED=0 go build -tags netgo,osusergo ${FLAGS} -o bin/leaf main.go

.PHONY: clean
clean:
	rm -rf bin