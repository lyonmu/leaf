VERSION =  $(shell git describe --tags --exact-match 2>/dev/null || git branch  --show-current )
REVISION = $(shell git rev-parse HEAD)
BRANCH = $(shell git branch  --show-current)
COMPILE_TIME= $(shell date +"%Y-%m-%d %H:%M:%S")
USER = $(shell  git log -1 --pretty=format:"%an")
PROJECT_NAME = $(notdir $(CURDIR))
FLAGS = -ldflags "-s -w \
	-X 'github.com/prometheus/common/version.Version=${VERSION}' \
	-X 'github.com/prometheus/common/version.Revision=${REVISION}' \
	-X 'github.com/prometheus/common/version.Branch=${BRANCH}' \
	-X 'github.com/prometheus/common/version.BuildUser=${USER}' \
	-X 'github.com/prometheus/common/version.BuildDate=${COMPILE_TIME}'"

.PHONY: generate build clean

# 默认构建
default: build

# 生成 eBPF Go 绑定代码
ebpf:
	cd ebpf && go generate -tags linux ./...

# 构建 Go 程序（启用静态链接优化，不依赖 libc）
build: ebpf
	CGO_ENABLED=0 && go build  ${FLAGS} -tags="sonic avx netgo osusergo"  -o bin/${PROJECT_NAME} main.go


# 清理生成的文件
clean:
	rm -f bin/
	rm -f ebpf/counter/counter_bpfeb.go ebpf/counter/counter_bpfeb.o ebpf/counter/counter_bpfel.go ebpf/counter/counter_bpfel.o
	rm -f ebpf/flow/flow_bpfeb.go ebpf/flow/flow_bpfeb.o ebpf/flow/flow_bpfel.go ebpf/flow/flow_bpfel.o