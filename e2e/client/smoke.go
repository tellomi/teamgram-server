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
	return obj, nil
}

func fillSmoke(v reflect.Value, depth int) {
	if !v.IsValid() || depth > 4 {
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
				if !f.IsNil() || isWrapper(f.Type().Elem()) {
					fillSmoke(f, depth+1)
					continue
				}
				if sample, ok := smokeSample(f.Type()); ok {
					f.Set(sample)
					fillSmoke(f, depth+1)
					continue
				}
				created := reflect.New(f.Type().Elem())
				fillSmoke(created.Elem(), depth+1)
				f.Set(created)
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
