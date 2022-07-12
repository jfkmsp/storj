// Copyright (C) 2022 Storj Labs, Inc.
// See LICENSE for copying information.

package apigen

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/zeebo/errs"
)

var tsTypeOverrides = map[string]string{
	"uuid.UUID":   "string",
	"time.Time":   "string",
	"memory.Size": "number",
}

// getBasicReflectType dereferences a pointer and gets the basic types from slices.
func getBasicReflectType(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	return t
}

// tsType gets the corresponding typescript type for a provided reflect.Type.
// Input is expected to be a (non pointer) struct or primitive.
func tsType(t reflect.Type) string {
	override := tsTypeOverrides[t.String()]
	if len(override) > 0 {
		return override
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32:
		return "number"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Struct:
		return t.Name()
	case reflect.Bool:
		return "boolean"
	case reflect.Slice:
		return t.Name() + "[]"
	default:
		//panic("unhandled type: " + t.Name())

		return ""
	}
}

// MustWriteTS writes generated TypeScript code into a file.
func (a *API) MustWriteTS(path string) {
	f := newTSGenFile(path, a)

	err := f.generateTS()
	if err != nil {
		panic(errs.Wrap(err))
	}

	err = f.write()
	if err != nil {
		panic(errs.Wrap(err))
	}
}

type tsGenFile struct {
	result string
	path   string
	types  map[reflect.Type]bool
	api    *API
}

func newTSGenFile(filepath string, api *API) *tsGenFile {
	f := &tsGenFile{
		path:  filepath,
		types: make(map[reflect.Type]bool),
		api:   api,
	}

	f.p("<<<<<< START >>>>>")
	f.p("// AUTOGENERATED BY private/apigen")
	f.p("// DO NOT EDIT.")

	return f
}

func (f *tsGenFile) p(format string, a ...interface{}) {
	f.result += fmt.Sprintf(format+"\n", a...)
}

func (f *tsGenFile) write() error {
	f.p("<<<<<< END >>>>>")
	return os.WriteFile(f.path, []byte(f.result), 0644)
}

func (f *tsGenFile) generateTS() error {
	for _, group := range f.api.EndpointGroups {
		for _, method := range group.endpoints {
			if method.Request != nil {
				reqType := reflect.TypeOf(method.Request)
				t := getBasicReflectType(reqType)
				override := tsTypeOverrides[reqType.String()]
				if len(override) == 0 {
					f.types[t] = true
				}
			}
			if method.Response != nil {
				resType := reflect.TypeOf(method.Response)
				t := getBasicReflectType(resType)
				override := tsTypeOverrides[resType.String()]
				if len(override) == 0 {
					f.types[t] = true
				}
			}
			if len(method.Params) > 0 {
				for _, p := range method.Params {
					t := getBasicReflectType(p.Type)
					override := tsTypeOverrides[p.Type.String()]
					if len(override) == 0 {
						f.types[t] = true
					}
				}
			}
		}
	}

	fmt.Println("all types")
	for t, _ := range f.types {
		fmt.Println(t.String())
		fmt.Println(tsType(t))
		f.emitStruct(t)
	}
	fmt.Println("end")

	return nil
}

func (f *tsGenFile) emitStruct(t reflect.Type) {
	if t.Kind() != reflect.Struct {
		// TODO: handle slices
		return
	}

	f.p("class %s {", t.Name())
	defer f.p("}")

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		attributes := strings.Fields(field.Tag.Get("json"))
		if len(attributes) == 0 || attributes[0] == "" {
			panic(t.Name() + " missing json declaration")
		}
		if attributes[0] == "-" {
			continue
		}
		f.p("\t%s: %s;", attributes[0], tsType(field.Type))
	}

	f.p("\n\tstatic fromJSON(v unknown): %s {", t.Name())
	// TODO:
	f.p("\t}")
}
