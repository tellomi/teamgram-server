package e2e

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// smokeMethod is one implemented P0/P1 RPC. CRC is the layer-232 constructor
// the server decodes. Add a row by regenerating this list; see docs/testing.md.
type smokeMethod struct {
	Name string
	CRC  int32
}

// smokeRequest builds a layer-232 request with the smallest values that still
// encode. Dummy ids and empty peers are intentional: the smoke test allows a
// business error and fails only when the method is not implemented or the
// reply does not decode.
func smokeRequest(m smokeMethod) (mtproto.TLObject, error) {
	if m.CRC == 0 {
		return nil, fmt.Errorf("no constructor for %s", m.Name)
	}
	obj := mtproto.NewTLObjectByClassID(m.CRC)
	if obj == nil {
		return nil, fmt.Errorf("constructor %d is not registered for %s", m.CRC, m.Name)
	}
	fillSmoke(reflect.ValueOf(obj), 0)
	// Some layer-232 constructors put several fields behind one flag bit.
	// Setting only one of them encodes a payload the server drops, and the
	// client then waits until the ack retry limit. Drop the field that
	// breaks a round trip; a business error is still a successful smoke call.
	repairSmoke(obj)
	return obj, nil
}

func fillSmoke(v reflect.Value, depth int) {
	if !v.IsValid() || depth > 8 {
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		fillSmoke(v.Elem(), depth+1)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Field(i)
			sf := v.Type().Field(i)
			if !sf.IsExported() || !f.CanSet() || sf.Name == "PredicateName" || sf.Name == "Constructor" {
				continue
			}
			switch f.Kind() {
			case reflect.Pointer:
				if isWrapper(f.Type().Elem()) {
					continue
				}
				if !f.IsNil() {
					fillSmoke(f, depth+1)
					continue
				}
				// Only a real constructor is safe. A zero object still sets the
				// presence flag and then writes no bytes, which desyncs the session.
				if sample, ok := smokeSample(f.Type()); ok {
					f.Set(sample)
					fillSmoke(f, depth+1)
				}
			case reflect.Slice:
				if f.IsNil() {
					f.Set(reflect.MakeSlice(f.Type(), 0, 0))
				}
			case reflect.String:
				if f.Len() == 0 {
					f.SetString("e2e")
				}
			case reflect.Int32:
				if f.Int() == 0 && (sf.Name == "Limit" || strings.HasSuffix(sf.Name, "Limit")) {
					f.SetInt(20)
				}
			}
		}
	}
}

func isWrapper(t reflect.Type) bool {
	return t.PkgPath() == reflect.TypeOf(wrapperspb.Int32Value{}).PkgPath()
}

// repairSmoke clears the optional field that stops a request from decoding
// back. One pass is enough: the broken payloads set a single shared flag.
func repairSmoke(obj mtproto.TLObject) {
	if smokeEncodes(obj) {
		return
	}
	var fields []reflect.Value
	collectSmokeFields(reflect.ValueOf(obj), &fields)
	for _, f := range fields {
		if !f.IsValid() || !f.CanSet() || f.IsNil() {
			continue
		}
		prev := f.Interface()
		f.Set(reflect.Zero(f.Type()))
		if smokeEncodes(obj) {
			return
		}
		f.Set(reflect.ValueOf(prev))
	}
}

func collectSmokeFields(v reflect.Value, out *[]reflect.Value) {
	if !v.IsValid() {
		return
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}
		collectSmokeFields(v.Elem(), out)
		return
	}
	if v.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		sf := v.Type().Field(i)
		if !sf.IsExported() || !f.CanSet() {
			continue
		}
		switch f.Kind() {
		case reflect.Pointer, reflect.Slice:
			if f.IsNil() {
				continue
			}
			if f.Kind() == reflect.Pointer && !isWrapper(f.Type().Elem()) {
				collectSmokeFields(f, out)
			}
			*out = append(*out, f)
		case reflect.Struct:
			collectSmokeFields(f, out)
		}
	}
}

func smokeEncodes(obj mtproto.TLObject) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	raw, err := encodeTL(obj)
	if err != nil {
		return false
	}
	return smokeDecodeExact(raw) == nil
}

func smokeDecodeExact(raw []byte) error {
	d := mtproto.NewDecodeBuf(raw)
	decoded := d.Object()
	if err := d.GetError(); err != nil {
		return err
	}
	if decoded == nil {
		return fmt.Errorf("decoded nil")
	}
	if d.GetOffset() != len(raw) {
		return fmt.Errorf("decoded %T but left %d bytes", decoded, len(raw)-d.GetOffset())
	}
	return nil
}

func smokeSample(ptr reflect.Type) (reflect.Value, bool) {
	id, ok := smokeEmptyClass[ptr.Elem().Name()]
	if !ok {
		return reflect.Value{}, false
	}
	obj := mtproto.NewTLObjectByClassID(id)
	if obj == nil {
		return reflect.Value{}, false
	}
	mv := reflect.ValueOf(obj)
	if mv.Type() == ptr {
		return mv, true
	}
	to := mv.MethodByName("To_" + ptr.Elem().Name())
	if !to.IsValid() || to.Type().NumIn() != 0 || to.Type().NumOut() != 1 {
		return reflect.Value{}, false
	}
	out := to.Call(nil)[0]
	if out.Type() != ptr {
		return reflect.Value{}, false
	}
	return out, true
}
