// Copyright (C) 2022 Storj Labs, Inc.
// See LICENSE for copying information.

package main

//go:generate go run ./

import (
	"time"

	"storj.io/common/uuid"
	"storj.io/storj/private/apigen"
	"storj.io/storj/satellite/accounting"
	"storj.io/storj/satellite/console"
)

func main() {
	// definition for REST API
	a := &apigen.API{
		Title:       "OpenAPI Test",
		Version:     "v0",
		Description: "Make it meaningful",
		PackageName: "consoleapi",
	}

	{
		g := a.Group("ProjectManagement", "projects")

		g.Post("/create", &apigen.Endpoint{
			Name:        "Create new Project",
			Description: "Creates new Project with given info",
			MethodName:  "GenCreateProject",
			RequestName: "createProject",
			Response:    console.Project{},
			Request:     console.ProjectInfo{},
		})

		g.Patch("/update/{id}", &apigen.Endpoint{
			Name:        "Update Project",
			Description: "Updates project with given info",
			MethodName:  "GenUpdateProject",
			RequestName: "updateProject",
			Response:    console.Project{},
			Request:     console.ProjectInfo{},
			PathParams: []apigen.Param{
				apigen.NewParam("id", uuid.UUID{}),
			},
		})

		g.Delete("/delete/{id}", &apigen.Endpoint{
			Name:        "Delete Project",
			Description: "Deletes project by id",
			MethodName:  "GenDeleteProject",
			RequestName: "deleteProject",
			PathParams: []apigen.Param{
				apigen.NewParam("id", uuid.UUID{}),
			},
		})

		g.Get("/", &apigen.Endpoint{
			Name:        "Get Projects",
			Description: "Gets all projects user has",
			MethodName:  "GenGetUsersProjects",
			RequestName: "getProjects",
			Response:    []console.Project{},
		})

		g.Get("/bucket-rollup", &apigen.Endpoint{
			Name:        "Get Project's Single Bucket Usage",
			Description: "Gets project's single bucket usage by bucket ID",
			MethodName:  "GenGetSingleBucketUsageRollup",
			RequestName: "getBucketRollup",
			Response:    accounting.BucketUsageRollup{},
			QueryParams: []apigen.Param{
				apigen.NewParam("projectID", uuid.UUID{}),
				apigen.NewParam("bucket", ""),
				apigen.NewParam("since", time.Time{}),
				apigen.NewParam("before", time.Time{}),
			},
		})

		g.Get("/bucket-rollups", &apigen.Endpoint{
			Name:        "Get Project's All Buckets Usage",
			Description: "Gets project's all buckets usage",
			MethodName:  "GenGetBucketUsageRollups",
			RequestName: "getBucketRollups",
			Response:    []accounting.BucketUsageRollup{},
			QueryParams: []apigen.Param{
				apigen.NewParam("projectID", uuid.UUID{}),
				apigen.NewParam("since", time.Time{}),
				apigen.NewParam("before", time.Time{}),
			},
		})
	}

	{
		g := a.Group("APIKeyManagement", "apikeys")

		g.Post("/create", &apigen.Endpoint{
			Name:        "Create new macaroon API key",
			Description: "Creates new macaroon API key with given info",
			MethodName:  "GenCreateAPIKey",
			RequestName: "createAPIKey",
			Response:    console.CreateAPIKeyResponse{},
			Request:     console.CreateAPIKeyRequest{},
		})
	}

	{
		g := a.Group("UserManagement", "users")

		g.Get("/", &apigen.Endpoint{
			Name:        "Get User",
			Description: "Gets User by request context",
			MethodName:  "GenGetUser",
			RequestName: "getUser",
			Response:    console.ResponseUser{},
		})
	}

	a.MustWriteGo("satellite/console/consoleweb/consoleapi/api.gen.go")
	a.MustWriteTS("web/satellite/src/api/testts.gen.ts")
	a.MustWriteOpenAPI("openapi.json")
}
