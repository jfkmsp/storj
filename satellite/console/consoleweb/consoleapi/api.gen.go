// AUTOGENERATED BY private/apigen
// DO NOT EDIT.

package consoleapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/zeebo/errs"
	"go.uber.org/zap"

	"storj.io/common/uuid"
	"storj.io/storj/private/api"
	"storj.io/storj/satellite/accounting"
	"storj.io/storj/satellite/console"
)

var ErrProjectsAPI = errs.Class("consoleapi projects api")

type ProjectManagementService interface {
	GenGetUsersProjects(context.Context) ([]console.Project, api.HTTPError)
	GenGetSingleBucketUsageRollup(context.Context, uuid.UUID, string, time.Time, time.Time) (*accounting.BucketUsageRollup, api.HTTPError)
	GenGetBucketUsageRollups(context.Context, uuid.UUID, time.Time, time.Time) ([]accounting.BucketUsageRollup, api.HTTPError)
}

// Handler is an api handler that exposes all projects related functionality.
type Handler struct {
	log     *zap.Logger
	service ProjectManagementService
	auth    api.Auth
}

func NewProjectManagement(log *zap.Logger, service ProjectManagementService, router *mux.Router, auth api.Auth) *Handler {
	handler := &Handler{
		log:     log,
		service: service,
		auth:    auth,
	}

	projectsRouter := router.PathPrefix("/api/v0/projects").Subrouter()
	projectsRouter.HandleFunc("/", handler.handleGenGetUsersProjects).Methods("GET")
	projectsRouter.HandleFunc("/bucket-rollup", handler.handleGenGetSingleBucketUsageRollup).Methods("GET")
	projectsRouter.HandleFunc("/bucket-rollups", handler.handleGenGetBucketUsageRollups).Methods("GET")

	return handler
}

func (h *Handler) handleGenGetUsersProjects(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var err error
	defer mon.Task()(&ctx)(&err)

	w.Header().Set("Content-Type", "application/json")

	ctx, err = h.auth.IsAuthenticated(ctx, r, true, true)
	if err != nil {
		api.ServeError(h.log, w, http.StatusUnauthorized, err)
		return
	}

	retVal, httpErr := h.service.GenGetUsersProjects(ctx)
	if httpErr.Err != nil {
		api.ServeError(h.log, w, httpErr.Status, httpErr.Err)
		return
	}

	err = json.NewEncoder(w).Encode(retVal)
	if err != nil {
		h.log.Debug("failed to write json GenGetUsersProjects response", zap.Error(ErrProjectsAPI.Wrap(err)))
	}
}

func (h *Handler) handleGenGetSingleBucketUsageRollup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var err error
	defer mon.Task()(&ctx)(&err)

	w.Header().Set("Content-Type", "application/json")

	ctx, err = h.auth.IsAuthenticated(ctx, r, true, true)
	if err != nil {
		api.ServeError(h.log, w, http.StatusUnauthorized, err)
		return
	}

	projectID, err := uuid.FromString(r.URL.Query().Get("projectID"))
	if err != nil {
		api.ServeError(h.log, w, http.StatusBadRequest, err)
		return
	}

	bucket := r.URL.Query().Get("bucket")
	if bucket == "" {
		api.ServeError(h.log, w, http.StatusBadRequest, errs.New("parameter 'bucket' can't be empty"))
		return
	}

	sinceStamp, err := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	if err != nil {
		api.ServeError(h.log, w, http.StatusBadRequest, err)
		return
	}

	since := time.Unix(sinceStamp, 0).UTC()

	beforeStamp, err := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	if err != nil {
		api.ServeError(h.log, w, http.StatusBadRequest, err)
		return
	}

	before := time.Unix(beforeStamp, 0).UTC()

	retVal, httpErr := h.service.GenGetSingleBucketUsageRollup(ctx, projectID, bucket, since, before)
	if httpErr.Err != nil {
		api.ServeError(h.log, w, httpErr.Status, httpErr.Err)
		return
	}

	err = json.NewEncoder(w).Encode(retVal)
	if err != nil {
		h.log.Debug("failed to write json GenGetSingleBucketUsageRollup response", zap.Error(ErrProjectsAPI.Wrap(err)))
	}
}

func (h *Handler) handleGenGetBucketUsageRollups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var err error
	defer mon.Task()(&ctx)(&err)

	w.Header().Set("Content-Type", "application/json")

	ctx, err = h.auth.IsAuthenticated(ctx, r, true, true)
	if err != nil {
		api.ServeError(h.log, w, http.StatusUnauthorized, err)
		return
	}

	projectID, err := uuid.FromString(r.URL.Query().Get("projectID"))
	if err != nil {
		api.ServeError(h.log, w, http.StatusBadRequest, err)
		return
	}

	sinceStamp, err := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	if err != nil {
		api.ServeError(h.log, w, http.StatusBadRequest, err)
		return
	}

	since := time.Unix(sinceStamp, 0).UTC()

	beforeStamp, err := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	if err != nil {
		api.ServeError(h.log, w, http.StatusBadRequest, err)
		return
	}

	before := time.Unix(beforeStamp, 0).UTC()

	retVal, httpErr := h.service.GenGetBucketUsageRollups(ctx, projectID, since, before)
	if httpErr.Err != nil {
		api.ServeError(h.log, w, httpErr.Status, httpErr.Err)
		return
	}

	err = json.NewEncoder(w).Encode(retVal)
	if err != nil {
		h.log.Debug("failed to write json GenGetBucketUsageRollups response", zap.Error(ErrProjectsAPI.Wrap(err)))
	}
}
