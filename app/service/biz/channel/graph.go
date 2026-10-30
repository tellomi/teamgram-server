package channel

import (
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/teamgram/proto/mtproto"
)

// graphDoc is one stats chart. Tokens live in this process. A restart makes
// a minted token come back as GRAPH_EXPIRED_RELOAD. Token "0" is the
// placeholder broadcast and megagroup stats already return.
type graphDoc struct {
	Name string
	Bar  bool
	X    []int64
	Y    []float64
}

var (
	graphMu    sync.Mutex
	graphSeq   atomic.Int64
	graphByTok = map[string]graphDoc{}
)

// MintGraph stores a chart and returns statsGraphAsync with its token.
func MintGraph(name string, xs []int64, ys []float64, bar bool) *mtproto.StatsGraph {
	if len(xs) == 0 {
		xs = []int64{1, 2}
		ys = []float64{0, 0}
	}
	if len(xs) == 1 {
		xs = []int64{xs[0], xs[0] + 86400}
		y := 0.0
		if len(ys) > 0 {
			y = ys[0]
		}
		ys = []float64{y, y}
	}
	if len(ys) < len(xs) {
		grown := make([]float64, len(xs))
		copy(grown, ys)
		ys = grown
	}
	token := "g" + graphID(graphSeq.Add(1))
	graphMu.Lock()
	graphByTok[token] = graphDoc{Name: name, Bar: bar, X: append([]int64(nil), xs...), Y: append([]float64(nil), ys...)}
	graphMu.Unlock()
	return mtproto.MakeTLStatsGraphAsync(&mtproto.StatsGraph{Token: token}).To_StatsGraph()
}

// ReadGraph resolves a token from stats.loadAsyncGraph.
func ReadGraph(token string) (*mtproto.StatsGraph, error) {
	switch token {
	case "":
		return nil, mtproto.ErrGraphInvalidReload
	case "outdated":
		return nil, mtproto.ErrGraphOutdatedReload
	case "expired":
		return nil, mtproto.ErrGraphExpiredReload
	case "0":
		return chartGraph(graphDoc{Name: "Views", X: []int64{1, 2}, Y: []float64{0, 0}}), nil
	}
	graphMu.Lock()
	doc, ok := graphByTok[token]
	graphMu.Unlock()
	if !ok {
		if len(token) > 0 && token[0] == 'g' {
			return nil, mtproto.ErrGraphExpiredReload
		}
		return nil, mtproto.ErrGraphInvalidReload
	}
	return chartGraph(doc), nil
}

func chartGraph(doc graphDoc) *mtproto.StatsGraph {
	kind := "line"
	if doc.Bar {
		kind = "bar"
	}
	name := doc.Name
	if name == "" {
		name = "Value"
	}
	xs := make([]any, 0, len(doc.X)+1)
	ys := make([]any, 0, len(doc.Y)+1)
	xs = append(xs, "x")
	ys = append(ys, "y0")
	for _, v := range doc.X {
		xs = append(xs, v)
	}
	for _, v := range doc.Y {
		ys = append(ys, v)
	}
	raw, _ := json.Marshal(map[string]any{
		"columns": []any{xs, ys},
		"types":   map[string]string{"x": "x", "y0": kind},
		"names":   map[string]string{"y0": name},
		"colors":  map[string]string{"y0": "#4c8bf5"},
	})
	return mtproto.MakeTLStatsGraph(&mtproto.StatsGraph{
		Json: mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: string(raw)}).To_DataJSON(),
	}).To_StatsGraph()
}

func graphID(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
