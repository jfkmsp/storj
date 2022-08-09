// Copyright (C) 2022 Storj Labs, Inc.
// See LICENSE for copying information.

package apigen

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/zeebo/errs"
)

// TODO maybe look at the Go structs MarshalJSON return type instead.
var tsTypeOverrides = map[string]string{
	"uuid.UUID":   "string",
	"time.Time":   "string",
	"memory.Size": "string",
	"[]uint8":     "string", // e.g. []byte
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
	case reflect.Ptr:
		return tsType(t.Elem())
	case reflect.Slice, reflect.Array:
		return tsType(t.Elem()) + "[]"
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "number"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "number"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Bool:
		return "boolean"
	case reflect.Struct:
		return t.Name()
	default:
		panic("unhandled type: " + t.Name())
	}
}

// MustWriteTS writes generated TypeScript code into a file.
func (a *API) MustWriteTS(path string) {
	f := newTSGenFile(path, a)

	f.generateTS()

	err := f.write()
	if err != nil {
		panic(errs.Wrap(err))
	}
}

type tsGenFile struct {
	result string
	path   string
	// types is a map of struct types and their struct type dependencies.
	// We use this to ensure all dependencies are written before their parent.
	types map[reflect.Type][]reflect.Type
	// typeList is a list of all struct types. We use this for sorting the types alphabetically
	// to ensure that the diff is minimized when the file is regenerated.
	typeList     []reflect.Type
	typesWritten map[reflect.Type]bool
	api          *API
}

func newTSGenFile(filepath string, api *API) *tsGenFile {
	f := &tsGenFile{
		path:         filepath,
		types:        make(map[reflect.Type][]reflect.Type),
		typesWritten: make(map[reflect.Type]bool),
		api:          api,
	}

	f.pf("// AUTOGENERATED BY private/apigen")
	f.pf("// DO NOT EDIT.")
	f.pf("")
	f.pf("import { HttpClient } from '@/utils/httpClient'")
	f.pf("")

	return f
}

func (f *tsGenFile) pf(format string, a ...interface{}) {
	f.result += fmt.Sprintf(format+"\n", a...)
}

func (f *tsGenFile) write() error {
	return os.WriteFile(f.path, []byte(f.result), 0644)
}

func (f *tsGenFile) getStructsFromType(t reflect.Type) {
	t = getBasicReflectType(t)
	override := tsTypeOverrides[t.String()]
	if len(override) > 0 {
		return
	}

	// if it is a struct, get any types needed from the fields
	if t.Kind() == reflect.Struct {
		if _, ok := f.types[t]; !ok {
			f.types[t] = []reflect.Type{}
			f.typeList = append(f.typeList, t)

			for i := 0; i < t.NumField(); i++ {
				field := t.Field(i)
				f.getStructsFromType(field.Type)

				if field.Type.Kind() == reflect.Struct {
					deps := f.types[t]
					deps = append(deps, getBasicReflectType(field.Type))
					f.types[t] = deps
				}
			}
		}
	}

}

func (f *tsGenFile) generateTS() {
	f.createClasses()

	for _, group := range f.api.EndpointGroups {
		// Not sure if this is a good name
		f.createAPIClient(group)
	}
}

func (f *tsGenFile) emitStruct(t reflect.Type) {
	override := tsTypeOverrides[t.String()]
	if len(override) > 0 {
		return
	}
	if f.typesWritten[t] {
		return
	}
	if t.Kind() != reflect.Struct {
		// TODO: handle slices
		// I'm not sure this is necessary. If it's not a struct then we don't need to create a TS class for it.
		// We just use a JS array of the class.
		return
	}

	for _, d := range f.types[t] {
		if f.typesWritten[d] {
			continue
		}
		f.emitStruct(d)
	}

	f.pf("class %s {", t.Name())
	defer f.pf("}\n")

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		attributes := strings.Fields(field.Tag.Get("json"))
		if len(attributes) == 0 || attributes[0] == "" {
			panic(t.Name() + " missing json declaration")
		}
		if attributes[0] == "-" {
			continue
		}
		f.pf("\t%s: %s;", attributes[0], tsType(field.Type))
	}

	f.typesWritten[t] = true
}

