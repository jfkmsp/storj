// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package satellitedb

import (
	"context"
	"go.opentelemetry.io/otel"
	"os"
	"time"

	"runtime"

	"github.com/zeebo/errs"
	"go.uber.org/zap"

	"storj.io/common/storj"
	"storj.io/common/uuid"
	"storj.io/private/dbutil/pgutil"
	"storj.io/storj/satellite/nodeevents"
	"storj.io/storj/satellite/satellitedb/dbx"
)

var _ nodeevents.DB = (*nodeEvents)(nil)

type nodeEvents struct {
	db *satelliteDB
}

// Insert a node event into the node events table.
func (ne *nodeEvents) Insert(ctx context.Context, email string, nodeID storj.NodeID, eventType nodeevents.Type) (nodeEvent nodeevents.NodeEvent, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	id, err := uuid.New()
	if err != nil {
		return nodeEvent, err
	}
	name, err := eventType.Name()
	if err != nil {
		return nodeEvent, err
	}
	entry, err := ne.db.Create_NodeEvent(ctx, dbx.NodeEvent_Id(id.Bytes()), dbx.NodeEvent_Email(email), dbx.NodeEvent_NodeId(nodeID.Bytes()), dbx.NodeEvent_Event(int(eventType)), dbx.NodeEvent_Create_Fields{})
	if err != nil {
		return nodeEvent, err
	}

	ne.db.log.Info("node event inserted", zap.String("name", name), zap.String("email", email), zap.String("node ID", nodeID.String()))

	return fromDBX(entry)
}

// GetLatestByEmailAndEvent gets latest node event by email and event type.
func (ne *nodeEvents) GetLatestByEmailAndEvent(ctx context.Context, email string, event nodeevents.Type) (nodeEvent nodeevents.NodeEvent, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	dbxNE, err := ne.db.First_NodeEvent_By_Email_And_Event_OrderBy_Desc_CreatedAt(ctx, dbx.NodeEvent_Email(email), dbx.NodeEvent_Event(int(event)))
	if err != nil {
		return nodeEvent, err
	}
	return fromDBX(dbxNE)
}

// GetNextBatch gets the next batch of events to combine into an email.
// It selects one item that was inserted before 'firstSeenBefore', then selects
// all entries with the same email and event so that they can be combined into a
// single email.
func (ne *nodeEvents) GetNextBatch(ctx context.Context, firstSeenBefore time.Time) (events []nodeevents.NodeEvent, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	rows, err := ne.db.QueryContext(ctx, `
		SELECT node_events.id, node_events.email, node_events.node_id, node_events.event
		FROM node_events
		INNER JOIN (
			SELECT email, event
			FROM node_events
			WHERE created_at < $1
				AND email_sent is NULL
			ORDER BY created_at ASC
			LIMIT 1
		) as t
		ON node_events.email = t.email
			AND node_events.event = t.event
		WHERE node_events.email_sent IS NULL
	`, firstSeenBefore)
	if err != nil {
		return nil, err
	}
	defer func() { err = errs.Combine(err, rows.Close()) }()

	for rows.Next() {
		var idBytes []byte
		var email string
		var nodeIDBytes []byte
		var event int
		err = rows.Scan(&idBytes, &email, &nodeIDBytes, &event)
		if err != nil {
			return nil, err
		}
		id, err := uuid.FromBytes(idBytes)
		if err != nil {
			return nil, err
		}
		nodeID, err := storj.NodeIDFromBytes(nodeIDBytes)
		if err != nil {
			return nil, err
		}
		events = append(events, nodeevents.NodeEvent{
			ID:     id,
			Email:  email,
			NodeID: nodeID,
			Event:  nodeevents.Type(event),
		})
	}

	return events, rows.Err()
}

func fromDBX(dbxNE *dbx.NodeEvent) (event nodeevents.NodeEvent, err error) {
	id, err := uuid.FromBytes(dbxNE.Id)
	if err != nil {
		return event, err
	}
	nodeID, err := storj.NodeIDFromBytes(dbxNE.NodeId)
	if err != nil {
		return event, err
	}
	return nodeevents.NodeEvent{
		ID:        id,
		Email:     dbxNE.Email,
		NodeID:    nodeID,
		Event:     nodeevents.Type(dbxNE.Event),
		CreatedAt: dbxNE.CreatedAt,
		EmailSent: dbxNE.EmailSent,
	}, nil
}

// UpdateEmailSent updates email_sent for a group of rows.
func (ne *nodeEvents) UpdateEmailSent(ctx context.Context, ids []uuid.UUID, timestamp time.Time) (err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	_, err = ne.db.ExecContext(ctx, `
		UPDATE node_events SET email_sent = $1
		WHERE id = ANY($2::bytea[])
	`, timestamp, pgutil.UUIDArray(ids))
	return err
}
