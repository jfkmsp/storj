// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package certificateclient

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"os"

	"runtime"
	"time"

	"github.com/zeebo/errs"

	"storj.io/common/identity"
	"storj.io/common/peertls/tlsopts"
	"storj.io/common/rpc"
	"storj.io/storj/certificate/certificatepb"
)

// Config is a config struct for use with a certificate signing service client.
type Config struct {
	Address string `help:"address of the certificate signing rpc service"`
	TLS     tlsopts.Config
}

// Client implements certificatepb.DRPCCertificatesClient.
type Client struct {
	conn   *rpc.Conn
	client certificatepb.DRPCCertificatesClient
}

// New creates a new certificate signing rpc client.
func New(ctx context.Context, dialer rpc.Dialer, address string) (_ *Client, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name(), trace.WithAttributes(attribute.String("address", address)))
	defer span.End()

	conn, err := dialer.DialAddressInsecure(ctx, address)
	if err != nil {
		return nil, err
	}

	return &Client{
		conn:   conn,
		client: certificatepb.NewDRPCCertificatesClient(conn),
	}, nil
}

// NewClientFrom creates a new certificate signing client from an existing
// cert signing client.
func NewClientFrom(client certificatepb.DRPCCertificatesClient) *Client {
	return &Client{
		client: client,
	}
}

// Sign submits a certificate signing request given the config.
func (config Config) Sign(ctx context.Context, ident *identity.FullIdentity, authToken string) (_ [][]byte, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	tlsOptions, err := tlsopts.NewOptions(ident, config.TLS, nil)
	if err != nil {
		return nil, err
	}
	client, err := New(ctx, rpc.NewDefaultDialer(tlsOptions), config.Address)
	if err != nil {
		return nil, err
	}
	defer func() { err = errs.Combine(err, client.Close()) }()

	return client.Sign(ctx, authToken)
}

// Sign claims an authorization using the token string and returns a signed
// copy of the client's CA certificate.
func (client *Client) Sign(ctx context.Context, tokenStr string) (_ [][]byte, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	res, err := client.client.Sign(ctx, &certificatepb.SigningRequest{
		AuthToken: tokenStr,
		Timestamp: time.Now().Unix(),
	})
	if err != nil {
		return nil, err
	}

	return res.Chain, nil
}

// Close closes the client.
func (client *Client) Close() error {
	if client.conn != nil {
		return client.conn.Close()
	}
	return nil
}
