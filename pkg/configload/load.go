// Package configload loads service yaml the way the process entrypoints do.
// ${TELLOMI_OTEL_ENDPOINT} is expanded from the environment. An empty value
// disables trace export so go-zero does not dial the unexpanded placeholder.
package configload

import (
	"os"
	"reflect"
	"strings"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/trace"
)

var telemetryType = reflect.TypeOf(trace.Config{})

// MustLoad reads path into v, expanding environment variables, and disables
// OTLP export when Telemetry.Endpoint is empty.
func MustLoad(path string, v any) {
	conf.MustLoad(path, v, conf.UseEnv())
	normalizeTelemetry(v)
}

func normalizeTelemetry(v any) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return
	}
	walkTelemetry(rv.Elem())
}

func walkTelemetry(rv reflect.Value) {
	if !rv.IsValid() {
		return
	}
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			return
		}
		walkTelemetry(rv.Elem())
	case reflect.Struct:
		if rv.Type() == telemetryType && rv.CanAddr() {
			normalizeTrace(rv.Addr().Interface().(*trace.Config))
			return
		}
		for i := 0; i < rv.NumField(); i++ {
			f := rv.Field(i)
			if !f.CanAddr() {
				continue
			}
			switch f.Kind() {
			case reflect.Struct, reflect.Pointer:
				walkTelemetry(f)
			}
		}
	}
}

func normalizeTrace(c *trace.Config) {
	ep := strings.TrimSpace(os.ExpandEnv(c.Endpoint))
	if ep == "" || strings.Contains(ep, "${") {
		c.Endpoint = ""
		c.Disabled = true
		return
	}
	c.Endpoint = ep
}