func (f *tsGenFile) createClasses() {
	for _, group := range f.api.EndpointGroups {
		for _, method := range group.endpoints {
			if method.Request != nil {
				reqType := reflect.TypeOf(method.Request)
				f.getStructsFromType(reqType)
			}
			if method.Response != nil {
				resType := reflect.TypeOf(method.Response)
				f.getStructsFromType(resType)
			}
			if len(method.QueryParams) > 0 {
				for _, p := range method.QueryParams {
					t := getBasicReflectType(p.Type)
					f.getStructsFromType(t)
				}
			}
		}
	}

	sort.Slice(f.typeList, func(i, j int) bool {
		return strings.Compare(f.typeList[i].Name(), f.typeList[j].Name()) < 0
	})

	for _, t := range f.typeList {
		f.emitStruct(t)
	}
}

func (f *tsGenFile) createAPIClient(group *EndpointGroup) {
	f.pf("export class %sHttpApi%s {", group.Prefix, strings.ToUpper(f.api.Version))
	f.pf("\tprivate readonly http: HttpClient = new HttpClient();")
	f.pf("\tprivate readonly ROOT_PATH: string = '/api/%s/%s';", f.api.Version, group.Prefix)
	f.pf("")
	for _, method := range group.endpoints {
		funcArgs, path := f.getArgsAndPath(method)

		returnStmt := "return"
		returnType := "void"
		if method.Response != nil {
			returnType = tsType(getBasicReflectType(reflect.TypeOf(method.Response)))
			if v := reflect.ValueOf(method.Response); v.Kind() == reflect.Array || v.Kind() == reflect.Slice {
				returnType = fmt.Sprintf("Array<%s>", returnType)
			}
			returnStmt += fmt.Sprintf(" response.json().then((body) => body as %s)", returnType)
		}
		returnStmt += ";"

		f.pf("\tpublic async %s(%s): Promise<%s> {", method.RequestName, funcArgs, returnType)
		f.pf("\t\tconst path = `%s`;", path)

		if method.Request != nil {
			f.pf("\t\tconst response = await this.http.%s(path, JSON.stringify(request));", strings.ToLower(method.Method))
		} else {
			f.pf("\t\tconst response = await this.http.%s(path);", strings.ToLower(method.Method))
		}

		f.pf("\t\tif (response.ok) {")
		f.pf("\t\t\t%s", returnStmt)
		f.pf("\t\t}")
		f.pf("\t\tconst err = await response.json()")
		f.pf("\t\tthrow new Error(err.error)")
		f.pf("\t}\n")
	}
	f.pf("}")
}

func (f *tsGenFile) getArgsAndPath(method *fullEndpoint) (funcArgs, path string) {
	// remove path parameter placeholders
	path = method.Path
	i := strings.Index(path, "{")
	if i > -1 {
		path = method.Path[:i]
	}
	path = "${this.ROOT_PATH}" + path

	if method.Request != nil {
		t := getBasicReflectType(reflect.TypeOf(method.Request))
		funcArgs += fmt.Sprintf("request: %s, ", tsType(t))
	}

	for _, p := range method.PathParams {
		funcArgs += fmt.Sprintf("%s: %s, ", p.Name, tsType(p.Type))
		path += fmt.Sprintf("/${%s}", p.Name)
	}

	for i, p := range method.QueryParams {
		if i == 0 {
			path += "?"
		} else {
			path += "&"
		}

		funcArgs += fmt.Sprintf("%s: %s, ", p.Name, tsType(p.Type))
		path += fmt.Sprintf("%s=${%s}", p.Name, p.Name)
	}

	path = strings.ReplaceAll(path, "//", "/")

	return strings.Trim(funcArgs, ", "), path
}
