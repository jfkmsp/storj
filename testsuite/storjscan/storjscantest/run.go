// Copyright (C) 2022 Storj Labs, Inc.
// See LICENSE for copying information.

package storjscantest

import (
	"context"
	"go.uber.org/zap"
	"storj.io/common/grant"
	"storj.io/common/storj"
	"storj.io/common/testcontext"
	"storj.io/private/dbutil/pgtest"
	"storj.io/storj/private/blockchain"
	"storj.io/storj/private/testplanet"
	"storj.io/storj/satellite/satellitedb/satellitedbtest"
	"storj.io/storjscan"
	"storj.io/storjscan/private/testeth"
	"storj.io/uplink"
	"testing"
)

// Stack contains references to storjscan app and eth test network.
type Stack struct {
	Log      *zap.Logger
	App      *storjscan.App
	StartApp func() error
	CloseApp func() error
	Network  *testeth.Network
	Token    blockchain.Address
}

// Test defines common services for storjscan tests.
type Test func(t *testing.T, ctx *testcontext.Context, planet *testplanet.Planet, stack *Stack)

// Run runs testplanet and storjscan and executes test function.
func Run(t *testing.T, test Test) {
	databases := satellitedbtest.Databases()
	if len(databases) == 0 {
		t.Fatal("Databases flag missing, set at least one:\n" +
			"-postgres-test-db=" + pgtest.DefaultPostgres + "\n" +
			"-cockroach-test-db=" + pgtest.DefaultCockroach)
	}

	config := testplanet.Config{
		SatelliteCount: 1, StorageNodeCount: 4, UplinkCount: 1,
		NonParallel: true,
	}

	for _, satelliteDB := range databases {
		satelliteDB := satelliteDB
		t.Run(satelliteDB.Name, func(t *testing.T) {
			parallel := !config.NonParallel
			if parallel {
				t.Parallel()
			}

			if satelliteDB.MasterDB.URL == "" {
				t.Skipf("Database %s connection string not provided. %s", satelliteDB.MasterDB.Name, satelliteDB.MasterDB.Message)
			}
			planetConfig := config
			if planetConfig.Name == "" {
				planetConfig.Name = t.Name()
			}

			_ = testplanet.NewLogger(t)

		})
	}
}

func provisionUplinks(ctx context.Context, t *testing.T, planet *testplanet.Planet) {
	for _, planetUplink := range planet.Uplinks {
		for _, satellite := range planet.Satellites {
			apiKey := planetUplink.APIKey[satellite.ID()]

			// create access grant manually to avoid dialing satellite for
			// project id and deriving key with argon2.IDKey method
			encAccess := grant.NewEncryptionAccessWithDefaultKey(&storj.Key{})
			encAccess.SetDefaultPathCipher(storj.EncAESGCM)

			grantAccess := grant.Access{
				SatelliteAddress: satellite.URL(),
				APIKey:           apiKey,
				EncAccess:        encAccess,
			}

			serializedAccess, err := grantAccess.Serialize()
			if err != nil {
				t.Fatalf("%+v", err)
			}
			access, err := uplink.ParseAccess(serializedAccess)
			if err != nil {
				t.Fatalf("%+v", err)
			}

			planetUplink.Access[satellite.ID()] = access
		}
	}
}
