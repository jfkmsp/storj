// Copyright (C) 2022 Storj Labs, Inc.
// See LICENSE for copying information.

package consoleapi

import (
	"encoding/json"
	"go.opentelemetry.io/otel"
	"net/http"
	"os"

	"runtime"

	"github.com/gorilla/mux"
	"github.com/zeebo/errs"
	"go.uber.org/zap"

	"storj.io/storj/satellite/abtesting"
	"storj.io/storj/satellite/console"
)

// ErrABAPI - console ab testing api error type.
var ErrABAPI = errs.Class("consoleapi ab testing error")

// ABTesting is an api controller that exposes all ab testing functionality.
type ABTesting struct {
	log     *zap.Logger
	service abtesting.ABService
}

// NewABTesting is a constructor for AB testing controller.
func NewABTesting(log *zap.Logger, service abtesting.ABService) *ABTesting {
	return &ABTesting{
		log:     log,
		service: service,
	}
}

// GetABValues gets AB test values for a specific user.
func (a *ABTesting) GetABValues(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var err error
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	user, err := console.GetUser(ctx)
	if err != nil {
		ServeJSONError(a.log, w, http.StatusUnauthorized, err)
		return
	}

	values, err := a.service.GetABValues(ctx, *user)
	if err != nil {
		ServeJSONError(a.log, w, http.StatusInternalServerError, err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(values)
	if err != nil {
		a.log.Error("Could not encode AB values", zap.Error(ErrABAPI.Wrap(err)))
	}
}

// SendHit sends an event to flagship.
func (a *ABTesting) SendHit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var err error
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	action := mux.Vars(r)["action"]
	if action == "" {
		ServeJSONError(a.log, w, http.StatusBadRequest, errs.New("parameter 'action' can't be empty"))
		return
	}

	user, err := console.GetUser(ctx)
	if err != nil {
		ServeJSONError(a.log, w, http.StatusUnauthorized, err)
		return
	}

	a.service.SendHit(ctx, *user, action)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(map[string]string{
		"message": "Upgrade hit acknowledged",
	})
	if err != nil {
		a.log.Error("failed to write json error response", zap.Error(ErrABAPI.Wrap(err)))
	}
}
