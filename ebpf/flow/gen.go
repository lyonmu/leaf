package flow

//go:generate go tool bpf2go -tags linux --go-package flow Flow flow.c -- -I/usr/include/x86_64-linux-gnu
