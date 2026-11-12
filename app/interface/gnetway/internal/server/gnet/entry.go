package gnet

import (
	"net"
	"strconv"
	"sync/atomic"

	"github.com/panjf2000/gnet/v2"
)

// Native entry classes. They are fixed labels: no client IP, auth key,
// session id, or user id. relay240_443 and relay_10445 are mutually
// exclusive. A PROXY header is used only after the socket peer was admitted
// by TrustedProxyCIDRs. A header on any other listener does not change the class.
const (
	entryDirect10443 = "direct_10443"
	entryRelay10445  = "relay_10445"
	entryRelay240443 = "relay240_443"
	entryUnknown     = "relay_unknown"
)

var entryOrder = []string{entryDirect10443, entryRelay10445, entryRelay240443, entryUnknown}

type entryTotals struct {
	Conns map[string]int64
	In    map[string]int64
	Out   map[string]int64
}

var (
	entryConn [4]atomic.Int64
	entryIn   [4]atomic.Int64
	entryOut  [4]atomic.Int64
)

func entryIndex(class string) int {
	switch class {
	case entryDirect10443:
		return 0
	case entryRelay10445:
		return 1
	case entryRelay240443:
		return 2
	case entryUnknown:
		return 3
	default:
		return -1
	}
}

// classifyNativeEntry picks one class.
// admitted is true only after a trusted PROXY peer's header was accepted.
// destPort is the header's destination port in that case, otherwise ignored.
// Any other destination port is relay_unknown, not a guessed 443 or 10443.
func classifyNativeEntry(localPort int, admitted bool, destPort int) string {
	if !admitted {
		if localPort == 10443 {
			return entryDirect10443
		}
		return ""
	}
	switch destPort {
	case 443:
		return entryRelay240443
	case 10443:
		return entryRelay10445
	default:
		return entryUnknown
	}
}

func (ctx *connContext) assignEntry(class string) {
	if ctx == nil || class == "" || ctx.entry != "" {
		return
	}
	ctx.entry = class
	if i := entryIndex(class); i >= 0 {
		entryConn[i].Add(1)
		entryConnMetric.Inc(class)
	}
}

func addEntryBytes(class string, in, out int64) {
	i := entryIndex(class)
	if i < 0 {
		return
	}
	if in > 0 {
		entryIn[i].Add(in)
		entryBytesMetric.Add(float64(in), class, "in")
	}
	if out > 0 {
		entryOut[i].Add(out)
		entryBytesMetric.Add(float64(out), class, "out")
	}
}

func inboundBuffered(c gnet.Conn) int {
	if c == nil {
		return 0
	}
	return c.InboundBuffered()
}

func countInbound(ctx *connContext, buffered int) {
	if ctx == nil || ctx.entry == "" || buffered < ctx.unconsumed {
		return
	}
	if n := buffered - ctx.unconsumed; n > 0 {
		addEntryBytes(ctx.entry, int64(n), 0)
		ctx.unconsumed = buffered
	}
}

func noteEntryReject(reason string) {
	switch reason {
	case "untrusted_peer", "timeout", "duplicate", "bad_header", "version", "local", "udp", "missing_header", "oversized", "discard", "command", "family", "unix", "unspec":
	default:
		reason = "other"
	}
	entryRejectMetric.Inc(reason)
}

func tcpPort(addr net.Addr) int {
	if addr == nil {
		return 0
	}
	_, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		return 0
	}
	return n
}

func destPort(addr net.Addr) int {
	tcp, ok := addr.(*net.TCPAddr)
	if !ok || tcp == nil {
		return 0
	}
	return tcp.Port
}

// EntryTotals is the process counters for tests. Labels are the fixed classes.
func EntryTotals() entryTotals {
	out := entryTotals{
		Conns: map[string]int64{},
		In:    map[string]int64{},
		Out:   map[string]int64{},
	}
	for i, class := range entryOrder {
		out.Conns[class] = entryConn[i].Load()
		out.In[class] = entryIn[i].Load()
		out.Out[class] = entryOut[i].Load()
	}
	return out
}
