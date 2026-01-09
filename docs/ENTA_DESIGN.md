# eBPF 系统探针 - 完整技术设计文档

## 文档信息

| 属性 | 值 |
|------|-----|
| 项目名称 | Leaf eBPF System Probe |
| 版本 | v2.0.0 |
| 创建日期 | 2026-01-09 |
| 状态 | 设计评审 |
| 文档作者 | Architecture Team |

---

## 目录

1. [项目概述](#1-项目概述)
2. [架构设计](#2-架构设计)
3. [详细设计](#3-详细设计)
4. [模块功能说明](#4-模块功能说明)
5. [开发步骤与里程碑](#5-开发步骤与里程碑)
6. [API 设计](#6-api-设计)
7. [数据模型](#7-数据模型)
8. [安全设计](#8-安全设计)
9. [性能优化](#9-性能优化)
10. [测试策略](#10-测试策略)
11. [部署方案](#11-部署方案)
12. [运维监控](#12-运维监控)
13. [附录](#13-附录)

---

## 1. 项目概述

### 1.1 背景

随着云原生架构的普及，系统可观测性在以下场景中变得至关重要：

- **性能监控**: 实时了解系统资源使用情况、识别性能瓶颈
- **安全审计**: 追踪进程行为、检测异常活动、数据泄露防护
- **故障排查**: 快速定位系统问题、分析性能退化根因
- **容量规划**: 收集长期趋势数据、优化资源分配

传统监控方案存在以下局限性：

1. 代理程序开销大 - 传统采集代理消耗显著系统资源
2. 数据时效性差 - 周期性采集导致数据延迟和丢失
3. 覆盖范围有限 - 难以深入内核层面获取完整信息
4. 扩展性不足 - 新增监控指标需要修改代理代码

### 1.2 项目目标

设计并实现基于 eBPF 的高性能系统探针，具备以下特性：

| 目标 | 指标 |
|------|------|
| 吞吐量 | 支持 100K+ events/second |
| 延迟 | 端到端延迟 < 10ms (P99) |
| 资源占用 | CPU < 5%, Memory < 200MB |
| 数据采集 | 网络/进程/性能指标全覆盖 |
| 输出能力 | Kafka 实时推送，100K+ messages/sec |
| 可扩展性 | 插件化架构，易于扩展新探针 |

### 1.3 技术选型

| 层级 | 技术 | 用途 | 理由 |
|------|------|------|------|
| 内核态 | C/eBPF | 内核探针程序 | 最高性能、零拷贝 |
| 用户态 | Go 1.24+ | 核心应用框架 | 高性能、并发友好、生态丰富 |
| eBPF框架 | github.com/cilium/ebpf v0.20+ | Go eBPF 库 | 官方推荐、类型安全、CO-RE支持 |
| eBPF编译 | bpf2go | 嵌入式编译 | 简化开发流程、零外部依赖 |
| 消息队列 | github.com/IBM/sarama | Kafka 客户端 | 高性能、功能完整 |
| 序列化 | github.com/bytedance/sonic | JSON 序列化 | 高性能、零拷贝 |
| 配置管理 | github.com/alecthomas/kong + yaml | CLI + 配置 | 结构化、类型安全 |
| 日志管理 | go.uber.org/zap | 结构化日志 | 高性能、低内存占用 |
| 日志轮转 | gopkg.in/natefinch/lumberjack.v2 | 日志文件轮转 | 稳定可靠 |

---

## 2. 架构设计

### 2.1 整体架构

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    eBPF 系统探针 - 整体架构                               │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│   ┌─────────────────────────────────────────────────────────────────┐   │
│   │                        基础设施层                                 │   │
│   │   ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐          │   │
│   │   │  Linux   │ │  eBPF    │ │  perf    │ │  trace   │          │   │
│   │   │  Kernel  │ │  Virtual │ │  events  │ │  points  │          │   │
│   │   │  5.10+   │ │  Machine │ │          │ │          │          │   │
│   │   │          │ │          │ │ RingBuf  │ │ Kprobes  │          │   │
│   │   └──────────┘ └──────────┘ └──────────┘ └──────────┘          │   │
│   └─────────────────────────────────────────────────────────────────┘   │
│                                 │                                       │
│   ┌─────────────────────────────▼───────────────────────────────────┐   │
│   │                    eBPF Probes Layer                             │   │
│   │                                                                   │   │
│   │   ┌───────────────────────────────────────────────────────────┐  │   │
│   │   │                   网络监控探针                             │  │   │
│   │   │  ┌────────────┐ ┌────────────┐ ┌────────────────────────┐ │  │   │
│   │   │  │Socket Hook │ │TCP Connect │ │TCP Close/State Change  │ │  │   │
│   │   │  │sock_send   │ │trace       │ │trace                   │ │  │   │
│   │   │  │recv        │ │            │ │                        │ │  │   │
│   │   │  └────────────┘ └────────────┘ └────────────────────────┘ │  │   │
│   │   └───────────────────────────────────────────────────────────┘  │   │
│   │                                                                   │   │
│   │   ┌───────────────────────────────────────────────────────────┐  │   │
│   │   │                   进程监控探针                             │  │   │
│   │   │  ┌────────────┐ ┌────────────┐ ┌────────────────────────┐ │  │   │
│   │   │  │Exec Trace  │ │Fork Trace  │ │Exit Trace              │ │  │   │
│   │   │  │sched_exec  │ │sched_fork  │ │sched_process_exit      │ │  │   │
│   │   │  └────────────┘ └────────────┘ └────────────────────────┘ │  │   │
│   │   └───────────────────────────────────────────────────────────┘  │   │
│   │                                                                   │   │
│   │   ┌───────────────────────────────────────────────────────────┐  │   │
│   │   │                   性能监控探针                             │  │   │
│   │   │  ┌────────────┐ ┌────────────┐ ┌────────────────────────┐ │  │   │
│   │   │  │CPU Sched   │ │IRQ Stats   │ │Softirq Stats           │ │  │   │
│   │   │  │scheduler   │ │handler     │ │handler                 │ │  │   │
│   │   │  └────────────┘ └────────────┘ └────────────────────────┘ │  │   │
│   │   └───────────────────────────────────────────────────────────┘  │   │
│   └─────────────────────────────────────────────────────────────────┘   │
│                              │                                           │
│   ┌──────────────────────────▼─────────────────────────────────────┐   │
│   │                    eBPF Maps (Kernel State)                     │   │
│   │                                                                   │   │
│   │   ┌───────────────────────────────────────────────────────────┐  │   │
│   │   │                   核心状态存储                             │  │   │
│   │   │  ┌────────────┐ ┌────────────┐ ┌────────────────────────┐ │  │   │
│   │   │  │proc_info   │ │net_conn    │ │events                  │ │  │   │
│   │   │  │Per-CPU     │ │LRU Hash    │ │RingBuf (256MB)         │ │  │   │
│   │   │  │64K slots   │ │100K entries│ │                        │ │  │   │
│   │   │  └────────────┘ └────────────┘ └────────────────────────┘ │  │   │
│   │   │  ┌────────────┐ ┌────────────┐ ┌────────────────────────┐ │  │   │
│   │   │  │stats_hist  │ │config      │ │rate_limit              │ │  │   │
│   │   │  │Per-CPU     │ │Read-Only   │ │Token Bucket            │ │  │   │
│   │   │  │Histogram   │ │            │ │                        │ │  │   │
│   │   │  └────────────┘ └────────────┘ └────────────────────────┘ │  │   │
│   │   └───────────────────────────────────────────────────────────┘  │   │
│   └─────────────────────────────────────────────────────────────────┘   │
│                              │                                           │
│   ┌──────────────────────────▼─────────────────────────────────────┐   │
│   │                    User Space Layer                             │   │
│   │                                                                   │   │
│   │   ┌───────────────────────────────────────────────────────────┐  │   │
│   │   │                   Core Controller                         │  │   │
│   │   │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐     │  │   │
│   │   │  │Loader    │ │Reconciler│ │State_mgr │ │RingReader│     │  │   │
│   │   │  │(cilium   │ │(Map同步) │ │(状态同步)│ │(事件读取)│     │  │   │
│   │   │  │ebpf)     │ │          │ │          │ │         │     │  │   │
│   │   │  └──────────┘ └──────────┘ └──────────┘ └──────────┘     │  │   │
│   │   └───────────────────────────────────────────────────────────┘  │   │
│   │                                                                   │   │
│   │   ┌───────────────────────────────────────────────────────────┐  │   │
│   │   │                   Event Processor                         │  │   │
│   │   │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐     │  │   │
│   │   │  │Network   │ │Process   │ │Perf      │ │Enricher  │     │  │   │
│   │   │  │Processor │ │Processor │ │Processor │ │(添加元数据)│     │  │   │
│   │   │  └──────────┘ └──────────┘ └──────────┘ └──────────┘     │  │   │
│   │   └───────────────────────────────────────────────────────────┘  │   │
│   │                                                                   │   │
│   │   ┌───────────────────────────────────────────────────────────┐  │   │
│   │   │                   Kafka Output Layer                      │  │   │
│   │   │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐     │  │   │
│   │   │  │Producer  │ │Batcher   │ │Serializer│ │Metrics   │     │  │   │
│   │   │  │(Sarama)  │ │(批量发送)│ │(Sonic)   │ │(监控)    │     │  │   │
│   │   │  └──────────┘ └──────────┘ └──────────┘ └──────────┘     │  │   │
│   │   └───────────────────────────────────────────────────────────┘  │   │
│   │                                                                   │   │
│   └─────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### 2.2 数据流设计

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          数据流全景图                                    │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  [内核事件产生]                                                          │
│       │                                                                 │
│       ▼                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    eBPF 探针层 (内核态)                           │
│  │                                                                  │   │
│  │  1. 网络探针 (tcp_connect, sock_send/recv, tcp_close)           │   │
│  │       │                                                          │   │
│  │       ▼                                                          │   │
│  │  2. 进程探针 (sched_process_exec, sched_process_fork,           │   │
│  │              sched_process_exit)                                 │   │
│  │       │                                                          │   │
│  │       ▼                                                          │   │
│  │  3. 性能探针 (scheduler, irq, softirq)                          │   │
│  │       │                                                          │   │
│  │       ▼                                                          │   │
│  │  4. 事件写入 RingBuf                                             │   │
│  │                                                                   │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│       │                                                                 │
│       │  [Ring Buffer]                                                │
│       ▼                                                                 │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    用户态处理                                     │
│  │                                                                  │   │
│  │  5. 事件读取 (epoll 等待)                                        │   │
│  │       │                                                          │   │
│  │       ▼                                                          │   │
│  │  6. 事件解码 (反序列化)                                          │   │
│  │       │                                                          │   │
│  │       ▼                                                          │   │
│  │  7. 事件分类 (网络/进程/性能)                                    │   │
│  │       │                                                          │   │
│  │       ▼                                                          │   │
│  │  8. 数据 enrichment (添加进程信息、容器ID等)                     │   │
│  │       │                                                          │   │
│  │       ▼                                                          │   │
│  │  9. 批量聚合                                                     │   │
│  │       │                                                          │   │
│  │       ▼                                                          │   │
│  │  10. Kafka 生产者发送 (Sarama)                                   │   │
│  │       │                                                          │   │
│  │       ▼                                                          │   │
│  │  11. 监控指标上报 (Prometheus)                                   │   │
│  │                                                                   │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### 2.3 核心组件说明

#### 2.3.1 eBPF Probes Layer

**职责**: 在内核态完成高性能事件采集

| 探针类型 | 挂载点 | 用途 | 性能影响 |
|---------|-------|------|---------|
| 网络 | tcp_v4_connect | TCP 连接建立追踪 | 低 |
| 网络 | inet_sendmsg/recvmsg | Socket 数据传输追踪 | 低 |
| 网络 | tcp_set_state | TCP 连接状态追踪 | 低 |
| 网络 | tcp_close | TCP 连接关闭追踪 | 低 |
| 进程 | sched_process_exec | 进程执行事件追踪 | 低 |
| 进程 | sched_process_fork | 进程 fork 事件追踪 | 低 |
| 进程 | sched_process_exit | 进程退出事件追踪 | 低 |
| 性能 | sched_switch | 上下文切换统计 | 中 |
| 性能 | irq_handler_entry | 硬中断统计 | 低 |
| 性能 | softirq_entry | 软中断统计 | 低 |

####.2 eBP 2.3F Maps

**核心数据结构**:

- `events`: RingBuf, 256MB, 事件传递
- `proc_info`: Per-CPU Array, 64K slots, 进程统计
- `net_conn`: LRU Hash, 100K entries, 连接追踪
- `config`: Array, 只读配置
- `stats_hist`: Per-CPU Hash, 直方图统计
- `rate_limit`: Array, 令牌桶限流

#### 2.3.3 用户态 Core Controller

**职责**:

- 使用 github.com/cilium/ebpf 加载 eBPF 程序
- Ring Buffer 事件读取
- Map 状态同步
- 资源管理
- Kafka 批量发送

---

## 3. 详细设计

### 3.1 文件结构

```
leaf/
├── bpf/                          # eBPF 内核代码 (C) - 待开发
│   ├── programs/
│   │   ├── network.c             # 网络探针程序
│   │   ├── process.c             # 进程探针程序
│   │   ├── performance.c         # 性能探针程序
│   │   └── common.c              # 公共辅助函数
│   ├── maps/
│   │   ├── events.h              # 事件结构定义
│   │   ├── proc_stats.h          # 进程统计 Map
│   │   ├── net_conn.h            # 网络连接 Map
│   │   └── config.h              # 配置结构
│   └── headers/
│       └── common.h              # 公共类型定义
│
├── main.go                       # 入口 (已有)
│
├── internal/                     # 内部包
│   ├── bpf/                      # eBPF 加载器 - 待开发
│   │   ├── loader.go             # eBPF 加载器 (cilium/ebpf)
│   │   ├── maps.go               # Map 访问封装
│   │   ├── programs.go           # Program 管理
│   │   └── ringbuf.go            # Ring Buffer 读取
│   ├── bootstrap/
│   │   └── bootstrap.go          # 启动逻辑 (已有)
│   ├── collector/                # 事件收集器 - 待开发
│   │   ├── network.go            # 网络事件收集
│   │   ├── process.go            # 进程事件收集
│   │   └── performance.go        # 性能事件收集
│   ├── enricher/                 # 数据 enrichment - 待开发
│   │   └── enricher.go           # 数据 enrichment
│   └── config/
│       └── config.go             # 配置加载 (已有)
│
├── pkg/                          # 公共包
│   ├── models/                   # 数据模型 - 待开发
│   │   ├── network.go            # 网络事件模型
│   │   ├── process.go            # 进程事件模型
│   │   ├── performance.go        # 性能事件模型
│   │   └── common.go             # 公共类型
│   ├── mq/                       # 消息队列 (已有)
│   │   ├── kafka/
│   │   │   ├── producer.go       # Kafka 生产者 (已有)
│   │   │   ├── subscriber.go     # Kafka 订阅者 (已有)
│   │   │   └── options.go        # 配置选项 (已有)
│   │   ├── serializer/           # 序列化器 (已有)
│   │   │   ├── json.go           # JSON 序列化 (已有)
│   │   │   ├── protobuf.go       # Protobuf 序列化 (已有)
│   │   │   ├── binary.go         # Binary 序列化 (已有)
│   │   │   └── string.go         # String 序列化 (已有)
│   │   └── mq.go                 # 接口定义 (已有)
│   ├── config/
│   │   └── base.go               # 基础配置 (已有)
│   ├── logger/
│   │   └── logger.go             # 日志封装 (已有)
│   └── tools/
│       └── id.go                 # ID 生成器 (已有)
│
├── test/                         # 测试 - 待开发
│   ├── e2e/                      # 端到端测试
│   └── integration/              # 集成测试
│
├── go.mod                        # 依赖管理 (已有)
├── go.sum                        # 依赖锁定 (已有)
├── Makefile                      # 构建脚本 (已有)
└── README.md                     # 项目说明 (已有)
```

### 3.2 现有代码分析

#### 3.2.1 已有的核心组件

**1. Kafka 生产者 (pkg/mq/kafka/producer.go)**

```go
// Producer 泛型生产者实现 - 已有的高性能实现
type Producer[K, V any] struct {
    producer   sarama.AsyncProducer
    keyCodec   mq.KeySerializer[K]
    valueCodec mq.ValueSerializer[V]
    topic      string
    closeCh    chan struct{}
    wg         sync.WaitGroup
}

// 特性：
// - 异步回执，高吞吐
// - 批量聚合 (10ms 频率或 16KB 阈值)
// - 支持多种压缩算法 (ZSTD, LZ4, Snappy)
// - SASL 认证支持
// - 泛型 Key/Value 支持
```

**2. JSON 序列化器 (pkg/mq/serializer/json.go)**

```go
// 基于 bytedance/sonic 的高性能 JSON 序列化
// 特性：
// - 零拷贝序列化
// - 对象池复用
// - 内存优化
```

**3. 配置管理 (pkg/config/base.go)**

```go
// KafkaConfig 已有的配置结构
type KafkaConfig struct {
    Brokers     []string
    SASL        KafkaSASLConfig
    Partitions  int
    Replication int
}
```

**4. 日志系统 (pkg/logger/logger.go)**

```go
// 基于 Zap 的结构化日志
// 特性：
// - 日志轮转 (lumberjack)
// - 多输出 (文件 + 控制台)
// - 模块名支持
```

#### 3.2.2 待开发的核心组件

**1. eBPF 加载器**

```go
package bpf

import (
    "github.com/cilium/ebpf"
    "github.com/cilium/ebpf/link"
    "github.com/cilium/ebpf/rlimit"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang -cflags="-O2 -Wall" bpf ./bpf/programs/...

type Loader struct {
    objs   *bpfObjects
    links  []link.Link
    ringBuf *ebpf.RingBuffer
}

// 需要实现：
// - Load() 加载 eBPF 程序
// - attachNetworkProbes() 附加网络探针
// - attachProcessProbes() 附加进程探针
// - attachPerformanceProbes() 附加性能探针
// - startRingBufferReader() 启动事件读取

```

**2. 事件收集器**

```go
package collector

type EventProcessor struct {
    networkHandler  func(*NetworkEvent)
    processHandler  func(*ProcessEvent)
    performanceHandler func(*PerformanceEvent)
}

// 需要实现：
// - HandleEvent() 处理 RingBuf 事件
// - Dispatch() 分发到对应处理器
// - Enrich() 数据 enrichment

```

---

## 4. 模块功能说明

### 4.1 eBPF 内核探针层

#### 4.1.1 网络探针 (bpf/programs/network.c)

**功能**:
- 捕获 TCP 连接建立事件 (tcp_v4_connect)
- 捕获 TCP 连接关闭事件 (tcp_close)
- 追踪 Socket 数据发送 (inet_sendmsg)
- 维护连接状态追踪表

**数据结构**:

```c
// 连接键
struct conn_key_t {
    __u8  family;      // AF_INET / AF_INET6
    __u8  protocol;    // IPPROTO_TCP
    __u32 saddr[4];    // 源 IP
    __u32 daddr[4];    // 目的 IP
    __u16 sport;       // 源端口
    __u16 dport;       // 目的端口
};

// 连接值
struct conn_value_t {
    __u64 created_at;      // 创建时间
    __u64 bytes_sent;      // 发送字节数
    __u64 bytes_recv;      // 接收字节数
    __u64 packets_sent;    // 发送包数
    __u64 packets_recv;    // 接收包数
};

// 网络事件
struct net_event_t {
    struct conn_key_t key;
    __u32 pid;
    char comm[16];
    char state[16];
    __u64 bytes_sent;
    __u64 bytes_recv;
    __u64 duration_ns;
};
```

#### 4.1.2 进程探针 (bpf/programs/process.c)

**功能**:
- 捕获进程执行事件 (sched_process_exec)
- 捕获进程 fork 事件 (sched_process_fork)
- 捕获进程退出事件 (sched_process_exit)
- 收集进程资源使用信息

**数据结构**:

```c
// 进程事件
struct process_event_t {
    __u64 timestamp;
    __u32 pid;            // 进程 ID
    __u32 tgid;           // 线程组 ID
    __u32 ppid;           // 父进程 ID
    __u32 uid;            // 用户 ID
    char comm[16];        // 进程名
    char filename[256];   // 可执行文件路径
    char cmdline[512];    // 命令行参数
    __int32 exit_code;    // 退出码
    __u64 utime;          // 用户态时间
    __u64 stime;          // 内核态时间
    __u64 min_flt;        // 次缺页次数
    __u64 maj_flt;        // 主缺页次数
};
```

#### 4.1.3 性能探针 (bpf/programs/performance.c)

**功能**:
- 捕获上下文切换事件 (sched_switch)
- 捕获硬中断进入事件 (irq_handler_entry)
- 捕获软中断进入事件 (softirq_entry)
- Per-CPU 统计聚合

**数据结构**:

```c
// 性能事件
struct perf_event_t {
    __u64 timestamp;
    char perf_type[16];   // 事件类型
    __u32 irq;            // 中断号
    char irq_name[32];    // 中断处理函数名
    __u32 softirq_vec;    // 软中断向量
    char softirq_name[8]; // 软中断名称
    __u32 prev_pid;       // 切换前进程
    __u32 next_pid;       // 切换后进程
    char comm[16];        // 进程名
};

// Per-CPU 统计
struct perf_stats_t {
    __u64 context_switches;
    __u64 preemptions;
    __u64 irq_count;
    __u64 softirq_count;
};
```

### 4.2 eBPF 加载器模块 (internal/bpf/)

#### 4.2.1 loader.go - 主加载器

**功能**:
- 加载 eBPF 程序和 Maps
- 附加探针到内核挂载点
- 管理探针生命周期
- 启动 RingBuffer 事件读取

**接口设计**:

```go
type Loader struct {
    objs      *bpfObjects    // bpf2go 生成的类型
    links     []link.Link    // 探针链接
    ringBuf   *ebpf.RingBuffer
    callbacks []EventCallback // 事件回调
}

type EventCallback func(eventType EventType, data []byte)

// NewLoader 创建新的加载器
func NewLoader(cfg *config.Config) *Loader

// Load 加载所有 eBPF 程序
func (l *Loader) Load() error

// Close 关闭所有资源
func (l *Loader) Close() error

// RegisterCallback 注册事件回调
func (l *Loader) RegisterCallback(cb EventCallback)

// IsLoaded 检查是否已加载
func (l *Loader) IsLoaded() bool
```

#### 4.2.2 maps.go - Map 访问封装

**功能**:
- 封装 eBPF Map 操作
- 提供类型安全的 Map 访问
- 支持批量读写

**接口设计**:

```go
type MapManager struct {
    loader *Loader
}

// NewMapManager 创建 Map 管理器
func NewMapManager(loader *Loader) *MapManager

// GetNetConnMap 获取网络连接 Map
func (m *MapManager) GetNetConnMap() (*ebpf.Map, error)

// GetStatsMap 获取统计 Map
func (m *MapManager) GetStatsMap() (*ebpf.Map, error)

// UpdateConfig 更新配置 Map
func (m *MapManager) UpdateConfig(cfg *ProbeConfig) error

// IterateConnections 遍历连接表
func (m *MapManager) IterateConnections(cb func(key *ConnKey, val *ConnValue) bool)
```

#### 4.2.3 ringbuf.go - RingBuffer 读取

**功能**:
- 高性能 RingBuffer 事件读取
- 事件分片和重组
- 错误处理和重试

**接口设计**:

```go
type RingBufferReader struct {
    rb     *ebpf.RingBuffer
    events chan []byte
    wg     sync.WaitGroup
}

type RingBufferConfig struct {
    NumPages   int           // 页面数 (默认 64)
    PollTimeout time.Duration // 轮询超时
    BufferSize int           // 用户态缓冲区大小
}

// NewRingBufferReader 创建读取器
func NewRingBufferReader(m *ebpf.Map, cfg RingBufferConfig) (*RingBufferReader, error)

// Start 启动读取循环
func (r *RingBufferReader) Start()

// Stop 停止读取
func (r *RingBufferReader) Stop()

// Events 返回事件通道
func (r *RingBufferReader) Events() <-chan []byte
```

### 4.3 事件收集器模块 (internal/collector/)

#### 4.3.1 collector.go - 主收集器

**功能**:
- 接收 RingBuffer 事件
- 事件解码和分类
- 事件聚合和批处理
- 调用 Kafka 发送

**接口设计**:

```go
type Collector struct {
    ringReader *bpf.RingBufferReader
    enricher   *Enricher
    producers  map[EventType]*kafka.Producer[string, []byte]
    batchCh    chan *BatchEvent
    wg         sync.WaitGroup
}

type BatchEvent struct {
    Events [][]byte
    Type   EventType
}

// NewCollector 创建收集器
func NewCollector(loader *bpf.Loader, cfg *config.Config) (*Collector, error)

// Start 启动收集
func (c *Collector) Start()

// Stop 停止收集
func (c *Stop) Stop()

// RegisterProducer 为事件类型注册生产者
func (c *Collector) RegisterProducer(eventType EventType, topic string)
```

#### 4.3.2 network.go - 网络事件处理

**功能**:
- 解析网络事件数据
- 验证连接信息
- 关联进程信息

**接口设计**:

```go
type NetworkProcessor struct {
    enricher *Enricher
}

type NetworkEvent struct {
    BaseEvent
    SrcIP     string
    DstIP     string
    SrcPort   uint16
    DstPort   uint16
    Protocol  uint8
    State     string
    BytesSent uint64
    BytesRecv uint64
}

// Process 处理网络事件
func (p *NetworkProcessor) Process(data []byte) (*models.EnrichedEvent, error)
```

#### 4.3.3 process.go - 进程事件处理

**功能**:
- 解析进程事件数据
- 提取执行参数
- 关联用户信息

**接口设计**:

```go
type ProcessProcessor struct {
    enricher *Enricher
}

type ProcessEvent struct {
    BaseEvent
    PID       uint32
    TGID      uint32
    PPID      uint32
    Filename  string
    Cmdline   string
    ExitCode  int32
    UTime     uint64
    STime     uint64
}

// Process 处理进程事件
func (p *ProcessProcessor) Process(data []byte) (*models.EnrichedEvent, error)
```

#### 4.3.4 performance.go - 性能事件处理

**功能**:
- 解析性能事件数据
- 聚合统计数据
- 生成性能指标

**接口设计**:

```go
type PerformanceProcessor struct {
    enricher *Enricher
    stats    *PerformanceStats
}

type PerformanceEvent struct {
    BaseEvent
    Type        string
    IRQ         uint32
    IRQName     string
    SoftirqVec  uint32
    SoftirqName string
    PrevPID     uint32
    NextPID     uint32
}

// Process 处理性能事件
func (p *PerformanceProcessor) Process(data []byte) (*models.EnrichedEvent, error)

// GetStats 获取聚合统计
func (p *PerformanceProcessor) GetStats() *PerformanceStats
```

### 4.4 数据 enrichment 模块 (internal/enricher/)

**功能**:
- 补充容器元数据
- 添加 cgroup 信息
- 丰富进程信息
- 添加时间戳和主机信息

**接口设计**:

```go
type Enricher struct {
    hostname  string
    container ContainerInfoProvider
}

type ContainerInfoProvider interface {
    GetContainerID(pid uint32) string
    GetCgroupPath(pid uint32) string
}

// NewEnricher 创建 enrichment 器
func NewEnricher() *Enricher

// Enrich 丰富事件数据
func (e *Enricher) Enrich(event *models.EnrichedEvent) error

// AddMetadata 添加元数据
func (e *Enricher) AddMetadata(event *models.EnrichedEvent, metadata map[string]string)
```

### 4.5 数据模型模块 (pkg/models/)

**功能**:
- 定义事件数据结构
- 提供序列化和反序列化
- 定义常量枚举

**内容**:

```go
// EventType 事件类型枚举
type EventType string

const (
    EventTypeTCPConnect      EventType = "tcp_connect"
    EventTypeTCPClose        EventType = "tcp_close"
    EventTypeProcessExec     EventType = "process_exec"
    EventTypeProcessFork     EventType = "process_fork"
    EventTypeProcessExit     EventType = "process_exit"
    EventTypePerfContextSwitch EventType = "context_switch"
    EventTypePerfIRQ         EventType = "irq"
    EventTypePerfSoftIRQ     EventType = "softirq"
)

// BaseEvent 基础事件
type BaseEvent struct {
    Type      EventType   `json:"type"`
    Timestamp int64       `json:"timestamp"`
    Hostname  string      `json:"hostname"`
    PID       uint32      `json:"pid"`
    TGID      uint32      `json:"tgid"`
    Comm      string      `json:"comm"`
}

// EnrichedEvent 增强事件
type EnrichedEvent struct {
    NetworkEvent     *NetworkEvent     `json:"network,omitempty"`
    ProcessEvent     *ProcessEvent     `json:"process,omitempty"`
    PerformanceEvent *PerformanceEvent `json:"performance,omitempty"`
    ContainerID      string            `json:"container_id,omitempty"`
    CgroupPath       string            `json:"cgroup_path,omitempty"`
    Metadata         map[string]string `json:"metadata,omitempty"`
}

// 序列化方法
func (e *EnrichedEvent) Serialize() ([]byte, error)
func (e *EnrichedEvent) GetKey() string
```

---

## 5. 开发步骤与里程碑

### 5.1 模块开发顺序

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          模块依赖关系                                    │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌──────────────┐                                                       │
│  │  数据模型    │  ◄─── 第一步 (基础依赖)                                │
│  │ (pkg/models) │                                                       │
│  └──────┬───────┘                                                       │
│         │                                                               │
│         ▼                                                               │
│  ┌──────────────┐                                                       │
│  │  eBPF 探针   │  ◄─── 第二步 (内核层)                                  │
│  │ (bpf/programs)│                                                       │
│  └──────┬───────┘                                                       │
│         │                                                               │
│         ▼                                                               │
│  ┌──────────────┐     ┌──────────────┐                                  │
│  │ eBPF 加载器  │ ──► │ 数据 enrichment│ ◄── 第三步                      │
│  │(internal/bpf)│     │(internal/enricher)│                              │
│  └──────┬───────┘     └──────────────┘                                  │
│         │                                                               │
│         ▼                                                               │
│  ┌──────────────┐                                                       │
│  │ 事件收集器   │  ◄─── 第四步                                           │
│  │(internal/collector)│                                                 │
│  └──────┬───────┘                                                       │
│         │                                                               │
│         ▼                                                               │
│  ┌──────────────┐                                                       │
│  │ 集成测试     │  ◄─── 第五步                                           │
│  │  (test/)     │                                                       │
│  └──────────────┘                                                       │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### 5.2 详细开发步骤

#### 阶段一：数据模型定义 (Day 1-2)

**任务**:
1. 创建 `pkg/models/` 目录
2. 定义事件类型枚举 `EventType`
3. 定义基础事件结构 `BaseEvent`
4. 定义网络事件结构 `NetworkEvent`
5. 定义进程事件结构 `ProcessEvent`
6. 定义性能事件结构 `PerformanceEvent`
7. 定义增强事件结构 `EnrichedEvent`
8. 实现序列化方法 `Serialize()`
9. 实现键生成方法 `GetKey()`
10. 编写单元测试

**交付物**:
- `pkg/models/common.go` - 常量和类型定义
- `pkg/models/network.go` - 网络事件模型
- `pkg/models/process.go` - 进程事件模型
- `pkg/models/performance.go` - 性能事件模型
- `pkg/models/*_test.go` - 单元测试

**代码示例**:

```go
// pkg/models/network.go
package models

// NetworkEvent 网络事件
type NetworkEvent struct {
    BaseEvent
    Family      uint8   `json:"family"`
    Protocol    uint8   `json:"protocol"`
    SrcIP       string  `json:"src_ip"`
    DstIP       string  `json:"dst_ip"`
    SrcPort     uint16  `json:"src_port"`
    DstPort     uint16  `json:"dst_port"`
    State       string  `json:"state"`
    BytesSent   uint64  `json:"bytes_sent"`
    BytesRecv   uint64  `json:"bytes_recv"`
    DurationNs  uint64  `json:"duration_ns"`
}
```

#### 阶段二：eBPF 探针开发 (Day 3-7)

**任务**:
1. 创建 `bpf/headers/` 目录，添加 `common.h`
2. 创建 `bpf/maps/` 目录，添加 Map 定义头文件
3. 开发 `bpf/programs/network.c`:
   - 实现 `send_event()` 辅助函数
   - 实现 `tcp_v4_connect` 探针 (fentry)
   - 实现 `tcp_close` 探针 (fentry)
   - 实现 `inet_sendmsg` 探针 (kprobe)
4. 开发 `bpf/programs/process.c`:
   - 实现 `sched_process_exec` 探针 (tracepoint)
   - 实现 `sched_process_fork` 探针 (tracepoint)
   - 实现 `sched_process_exit` 探针 (tracepoint)
5. 开发 `bpf/programs/performance.c`:
   - 实现 `sched_switch` 探针 (tracepoint)
   - 实现 `irq_handler_entry` 探针 (tracepoint)
   - 实现 `softirq_entry` 探针 (tracepoint)
6. 更新 `Makefile` 添加 eBPF 编译目标
7. 测试编译

**交付物**:
- `bpf/headers/common.h` - 公共头文件
- `bpf/maps/events.h` - 事件结构定义
- `bpf/maps/net_conn.h` - 连接追踪 Map
- `bpf/maps/perf_stats.h` - 性能统计 Map
- `bpf/programs/network.c` - 网络探针
- `bpf/programs/process.c` - 进程探针
- `bpf/programs/performance.c` - 性能探针
- 编译通过的 `.o` 文件

**代码示例**:

```c
// bpf/programs/network.c - TCP 连接建立探针
SEC("fentry/tcp_v4_connect")
int BPF_PROG(tcp_v4_connect, struct sock *sk) {
    struct net_event_t evt = {};
    struct conn_key_t key = {};
    
    if (sk->sk_protocol != IPPROTO_TCP) {
        return 0;
    }
    
    key.family = AF_INET;
    key.sport = sk->__sk_common.skc_num;
    key.dport = bpf_ntohs(sk->__sk_common.skc_dport);
    key.saddr[0] = sk->__sk_common.skc_rcv_saddr;
    key.daddr[0] = sk->__sk_common.skc_daddr;
    
    evt.key = key;
    evt.pid = bpf_get_current_pid_tgid() >> 32;
    bpf_get_current_comm(evt.comm, sizeof(evt.comm));
    evt.state = TCP_SYN_SENT;
    
    return send_event(ctx, EVENT_TYPE_TCP_CONNECT, &evt, sizeof(evt));
}
```

#### 阶段三：eBPF 加载器开发 (Day 8-12)

**任务**:
1. 创建 `internal/bpf/` 目录
2. 开发 `loader.go`:
   - 定义 `bpfObjects` 结构
   - 实现 `NewLoader()` 构造函数
   - 实现 `Load()` 方法
   - 实现探针附加方法
   - 实现资源清理
3. 开发 `maps.go`:
   - 定义 `MapManager` 结构
   - 实现 Map 访问封装
   - 实现批量操作
4. 开发 `ringbuf.go`:
   - 定义 `RingBufferReader` 结构
   - 实现事件读取循环
   - 实现错误处理
5. 集成 `bpf2go` 编译
6. 编写单元测试

**交付物**:
- `internal/bpf/loader.go` - 主加载器
- `internal/bpf/maps.go` - Map 管理
- `internal/bpf/ringbuf.go` - RingBuffer 读取
- `internal/bpf/bpf_bpfeb.go` - bpf2go 生成
- `internal/bpf/*_test.go` - 单元测试

**代码示例**:

```go
// internal/bpf/loader.go - 加载器主逻辑
func (l *Loader) Load() error {
    // 1. 移除内存锁限制
    if err := rlimit.RemoveMemlock(); err != nil {
        log.Printf("警告: 移除 memlock 限制失败: %v", err)
    }
    
    // 2. 加载 eBPF 程序
    l.objs = &bpfObjects{}
    if err := loadBpfObjects(l.objs, nil); err != nil {
        return fmt.Errorf("加载 eBPF 对象失败: %w", err)
    }
    
    // 3. 附加探针
    if err := l.attachNetworkProbes(); err != nil {
        return fmt.Errorf("附加网络探针失败: %w", err)
    }
    if err := l.attachProcessProbes(); err != nil {
        return fmt.Errorf("附加进程探针失败: %w", err)
    }
    if err := l.attachPerformanceProbes(); err != nil {
        return fmt.Errorf("附加性能探针失败: %w", err)
    }
    
    // 4. 启动 RingBuffer
    if err := l.startRingBufferReader(); err != nil {
        return fmt.Errorf("启动 RingBuffer 失败: %w", err)
    }
    
    log.Println("eBPF 探针加载成功")
    return nil
}
```

#### 阶段四：事件收集器开发 (Day 13-17)

**任务**:
1. 创建 `internal/collector/` 目录
2. 开发 `collector.go`:
   - 定义 `Collector` 结构
   - 实现事件接收和分发
   - 实现 Kafka 集成
3. 开发 `network.go`:
   - 定义 `NetworkProcessor` 结构
   - 实现网络事件解析
   - 实现事件验证
4. 开发 `process.go`:
   - 定义 `ProcessProcessor` 结构
   - 实现进程事件解析
5. 开发 `performance.go`:
   - 定义 `PerformanceProcessor` 结构
   - 实现性能事件解析
   - 实现统计聚合
6. 复用已有的 Kafka 生产者
7. 编写单元测试

**交付物**:
- `internal/collector/collector.go` - 主收集器
- `internal/collector/network.go` - 网络事件处理
- `internal/collector/process.go` - 进程事件处理
- `internal/collector/performance.go` - 性能事件处理
- `internal/collector/*_test.go` - 单元测试

**代码示例**:

```go
// internal/collector/collector.go - 主收集器
type Collector struct {
    ringReader *bpf.RingBufferReader
    enricher   *Enricher
    producers  map[EventType]*kafka.Producer[string, []byte]
    batchCh    chan *BatchEvent
    wg         sync.WaitGroup
}

func (c *Collector) Start() {
    c.wg.Add(1)
    go c.batchProcessor()
    
    c.ringReader.Start()
    for data := range c.ringReader.Events() {
        c.dispatch(data)
    }
}

func (c *Collector) dispatch(data []byte) {
    // 解析事件头获取类型
    var header EventHeader
    if err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &header); err != nil {
        return
    }
    
    // 分发到对应处理器
    switch header.Type {
    case EventTypeTCPConnect, EventTypeTCPClose:
        c.processNetworkEvent(header.Type, data)
    case EventTypeProcessExec, EventTypeProcessFork, EventTypeProcessExit:
        c.processProcessEvent(header.Type, data)
    case EventTypePerfContextSwitch, EventTypePerfIRQ, EventTypePerfSoftIRQ:
        c.processPerformanceEvent(header.Type, data)
    }
}
```

#### 阶段五：数据 enrichment 开发 (Day 18-20)

**任务**:
1. 创建 `internal/enricher/` 目录
2. 开发 `enricher.go`:
   - 定义 `Enricher` 结构
   - 实现容器信息获取
   - 实现 cgroup 信息读取
   - 实现元数据补充
3. 集成 `/proc` 文件系统读取
4. 支持容器环境检测
5. 编写单元测试

**交付物**:
- `internal/enricher/enricher.go` - 数据 enrichment
- `internal/enricher/container.go` - 容器信息
- `internal/enricher/*_test.go` - 单元测试

**代码示例**:

```go
// internal/enricher/enricher.go
type Enricher struct {
    hostname string
}

func (e *Enricher) Enrich(event *models.EnrichedEvent) error {
    // 添加主机名
    event.Hostname = e.hostname
    
    // 尝试获取容器信息
    if containerID := e.getContainerID(event.PID); containerID != "" {
        event.ContainerID = containerID
    }
    
    // 尝试获取 cgroup 信息
    if cgroupPath := e.getCgroupPath(event.PID); cgroupPath != "" {
        event.CgroupPath = cgroupPath
    }
    
    return nil
}

func (e *Enricher) getContainerID(pid uint32) string {
    // 读取 /proc/<pid>/cgroup 获取容器 ID
    path := fmt.Sprintf("/proc/%d/cgroup", pid)
    data, err := os.ReadFile(path)
    if err != nil {
        return ""
    }
    // 解析 cgroup 路径提取容器 ID
    // ...
    return ""
}
```

#### 阶段六：集成测试 (Day 21-24)

**任务**:
1. 创建 `test/` 目录
2. 开发集成测试:
   - eBPF 探针加载测试
   - 事件采集测试
   - Kafka 发送测试
   - 端到端测试
3. 性能测试:
   - 吞吐量测试
   - 延迟测试
   - 资源占用测试
4. 编写测试报告

**交付物**:
- `test/integration/` - 集成测试
- `test/e2e/` - 端到端测试
- `test/performance/` - 性能测试
- `TEST_REPORT.md` - 测试报告

### 5.3 里程碑时间表

| 阶段 | 日期 | 里程碑 | 交付物 |
|------|------|--------|--------|
| 阶段一 | Day 1-2 | 数据模型定义完成 | pkg/models/ |
| 阶段二 | Day 3-7 | eBPF 探针开发完成 | bpf/programs/*.c |
| 阶段三 | Day 8-12 | eBPF 加载器完成 | internal/bpf/ |
| 阶段四 | Day 13-17 | 事件收集器完成 | internal/collector/ |
| 阶段五 | Day 18-20 | 数据 enrichment 完成 | internal/enricher/ |
| 阶段六 | Day 21-24 | 集成测试完成 | test/ |
| **总计** | **24 天** | **生产就绪** | **完整系统** |

### 5.4 每日任务检查清单

```markdown
## 每日开发检查清单

### 编码规范
- [ ] 代码遵循项目风格指南
- [ ] 添加必要的注释
- [ ] 函数长度 < 50 行
- [ ] 变量命名清晰

### 测试
- [ ] 编写单元测试
- [ ] 测试覆盖率 > 80%
- [ ] 运行现有测试

### 文档
- [ ] 更新接口文档
- [ ] 记录设计决策
- [ ] 更新 ENTA_DESIGN.md

### 代码审查
- [ ] 自查代码
- [ ] 提交 PR (如有需要)
```

---

## 6. API 设计

### 6.1 Kafka Topic 设计

| Topic | 用途 | 分区策略 | 保留时间 |
|-------|------|---------|---------|
| system.events.network | 网络事件 | 按源IP哈希 | 7天 |
| system.events.process | 进程事件 | 按PID哈希 | 7天 |
| system.events.performance | 性能事件 | 按主机名 | 7天 |
| system.metrics | 聚合指标 | 按指标类型 | 30天 |

### 6.2 消息格式

```json
{
  "type": "network",
  "timestamp": 1704806400000000000,
  "hostname": "prod-web-01",
  "data": {
    "type": "tcp_connect",
    "pid": 12345,
    "comm": "curl",
    "src_ip": "10.0.0.1",
    "src_port": 54321,
    "dst_ip": "93.184.216.34",
    "dst_port": 443,
    "container_id": "abc123def456"
  }
}
```

---

## 7. 数据模型

### 7.1 网络连接模型

```go
type ConnectionKey struct {
    Family   uint8    // AF_INET or AF_INET6
    Protocol uint8    // IPPROTO_TCP, IPPROTO_UDP
    SrcIP    net.IP
    DstIP    net.IP
    SrcPort  uint16
    DstPort  uint16
}

type Connection struct {
    Key           ConnectionKey
    State         ConnectionState
    CreatedAt     time.Time
    BytesSent     uint64
    BytesRecv     uint64
    PacketsSent   uint64
    PacketsRecv   uint64
    PID           uint32
    Comm          string
}
```

### 7.2 进程模型

```go
type ProcessInfo struct {
    PID        uint32
    TGID       uint32
    PPID       uint32
    Comm       string
    Filename   string
    Cmdline    string
    State      ProcessState
    UTime      uint64
    STime      uint64
    MinFlt     uint64
    MajFlt     uint64
    StartTime  time.Time
}
```

---

## 8. 安全设计

### 8.1 权限要求

```yaml
security:
  capabilities:
    - CAP_SYS_ADMIN       # 加载 eBPF 程序
    - CAP_BPF             # 加载 eBPF 程序 (内核 5.8+)
    - CAP_PERFMON         # 使用 perf 事件
    - CAP_NET_ADMIN       # 网络相关探针 (可选)

  capabilities_dROP:
    - CAP_SETPCAP
    - CAP_SETFCAP

  namespaces:
    network_namespace: "host"
```

### 8.2 数据脱敏

```go
type SecurityConfig struct {
    Redaction struct {
        Enabled   bool
        Headers   []string  // authorization, cookie 等
        Cmdline   bool      // 是否脱敏命令行参数
    }
    Filter struct {
        ExcludeUIDs    []uint32
        ExcludeComm    []string
        MaxCmdlineLen  int
    }
}
```

---

## 9. 性能优化

### 9.1 eBPF 优化策略

1. **使用 fentry 替代 kprobe** - 更优性能，内核 5.5+
2. **使用 RingBuf 替代 Perf Buffer** - 更高性能、更低开销
3. **使用 Per-CPU Maps** - 避免锁竞争
4. **批量操作** - bpf_map_batch_update
5. **避免循环** - 使用有限循环或展开

### 9.2 用户态优化

```go
type PerformanceConfig struct {
    Batch struct {
        Size    int           // 批量大小: 100-1000
        Timeout time.Duration // 超时: 10-100ms
    }
    Concurrency struct {
        EventWorkers   int // 事件处理 worker 数
        EnrichWorkers  int // enrichment worker 数
    }
    Kafka struct {
        BatchSize     int
        BatchTimeout  time.Duration
        MaxAttempts   int
    }
    Memory struct {
        MaxMemory   int64
        GCPercent   int
    }
}
```

### 9.3 性能目标

| 指标 | 目标值 | 测量方法 |
|------|--------|---------|
| 吞吐量 | 100K events/sec | 单节点满负载 |
| Kafka 吞吐 | 100K msgs/sec | 批量发送测试 |
| 延迟 | < 10ms P99 | 端到端延迟 |
| CPU | < 5% 单核 | 高负载测试 |
| 内存 | < 200MB | 持续运行 |
| 事件丢失 | < 0.01% | 高负载测试 |

---

## 10. 测试策略

### 10.1 测试金字塔

```
┌────────────────────┐   E2E 测试  (5%)
├────────────────────┤
│   集成测试 (25%)    │
├────────────────────┤
│   单元测试 (70%)    │
└────────────────────┘
```

### 10.2 测试类型

1. **单元测试** - eBPF 函数、数据处理
2. **集成测试** - 组件交互、Kafka 集成
3. **E2E 测试** - 完整数据采集场景
4. **性能测试** - 吞吐量、延迟测试
5. **模糊测试** - 协议解析健壮性

---

## 11. 部署方案

### 11.1 Kubernetes DaemonSet

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: leaf
  namespace: system-observability
spec:
  template:
    spec:
      hostNetwork: true
      hostPID: true
      containers:
        - name: leaf
          image: leaf:latest
          securityContext:
            privileged: true
            capabilities:
              add:
                - SYS_ADMIN
                - BPF
                - PERFMON
          resources:
            requests:
              memory: "256Mi"
              cpu: "250m"
            limits:
              memory: "512Mi"
              cpu: "1000m"
```

### 11.2 Docker

```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
RUN apk add --no-cache clang llvm make libelf-dev linux-headers
COPY . .
RUN make bpf && CGO_ENABLED=0 go build -o leaf main.go

FROM alpine:3.19 AS runtime
RUN apk add --no-cache libelf libpcre bash
COPY --from=builder /app/leaf /usr/local/bin/
COPY --from=builder /app/bpf /bpf
ENV KAFKA_BROKERS=kafka:9092
ENTRYPOINT ["leaf", "run"]
```

### 11.3 Systemd

```ini
[Unit]
Description=Leaf eBPF System Probe
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/leaf run --config /etc/leaf/config.yaml
CapabilityBoundingSet=CAP_SYS_ADMIN CAP_BPF CAP_PERFMON
AmbientCapabilities=CAP_SYS_ADMIN CAP_BPF CAP_PERFMON
NoNewPrivileges=true
ProtectSystem=strict

[Install]
WantedBy=multi-user.target
```

---

## 12. 运维监控

### 12.1 关键指标

| 指标名 | 类型 | 描述 |
|--------|------|------|
| leaf_events_total | counter | 采集的事件总数 |
| leaf_events_network_total | counter | 网络事件数 |
| leaf_events_process_total | counter | 进程事件数 |
| leaf_events_performance_total | counter | 性能事件数 |
| leaf_kafka_messages_total | counter | 发送的 Kafka 消息数 |
| leaf_kafka_send_errors_total | counter | Kafka 发送错误数 |
| leaf_ringbuf_lost_events | counter | RingBuf 丢失事件数 |
| leaf_processing_latency_seconds | histogram | 处理延迟 |
| leaf_cpu_usage_percent | gauge | CPU 使用率 |
| leaf_memory_usage_bytes | gauge | 内存使用 |

### 12.2 Grafana 仪表板

- Events per Second (按类型)
- Kafka Send Rate
- Processing Latency
- RingBuffer Usage
- Memory & CPU Usage
- Error Rates

---

## 13. 附录

### 13.1 术语表

| 术语 | 定义 |
|------|------|
| eBPF | Extended Berkeley Packet Filter |
| fentry | 内核函数入口探针 (比 kprobe 更优) |
| kprobe | 内核动态探针 |
| tracepoint | 内核静态跟踪点 |
| RingBuf | 环形缓冲区 (BPF_MAP_TYPE_RINGBUF) |
| CO-RE | Compile Once - Run Everywhere |
| Sarama | Go Kafka 客户端库 |

### 13.2 参考资料

| 类型 | 资源 |
|------|------|
| eBPF 官方文档 | https://ebpf.io/what-is-ebpf/ |
| cilium/ebpf | https://github.com/cilium/ebpf |
| bpf2go | https://github.com/cilium/ebpf/tree/main/cmd/bpf2go |
| Sarama (Kafka) | https://github.com/IBM/sarama |
| Sonic (JSON) | https://github.com/bytedance/sonic |
| Linux Tracepoints | https://www.kernel.org/doc/html/latest/trace/tracepoints.html |

### 13.3 探针挂载点参考

| 类别 | 挂载点 | 内核版本 | 备注 |
|------|--------|---------|------|
| 网络 | tcp_v4_connect | 4.x+ | fentry 更优 |
| 网络 | tcp_close | 4.x+ | fentry 更优 |
| 网络 | inet_sendmsg | 4.x+ | - |
| 进程 | sched_process_exec | 2.6+ | tracepoint |
| 进程 | sched_process_fork | 2.6+ | tracepoint |
| 进程 | sched_process_exit | 2.6+ | tracepoint |
| 性能 | sched_switch | 2.6+ | tracepoint |
| 性能 | irq_handler_entry | 2.6+ | tracepoint |
| 性能 | softirq_entry | 2.6+ | tracepoint |

### 13.4 现有代码 vs 设计文档对照

| 组件 | 设计文档 | 现有代码 | 状态 |
|------|---------|---------|------|
| 项目名称 | ESPK | leaf | 已调整 |
| 模块路径 | espk/espk | github.com/lyonmu/leaf | 已调整 |
| Kafka 生产者 | segmentio/kafka-go | IBM/sarama | 已调整 |
| JSON 序列化 | 标准库 | bytedance/sonic | 已调整 |
| CLI 框架 | - | alecthomas/kong | 已有 |
| 日志系统 | - | zap + lumberjack | 已有 |
| 配置加载 | Viper | yaml + kong | 已有 |
| eBPF 加载器 | cilium/ebpf | cilium/ebpf (依赖已添加) | 待开发 |
| 探针程序 | C/eBPF | - | 待开发 |
| 事件模型 | Go structs | - | 待开发 |

### 13.5 开发环境要求

```bash
# 系统要求
- Linux Kernel 5.10+
- Go 1.24+
- clang/LLVM 14+
- libelf-dev
- linux-headers

# 开发工具
- make
- git
- docker (可选)

# 安装依赖 (Ubuntu)
sudo apt-get install -y \
    clang \
    llvm \
    libelf-dev \
    linux-headers-$(uname -r) \
    make

# 安装依赖 (CentOS/RHEL)
sudo yum install -y \
    clang \
    llvm \
    libelf-devel \
    kernel-headers \
    make
```

### 13.6 快速开始命令

```bash
# 1. 克隆项目
git clone https://github.com/lyonmu/leaf.git
cd leaf

# 2. 安装 Go 依赖
go mod download

# 3. 编译 eBPF 程序
make bpf

# 4. 构建项目
make build

# 5. 运行测试
go test ./...

# 6. 运行程序
sudo ./bin/leaf run --config config.yaml
```

---

## 文档变更历史

| 版本 | 日期 | 作者 | 变更说明 |
|------|------|------|----------|
| v1.0.0 | 2026-01-09 | Architecture Team | 初始版本 (网络流量分析器) |
| v2.0.0 | 2026-01-09 | Architecture Team | 重构为 eBPF 系统探针, 集成 Kafka 输出, 根据现有代码调整技术选型 |
| v2.1.0 | 2026-01-09 | Architecture Team | 添加详细模块功能说明和开发步骤 |

---

**文档结束**
