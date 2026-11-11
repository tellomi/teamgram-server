package gnet

import (
	"net"
	"sync"
	"testing"
)

func TestClassifyNativeEntry(t *testing.T) {
	if got := classifyNativeEntry(10443, false, 443); got != entryDirect10443 {
		t.Fatalf("forged dest port on the public listener: %s", got)
	}
	if got := classifyNativeEntry(10443, false, 0); got != entryDirect10443 {
		t.Fatalf("direct %s", got)
	}
	if got := classifyNativeEntry(10444, false, 443); got != "" {
		t.Fatalf("optional ppv1 listener counted as an entry: %s", got)
	}
	if got := classifyNativeEntry(5222, false, 0); got != "" {
		t.Fatalf("other direct port %s", got)
	}
	if got := classifyNativeEntry(10445, true, 10443); got != entryRelay10445 {
		t.Fatalf("240:10443 via private PROXY: %s", got)
	}
	if got := classifyNativeEntry(10445, true, 443); got != entryRelay240443 {
		t.Fatalf("240:443 via private PROXY: %s", got)
	}
	if got := classifyNativeEntry(10445, true, 8443); got != entryUnknown {
		t.Fatalf("unknown frontend guessed: %s", got)
	}
	if got := classifyNativeEntry(10445, true, 0); got != entryUnknown {
		t.Fatalf("missing dest port guessed: %s", got)
	}
	if got := classifyNativeEntry(10445, false, 443); got != "" {
		t.Fatalf("untrusted header became %s", got)
	}
}

func TestEntryCountersAreExclusiveAndIdempotent(t *testing.T) {
	before := EntryTotals()
	ctx := newConnContext()
	ctx.assignEntry(entryDirect10443)
	ctx.assignEntry(entryRelay240443)
	if ctx.entry != entryDirect10443 {
		t.Fatalf("reclassified %s", ctx.entry)
	}
	addEntryBytes(ctx.entry, 10, 4)
	addEntryBytes(ctx.entry, 3, 0)
	after := EntryTotals()
	if d := after.Conns[entryDirect10443] - before.Conns[entryDirect10443]; d != 1 {
		t.Fatalf("conn delta %d", d)
	}
	if d := after.In[entryDirect10443] - before.In[entryDirect10443]; d != 13 {
		t.Fatalf("in delta %d", d)
	}
	if d := after.Out[entryDirect10443] - before.Out[entryDirect10443]; d != 4 {
		t.Fatalf("out delta %d", d)
	}
	if after.Conns[entryRelay240443] != before.Conns[entryRelay240443] {
		t.Fatal("direct connection also counted as 240:443")
	}
}

func TestEntryBytesConcurrent(t *testing.T) {
	before := EntryTotals()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 100; n++ {
				addEntryBytes(entryRelay10445, 2, 1)
			}
		}()
	}
	wg.Wait()
	after := EntryTotals()
	if d := after.In[entryRelay10445] - before.In[entryRelay10445]; d != 1600 {
		t.Fatalf("concurrent in %d", d)
	}
	if d := after.Out[entryRelay10445] - before.Out[entryRelay10445]; d != 800 {
		t.Fatalf("concurrent out %d", d)
	}
}

func TestCountInboundDoesNotDoubleCount(t *testing.T) {
	before := EntryTotals()
	ctx := newConnContext()
	ctx.assignEntry(entryRelay240443)
	countInbound(ctx, 20)
	countInbound(ctx, 20)
	countInbound(ctx, 25)
	ctx.unconsumed = 5
	countInbound(ctx, 5)
	after := EntryTotals()
	if d := after.In[entryRelay240443] - before.In[entryRelay240443]; d != 25 {
		t.Fatalf("inbound counted %d, want 25", d)
	}
	if after.Conns[entryRelay240443]-before.Conns[entryRelay240443] != 1 {
		t.Fatal("bytes path opened another connection")
	}
}

func TestDestPortIgnoresNonTCP(t *testing.T) {
	if destPort(&net.UDPAddr{IP: net.ParseIP("203.0.113.8"), Port: 443}) != 0 {
		t.Fatal("udp destination counted")
	}
	if destPort(&net.TCPAddr{IP: net.ParseIP("203.0.113.8"), Port: 443}) != 443 {
		t.Fatal("tcp destination")
	}
	if tcpPort(&net.TCPAddr{IP: net.ParseIP("10.0.0.8"), Port: 10443}) != 10443 {
		t.Fatal("local port")
	}
}

func TestRejectReasonIsFixed(t *testing.T) {
	noteEntryReject("untrusted_peer")
	noteEntryReject("203.0.113.9")
	noteEntryReject("client_ip")
}
