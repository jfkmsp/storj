// Copyright (C) 2022 Storj Labs, Inc.
// See LICENSE for copying information.

package apigen

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/zeebo/errs"
)

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
	Get specMethod `json:"get"`
}

type specMethod struct {
	OperationID string    `json:"operationId"`
	Summary     string    `json:"summary"`
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
	ApplicationJSON appjson `json:"application/json"`
}

type appjson struct {
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
		for _, method := range group.endpoints {

			o.Paths[method.Path] = specEndpoint{
				Get: specMethod{
					OperationID: method.MethodName,
					Summary:     method.Description,
					Response: responses{
						Num200: code200{
							Description: "Get From Somewhere",
							Content:     contentType{},
						},
					},
				},
			}

		}
	}

	out, err := json.MarshalIndent(o, "", "\t")
	if err != nil {
		return err
	}
	f.result += string(out)

	return nil
}
