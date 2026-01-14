//go:build ignore

#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <bpf/bpf_helpers.h>

// 协议常量
#ifndef IPPROTO_TCP
#define IPPROTO_TCP 6
#endif
#ifndef IPPROTO_UDP
#define IPPROTO_UDP 17
#endif
#ifndef ETH_P_IP
#define ETH_P_IP 0x0800
#endif
#ifndef bpf_ntohs
#define bpf_ntohs(x) __builtin_bswap16(x)
#endif

// 五元组事件结构体
struct flow_event {
    __u32 src_ip;
    __u32 dst_ip;
    __u16 src_port;
    __u16 dst_port;
    __u8  protocol;
    __u8  pad[3];  // 8字节对齐
};

// Ring Buffer 用于发送事件到用户态
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 1 << 20);  // 1MB buffer
} events SEC(".maps");

// XDP 程序：解析数据包并提取五元组
SEC("xdp")
int process_packet(struct xdp_md *ctx) {
    void *data = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;
    struct ethhdr *eth = data;

    // 检查以太网头
    if (data + sizeof(*eth) > data_end)
        return XDP_DROP;

    // 只处理 IPv4
    if (bpf_ntohs(eth->h_proto) != ETH_P_IP)
        return XDP_PASS;

    struct iphdr *ip = data + sizeof(*eth);
    if (data + sizeof(*eth) + sizeof(*ip) > data_end)
        return XDP_DROP;

    struct flow_event *event = bpf_ringbuf_reserve(&events, sizeof(*event), 0);
    if (!event)
        return XDP_PASS;

    event->src_ip = ip->saddr;
    event->dst_ip = ip->daddr;
    event->protocol = ip->protocol;
    event->pad[0] = 0;

    // 处理 TCP/UDP 端口
    void *transport = ip + 1;

    if (ip->protocol == IPPROTO_TCP) {
        struct tcphdr *tcp = transport;
        if (transport + sizeof(*tcp) <= data_end) {
            event->src_port = tcp->source;
            event->dst_port = tcp->dest;
        }
    } else if (ip->protocol == IPPROTO_UDP) {
        struct udphdr *udp = transport;
        if (transport + sizeof(*udp) <= data_end) {
            event->src_port = udp->source;
            event->dst_port = udp->dest;
        }
    }

    bpf_ringbuf_submit(event, 0);
    return XDP_PASS;
}

char __license[] SEC("license") = "Dual MIT/GPL";
