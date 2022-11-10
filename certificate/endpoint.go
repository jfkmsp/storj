// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package certificate

import (
	"context"
	"go.opentelemetry.io/otel"
	"os"

	"runtime"

	"go.uber.org/zap"

	"storj.io/common/identity"
	"storj.io/common/rpc/rpcpeer"
	"storj.io/common/rpc/rpcstatus"
	"storj.io/storj/certificate/authorization"
	"storj.io/storj/certificate/certificatepb"
	"storj.io/storj/certificate/rpcerrs"
)

// Endpoint implements pb.CertificatesServer.
type Endpoint struct {
	certificatepb.DRPCCertificatesUnimplementedServer

	rpclog          *rpcerrs.Log
	log             *zap.Logger
	ca              *identity.FullCertificateAuthority
	authorizationDB *authorization.DB
	minDifficulty   uint16
}

// NewEndpoint creates a new certificate signing server.
func NewEndpoint(log *zap.Logger, ca *identity.FullCertificateAuthority, authorizationDB *authorization.DB, minDifficulty uint16) *Endpoint {
	rpclog := rpcerrs.NewLog(&Error, log, rpcerrs.StatusMap{
		&authorization.ErrNotFound:       rpcstatus.Unauthenticated,
		&authorization.ErrInvalidClaim:   rpcstatus.InvalidArgument,
		&authorization.ErrInvalidToken:   rpcstatus.InvalidArgument,
		&authorization.ErrAlreadyClaimed: rpcstatus.AlreadyExists,
	})

	return &Endpoint{
		rpclog:          rpclog,
		log:             log,
		ca:              ca,
		authorizationDB: authorizationDB,
		minDifficulty:   minDifficulty,
	}
}

// Sign signs the CA certificate of the remote peer's identity with the `certs.ca` certificate.
// Returns a certificate chain consisting of the remote peer's CA followed by the CA chain.
func (endpoint Endpoint) Sign(ctx context.Context, req *certificatepb.SigningRequest) (_ *certificatepb.SigningResponse, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()
	peer, err := rpcpeer.FromContext(ctx)
	if err != nil {
		msg := "error getting peer from context"
		return nil, endpoint.rpclog.Error(msg, err)
	}

	peerIdent, err := identity.PeerIdentityFromPeer(peer)
	if err != nil {
		msg := "error getting peer identity"
		return nil, endpoint.rpclog.Error(msg, err)
	}

	signedPeerCA, err := endpoint.ca.Sign(peerIdent.CA)
	if err != nil {
		msg := "error signing peer CA"
		return nil, endpoint.rpclog.Error(msg, err)
	}

	signedChainBytes := [][]byte{signedPeerCA.Raw, endpoint.ca.Cert.Raw}
	signedChainBytes = append(signedChainBytes, endpoint.ca.RawRestChain()...)
	err = endpoint.authorizationDB.Claim(ctx, &authorization.ClaimOpts{
		Req:           req,
		Peer:          peer,
		ChainBytes:    signedChainBytes,
		MinDifficulty: endpoint.minDifficulty,
	})
	if err != nil {
		msg := "error claiming authorization"
		return nil, endpoint.rpclog.Error(msg, err)
	}

	difficulty, err := peerIdent.ID.Difficulty()
	if err != nil {
		msg := "error checking difficulty"
		return nil, endpoint.rpclog.Error(msg, err)
	}
	token, err := authorization.ParseToken(req.AuthToken)
	if err != nil {
		msg := "error parsing auth token"
		return nil, endpoint.rpclog.Error(msg, err)
	}
	tokenFormatter := authorization.Authorization{
		Token: *token,
	}
	endpoint.log.Info("certificate successfully signed",
		zap.Stringer("Node ID", peerIdent.ID),
		zap.Uint16("difficulty", difficulty),
		zap.Stringer("truncated token", tokenFormatter),
	)

	return &certificatepb.SigningResponse{
		Chain: signedChainBytes,
	}, nil
}
