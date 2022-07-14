// Copyright (C) 2022 Storj Labs, Inc.
// See LICENSE for copying information.

package apigen

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/zeebo/errs"
)

var openApiTypeOverrides = map[string]string{
	"uuid.UUID":   "string",
	"time.Time":   "string",
	"memory.Size": "integer",
	"[]uint8":     "string", // e.g. []byte
}

type openAPISpec struct {
	Openapi string                  `json:"openapi"`
	Info    specInfo                `json:"info"`
	Paths   map[string]specEndpoint `json:"paths"`
}

type specInfo struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

type specEndpoint struct {
	Get    *specMethod `json:"get,omitempty"`
	Patch  *specMethod `json:"patch,omitempty"`
	Post   *specMethod `json:"post,omitempty"`
	Delete *specMethod `json:"delete,omitempty"`
}

type specMethod struct {
	OperationID string    `json:"operationId"`
	Summary     string    `json:"summary"`
	Description string    `json:"description"`
	Response    responses `json:"responses"`
}

type responses struct {
	Num200 code200 `json:"200"` //change to struct
}

type code200 struct {
	Description string      `json:"description"`
	Content     contentType `json:"content"`
}

type contentType struct {
	ApplicationJSON schema `json:"application/json"`
}

type schema struct {
	Schema schemaProps `json:"schema"`
}

type schemaProps struct {
	Type       string                  `json:"type"`
	Properties map[string]propertyInfo `json:"properties,omitempty"`
}

type propertyInfo struct {
	Type string `json:"type"`
}

// openApiType gets the corresponding typescript type for a provided reflect.Type.
// Input is expected to be a (non pointer) struct or primitive.
func openApiType(t reflect.Type) string {
	override := tsTypeOverrides[t.String()]
	if len(override) > 0 {
		return override
	}
	switch t.Kind() {
	case reflect.Ptr:
		return openApiType(t.Elem())
	case reflect.Slice:
		return openApiType(t.Elem()) + "[]"
	case reflect.String:
		return "string"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "integer"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Bool:
		return "boolean"
	case reflect.Struct:
		return t.Name()
	default:
		panic("unhandled type: " + t.Name())

		//return ""
	}
}

// MustWriteOpenAPI writes generated OpenAPI code into a file.
func (a *API) MustWriteOpenAPI(path string) {
	f := newOpenAPIGenFile(path, a)

	err := f.generateOpenAPI()
	if err != nil {
		panic(errs.Wrap(err))
	}

	err = f.write()
	if err != nil {
		panic(errs.Wrap(err))
	}
}

type openAPIGenFile struct {
	result string
	path   string
	api    *API
}

func newOpenAPIGenFile(filepath string, api *API) *openAPIGenFile {
	f := &openAPIGenFile{
		path: filepath,
		api:  api,
	}

	return f
}

func (f *openAPIGenFile) p(format string, a ...interface{}) {
	f.result += fmt.Sprintf(format+"\n", a...)
}

func (f *openAPIGenFile) write() error {
	return os.WriteFile(f.path, []byte(f.result), 0644)
}

func (f *openAPIGenFile) generateOpenAPI() error {
	o := &openAPISpec{
		Openapi: "3.0.0",
		Info: specInfo{
			Title:   f.api.Title,
			Version: f.api.Version,
		},
		Paths: make(map[string]specEndpoint),
	}

	for _, group := range f.api.EndpointGroups {
		for _, e := range group.endpoints {
			endpointToAdd := specEndpoint{}

			methodToAdd := &specMethod{
				Summary:     e.Name,
				Description: e.Description,
			}

			if e.Response != nil {
				// object case
				t := reflect.TypeOf(e.Response)
				if t.Kind() == reflect.Struct {
					schemaToAdd := schema{
						Schema: schemaProps{
							Type:       "object",
							Properties: make(map[string]propertyInfo),
						},
					}

					for i := 0; i < t.NumField(); i++ {
						field := t.Field(i)
						attributes := strings.Fields(field.Tag.Get("json"))
						if len(attributes) == 0 || attributes[0] == "" {
							panic(t.Name() + " missing json declaration")
						}
						if attributes[0] == "-" {
							continue
						}
						name := attributes[0]
						schemaToAdd.Schema.Properties[name] = propertyInfo{Type: openApiType(field.Type)}
					}

					respCodeContent := responses{
						Num200: code200{
							Description: "success",
							Content: contentType{
								ApplicationJSON: schemaToAdd,
							},
						},
					}

					methodToAdd.Response = respCodeContent
				}
			}

			switch e.Method {
			case http.MethodGet:
				// specMethod specifics
				endpointToAdd.Get = methodToAdd
			case http.MethodPatch:
				endpointToAdd.Patch = methodToAdd
			case http.MethodPost:
				endpointToAdd.Post = methodToAdd
			case http.MethodDelete:
				endpointToAdd.Delete = methodToAdd
			default:
				panic("unhandled method")
			}

			fullpath := filepath.Join("/api", f.api.Version, group.Prefix, e.Path)
			o.Paths[fullpath] = endpointToAdd

			// o.Paths[method.Path] = specEndpoint{
			// 	Get: specMethod{
			// 		OperationID: method.MethodName,
			// 		Summary:     method.Description,
			// 		Response: responses{
			// 			Num200: code200{
			// 				Description: "Get From Somewhere",
			// 				Content:     contentType{},
			// 			},
			// 		},
			// 	},
			// }

		}
	}

	out, err := json.MarshalIndent(o, "", "\t")
	if err != nil {
		return err
	}
	f.result += string(out)

	return nil
}
