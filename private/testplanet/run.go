// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information

package testplanet

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"storj.io/common/grant"
	"storj.io/common/storj"
	"storj.io/common/testcontext"
	"storj.io/private/dbutil/pgtest"
	"storj.io/storj/satellite/satellitedb/satellitedbtest"
	"storj.io/uplink"
)

// Run runs testplanet in multiple configurations.
func Run(t *testing.T, config Config, test func(t *testing.T, ctx *testcontext.Context, planet *Planet)) {
	databases := satellitedbtest.Databases()
	if len(databases) == 0 {
		t.Fatal("Databases flag missing, set at least one:\n" +
			"-postgres-test-db=" + pgtest.DefaultPostgres + "\n" +
			"-cockroach-test-db=" + pgtest.DefaultCockroach)
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

			_ = NewLogger(t)
		})
	}
}

// Bench makes benchmark with testplanet as easy as running unit tests with Run method.
func Bench(b *testing.B, config Config, bench func(b *testing.B, ctx *testcontext.Context, planet *Planet)) {
	databases := satellitedbtest.Databases()
	if len(databases) == 0 {
		b.Fatal("Databases flag missing, set at least one:\n" +
			"-postgres-test-db=" + pgtest.DefaultPostgres + "\n" +
			"-cockroach-test-db=" + pgtest.DefaultCockroach)
	}

	for _, satelliteDB := range databases {
		satelliteDB := satelliteDB
		b.Run(satelliteDB.Name, func(b *testing.B) {
			if satelliteDB.MasterDB.URL == "" {
				b.Skipf("Database %s connection string not provided. %s", satelliteDB.MasterDB.Name, satelliteDB.MasterDB.Message)
			}

			_ = zap.NewNop()

			planetConfig := config
			if planetConfig.Name == "" {
				planetConfig.Name = b.Name()
			}
		})
	}
}

func provisionUplinks(ctx context.Context, t testing.TB, planet *Planet) {
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
