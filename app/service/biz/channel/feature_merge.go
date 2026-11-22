package channel

import (
	"bytes"
	"encoding/json"
)

// stampFeatureBase remembers the row this copy was read from so a later
// save can merge its own edits onto whatever is stored then.
func stampFeatureBase(f *Features) *Features {
	if f == nil {
		f = &Features{}
	}
	f.base = f.clone()
	f.baseRev = f.Rev
	return f
}

// mergeFeatureWrite applies the changes between proposed.base and proposed
// onto current. Map keys and array elements the caller did not touch stay,
// including a JoinTop another invite wrote after this copy was read.
// A nil base is an empty read: only fields that differ from zero are applied,
// so a snapshot-less write cannot clear HiddenPrehistory.
func mergeFeatureWrite(current, proposed *Features) (*Features, error) {
	if proposed == nil {
		if current == nil {
			return &Features{}, nil
		}
		return current.clone(), nil
	}
	base := proposed.base
	if base == nil {
		base = &Features{}
	}
	rev := int64(0)
	if current != nil {
		rev = current.Rev
	}
	merged, err := applyFeatureDiff(current, base, proposed)
	if err != nil {
		return nil, err
	}
	merged.Rev = rev + 1
	merged.base = nil
	merged.baseRev = 0
	merged.loadFailed = false
	return merged, nil
}

func applyFeatureDiff(current, base, proposed *Features) (*Features, error) {
	cur, err := featureMap(current)
	if err != nil {
		return nil, err
	}
	b, err := featureMap(base)
	if err != nil {
		return nil, err
	}
	p, err := featureMap(proposed)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	for k := range cur {
		seen[k] = struct{}{}
	}
	for k := range b {
		seen[k] = struct{}{}
	}
	for k := range p {
		seen[k] = struct{}{}
	}
	for k := range seen {
		if canonRaw(p[k]) == canonRaw(b[k]) {
			continue
		}
		merged, drop, err := mergeValue(cur[k], b[k], p[k])
		if err != nil {
			return nil, err
		}
		if drop {
			delete(cur, k)
			continue
		}
		cur[k] = merged
	}
	delete(cur, "Rev")
	raw, err := json.Marshal(cur)
	if err != nil {
		return nil, err
	}
	out := &Features{}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, err
	}
	return out, nil
}

func featureMap(f *Features) (map[string]json.RawMessage, error) {
	if f == nil {
		return map[string]json.RawMessage{}, nil
	}
	cp := *f
	cp.Rev = 0
	cp.base = nil
	cp.baseRev = 0
	cp.loadFailed = false
	raw, err := json.Marshal(&cp)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]json.RawMessage{}
	}
	delete(m, "Rev")
	return m, nil
}

func mergeValue(current, base, proposed json.RawMessage) (json.RawMessage, bool, error) {
	if jsonKind(proposed) == '{' || jsonKind(base) == '{' {
		raw, err := mergeJSONObject(current, base, proposed)
		return raw, false, err
	}
	if jsonKind(proposed) == '[' || jsonKind(base) == '[' {
		raw, err := mergeJSONArray(current, base, proposed)
		return raw, false, err
	}
	if canonRaw(proposed) == "null" && canonRaw(base) != "null" {
		return nil, true, nil
	}
	return append(json.RawMessage(nil), proposed...), false, nil
}

func mergeJSONObject(current, base, proposed json.RawMessage) (json.RawMessage, error) {
	cm := decodeObject(current)
	bm := decodeObject(base)
	pm := decodeObject(proposed)
	if jsonKind(proposed) != '{' {
		for k := range bm {
			delete(cm, k)
		}
		if len(cm) == 0 {
			return []byte("null"), nil
		}
		return json.Marshal(cm)
	}
	seen := map[string]struct{}{}
	for k := range cm {
		seen[k] = struct{}{}
	}
	for k := range bm {
		seen[k] = struct{}{}
	}
	for k := range pm {
		seen[k] = struct{}{}
	}
	for k := range seen {
		if canonRaw(pm[k]) == canonRaw(bm[k]) {
			continue
		}
		if canonRaw(pm[k]) == "null" && canonRaw(bm[k]) != "null" {
			delete(cm, k)
			continue
		}
		cm[k] = append(json.RawMessage(nil), pm[k]...)
	}
	if len(cm) == 0 {
		return []byte("null"), nil
	}
	return json.Marshal(cm)
}

func mergeJSONArray(current, base, proposed json.RawMessage) (json.RawMessage, error) {
	cb := decodeArray(current)
	bb := decodeArray(base)
	pb := decodeArray(proposed)
	// An unchanged list keeps the stored order, including a reorder another
	// writer already committed. A changed list uses the caller's order, so
	// channels.reorderUsernames is not dropped while the channel row is updated.
	if sameRawSequence(bb, pb) {
		if jsonKind(current) == '[' {
			return append(json.RawMessage(nil), bytes.TrimSpace(current)...), nil
		}
		if len(cb) == 0 {
			return []byte("null"), nil
		}
		return json.Marshal(cb)
	}
	baseCount := countRaw(bb)
	propCount := countRaw(pb)
	out := append([]json.RawMessage{}, pb...)
	occ := map[string]int{}
	for _, el := range cb {
		key := canonRaw(el)
		occ[key]++
		if occ[key] <= baseCount[key] {
			continue
		}
		callerAdded := propCount[key] - baseCount[key]
		if callerAdded < 0 {
			callerAdded = 0
		}
		if occ[key]-baseCount[key] <= callerAdded {
			continue
		}
		out = append(out, el)
	}
	if out == nil {
		out = []json.RawMessage{}
	}
	return json.Marshal(out)
}

func sameRawSequence(a, b []json.RawMessage) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if canonRaw(a[i]) != canonRaw(b[i]) {
			return false
		}
	}
	return true
}

func countRaw(list []json.RawMessage) map[string]int {
	out := map[string]int{}
	for _, el := range list {
		out[canonRaw(el)]++
	}
	return out
}

func decodeObject(raw json.RawMessage) map[string]json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if jsonKind(raw) != '{' {
		return map[string]json.RawMessage{}
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return map[string]json.RawMessage{}
	}
	return m
}

func decodeArray(raw json.RawMessage) []json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if jsonKind(raw) != '[' {
		return nil
	}
	var a []json.RawMessage
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil
	}
	return a
}

func jsonKind(raw json.RawMessage) byte {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return 0
	}
	return raw[0]
}

func canonRaw(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "null"
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return string(raw)
	}
	return string(b)
}
