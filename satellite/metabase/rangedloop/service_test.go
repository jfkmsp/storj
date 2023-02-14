// Copyright (C) 2022 Storj Labs, Inc.
// See LICENSE for copying information.

package rangedloop_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"storj.io/common/memory"
	"storj.io/common/testcontext"
	"storj.io/common/testrand"
	"storj.io/common/uuid"
	"storj.io/storj/private/testplanet"
	"storj.io/storj/satellite/accounting/nodetally"
	"storj.io/storj/satellite/audit"
	"storj.io/storj/satellite/gc/bloomfilter"
	"storj.io/storj/satellite/gracefulexit"
	"storj.io/storj/satellite/metabase/rangedloop"
	"storj.io/storj/satellite/metabase/rangedloop/rangedlooptest"
	"storj.io/storj/satellite/metabase/segmentloop"
	"storj.io/storj/satellite/metrics"
	"storj.io/storj/satellite/repair/checker"
)

func TestLoopCount(t *testing.T) {
	for _, parallelism := range []int{1, 2, 3} {
		for _, nSegments := range []int{0, 1, 2, 11} {
			for _, nObservers := range []int{0, 1, 2} {
				t.Run(
					fmt.Sprintf("par%d_seg%d_obs%d", parallelism, nSegments, nObservers),
					func(t *testing.T) {
						runCountTest(t, parallelism, nSegments, nObservers)
					},
				)
			}
		}
	}
}

func runCountTest(t *testing.T, parallelism int, nSegments int, nObservers int) {
	batchSize := 2
	ctx := testcontext.New(t)

	observers := []rangedloop.Observer{}
	for i := 0; i < nObservers; i++ {
		observers = append(observers, &rangedlooptest.CountObserver{})
	}

	loopService := rangedloop.NewService(
		zaptest.NewLogger(t),
		rangedloop.Config{
			BatchSize:   batchSize,
			Parallelism: parallelism,
		},
		&rangedlooptest.RangeSplitter{
			Segments: make([]segmentloop.Segment, nSegments),
		},
		observers,
	)

	observerDurations, err := loopService.RunOnce(ctx)
	require.NoError(t, err)
	require.Len(t, observerDurations, nObservers)

	for _, observer := range observers {
		countObserver := observer.(*rangedlooptest.CountObserver)
		require.Equal(t, nSegments, countObserver.NumSegments)
	}
}

func TestLoopDuration(t *testing.T) {
	t.Skip("Flaky test because it validates concurrency by measuring time")

	nSegments := 8
	nObservers := 2
	parallelism := 4
	batchSize := 2
	sleepIncrement := time.Millisecond * 10

	ctx := testcontext.New(t)

	observers := []rangedloop.Observer{}
	for i := 0; i < nObservers; i++ {
		observers = append(observers, &rangedlooptest.SleepObserver{
			Duration: sleepIncrement,
		})
	}

	segments := []segmentloop.Segment{}
	for i := 0; i < nSegments; i++ {
		streamId, err := uuid.FromBytes([]byte{byte(i), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
		require.NoError(t, err)
		segments = append(segments, segmentloop.Segment{
			StreamID: streamId,
		})
	}

	loopService := rangedloop.NewService(
		zaptest.NewLogger(t),
		rangedloop.Config{
			BatchSize:   batchSize,
			Parallelism: parallelism,
		},
		&rangedlooptest.RangeSplitter{
			Segments: segments,
		},
		observers,
	)

	start := time.Now()
	observerDurations, err := loopService.RunOnce(ctx)
	require.NoError(t, err)

	duration := time.Since(start)
	expectedDuration := time.Duration(int64(nSegments) * int64(sleepIncrement) * int64(nObservers) / int64(parallelism))
	require.Equal(t, expectedDuration, duration.Truncate(sleepIncrement))

	require.Len(t, observerDurations, nObservers)
	for _, observerDuration := range observerDurations {
		expectedSleep := time.Duration(int64(nSegments) * int64(sleepIncrement))
		require.Equal(t, expectedSleep, observerDuration.Duration.Round(sleepIncrement))
	}
}

func TestLoopCancellation(t *testing.T) {
	parallelism := 2
	batchSize := 1
	ctx, cancel := context.WithCancel(testcontext.NewWithTimeout(t, time.Second*10))

	observers := []rangedloop.Observer{
		&rangedlooptest.CountObserver{},
		&rangedlooptest.CallbackObserver{
			OnProcess: func(ctx context.Context, segments []segmentloop.Segment) error {
				// cancel from inside the loop, when it is certain that the loop has started
				cancel()
				return nil
			},
		},
	}

	loopService := rangedloop.NewService(
		zaptest.NewLogger(t),
		rangedloop.Config{
			BatchSize:   batchSize,
			Parallelism: parallelism,
		},
		&rangedlooptest.InfiniteSegmentProvider{},
		observers,
	)

	_, err := loopService.RunOnce(ctx)

	require.ErrorIs(t, err, context.Canceled)
}

func TestLoopContinuesAfterObserverError(t *testing.T) {
	parallelism := 2
	batchSize := 1
	segments := make([]segmentloop.Segment, 2)

	numOnStartCalls := 0
	numOnForkCalls := 0
	numOnProcessCalls := int32(0)
	numOnJoinCalls := 0
	numOnFinishCalls := 0

	incNumOnProcessCalls := func() {
		atomic.AddInt32(&numOnProcessCalls, 1)
	}

	// first and last observer emit no error
	// other observers emit an error at different stages
	observers := []rangedloop.Observer{
		&rangedlooptest.CallbackObserver{
			OnStart: func(ctx context.Context, t time.Time) error {
				numOnStartCalls++
				return nil
			},
			OnFork: func(ctx context.Context) (rangedloop.Partial, error) {
				numOnForkCalls++
				return nil, nil
			},
			OnProcess: func(ctx context.Context, segments []segmentloop.Segment) error {
				incNumOnProcessCalls()
				return nil
			},
			OnJoin: func(ctx context.Context, partial rangedloop.Partial) error {
				numOnJoinCalls++
				return nil
			},
			OnFinish: func(ctx context.Context) error {
				numOnFinishCalls++
				return nil
			},
		},
		&rangedlooptest.CallbackObserver{
			OnStart: func(ctx context.Context, t time.Time) error {
				numOnStartCalls++
				return errors.New("Test OnStart error")
			},
			OnFork: func(ctx context.Context) (rangedloop.Partial, error) {
				require.Fail(t, "OnFork should not be called")
				return nil, nil
			},
			OnProcess: func(ctx context.Context, segments []segmentloop.Segment) error {
				require.Fail(t, "OnProcess should not be called")
				return nil
			},
			OnJoin: func(ctx context.Context, partial rangedloop.Partial) error {
				require.Fail(t, "OnJoin should not be called")
				return nil
			},
			OnFinish: func(ctx context.Context) error {
				require.Fail(t, "OnFinish should not be called")
				return nil
			},
		},
		&rangedlooptest.CallbackObserver{
			OnStart: func(ctx context.Context, t time.Time) error {
				numOnStartCalls++
				return nil
			},
			OnFork: func(ctx context.Context) (rangedloop.Partial, error) {
				numOnForkCalls++
				return nil, errors.New("Test OnFork error")
			},
			OnProcess: func(ctx context.Context, segments []segmentloop.Segment) error {
				require.Fail(t, "OnProcess should not be called")
				return nil
			},
			OnJoin: func(ctx context.Context, partial rangedloop.Partial) error {
				require.Fail(t, "OnJoin should not be called")
				return nil
			},
			OnFinish: func(ctx context.Context) error {
				require.Fail(t, "OnFinish should not be called")
				return nil
			},
		},
		&rangedlooptest.CallbackObserver{
			OnStart: func(ctx context.Context, t time.Time) error {
				numOnStartCalls++
				return nil
			},
			OnFork: func(ctx context.Context) (rangedloop.Partial, error) {
				numOnForkCalls++
				return nil, nil
			},
			OnProcess: func(ctx context.Context, segments []segmentloop.Segment) error {
				incNumOnProcessCalls()
				return errors.New("Test OnProcess error")
			},
			OnJoin: func(ctx context.Context, partial rangedloop.Partial) error {
				require.Fail(t, "OnJoin should not be called")
				return nil
			},
			OnFinish: func(ctx context.Context) error {
				require.Fail(t, "OnFinish should not be called")
				return nil
			},
		},
		&rangedlooptest.CallbackObserver{
			OnStart: func(ctx context.Context, t time.Time) error {
				numOnStartCalls++
				return nil
			},
			OnFork: func(ctx context.Context) (rangedloop.Partial, error) {
				numOnForkCalls++
				return nil, nil
			},
			OnProcess: func(ctx context.Context, segments []segmentloop.Segment) error {
				incNumOnProcessCalls()
				return nil
			},
			OnJoin: func(ctx context.Context, partial rangedloop.Partial) error {
				numOnJoinCalls++
				return errors.New("Test OnJoin error")
			},
			OnFinish: func(ctx context.Context) error {
				require.Fail(t, "OnFinish should not be called")
				return nil
			},
		},
		&rangedlooptest.CallbackObserver{
			OnStart: func(ctx context.Context, t time.Time) error {
				numOnStartCalls++
				return nil
			},
			OnFork: func(ctx context.Context) (rangedloop.Partial, error) {
				numOnForkCalls++
				return nil, nil
			},
			OnProcess: func(ctx context.Context, segments []segmentloop.Segment) error {
				incNumOnProcessCalls()
				return nil
			},
			OnJoin: func(ctx context.Context, partial rangedloop.Partial) error {
				numOnJoinCalls++
				return nil
			},
			OnFinish: func(ctx context.Context) error {
				numOnFinishCalls++
				return errors.New("Test OnFinish error")
			},
		},
		&rangedlooptest.CallbackObserver{
			OnStart: func(ctx context.Context, t time.Time) error {
				numOnStartCalls++
				return nil
			},
			OnFork: func(ctx context.Context) (rangedloop.Partial, error) {
				numOnForkCalls++
				return nil, nil
			},
			OnProcess: func(ctx context.Context, segments []segmentloop.Segment) error {
				incNumOnProcessCalls()
				return nil
			},
			OnJoin: func(ctx context.Context, partial rangedloop.Partial) error {
				numOnJoinCalls++
				return nil
			},
			OnFinish: func(ctx context.Context) error {
				numOnFinishCalls++
				return nil
			},
		},
	}

	loopService := rangedloop.NewService(
		zaptest.NewLogger(t),
		rangedloop.Config{
			BatchSize:   batchSize,
			Parallelism: parallelism,
		},
		&rangedlooptest.RangeSplitter{
			Segments: segments,
		},
		observers,
	)

	observerDurations, err := loopService.RunOnce(testcontext.New(t))
	require.NoError(t, err)
	require.Len(t, observerDurations, len(observers))

	require.EqualValues(t, 7, numOnStartCalls)
	require.EqualValues(t, 6*parallelism, numOnForkCalls)
	require.EqualValues(t, 5*parallelism-1, numOnProcessCalls)
	require.EqualValues(t, 4*parallelism-1, numOnJoinCalls)
	require.EqualValues(t, 3, numOnFinishCalls)

	// success observer should have the duration reported
	require.Greater(t, observerDurations[0].Duration, time.Duration(0))
	require.Greater(t, observerDurations[6].Duration, time.Duration(0))

	// error observers should have sentinel duration reported
	require.Equal(t, observerDurations[1].Duration, -1*time.Second)
	require.Equal(t, observerDurations[2].Duration, -1*time.Second)
	require.Equal(t, observerDurations[3].Duration, -1*time.Second)
	require.Equal(t, observerDurations[4].Duration, -1*time.Second)
	require.Equal(t, observerDurations[5].Duration, -1*time.Second)
}

func TestAllInOne(t *testing.T) {
	testplanet.Run(t, testplanet.Config{
		SatelliteCount: 1, StorageNodeCount: 4, UplinkCount: 1,
	}, func(t *testing.T, ctx *testcontext.Context, planet *testplanet.Planet) {
		log := zaptest.NewLogger(t)
		satellite := planet.Satellites[0]

		for i := 0; i < 100; i++ {
			err := planet.Uplinks[0].Upload(ctx, satellite, "testbucket", "object"+strconv.Itoa(i), testrand.Bytes(5*memory.KiB))
			require.NoError(t, err)
		}

		require.NoError(t, planet.Uplinks[0].CreateBucket(ctx, satellite, "bf-bucket"))

		metabaseProvider := rangedloop.NewMetabaseRangeSplitter(satellite.Metabase.DB, 0, 10)

		config := rangedloop.Config{
			Parallelism: 8,
			BatchSize:   3,
		}

		bfConfig := satellite.Config.GarbageCollectionBF
		bfConfig.Bucket = "bf-bucket"
		accessGrant, err := planet.Uplinks[0].Access[satellite.ID()].Serialize()
		require.NoError(t, err)
		bfConfig.AccessGrant = accessGrant

		service := rangedloop.NewService(log, config, metabaseProvider, []rangedloop.Observer{
			rangedloop.NewLiveCountObserver(),
			metrics.NewObserver(),
			nodetally.NewRangedLoopObserver(log.Named("accounting:nodetally"),
				satellite.DB.StoragenodeAccounting(),
			),
			audit.NewObserver(log.Named("audit"),
				satellite.DB.VerifyQueue(),
				satellite.Config.Audit,
			),
			gracefulexit.NewObserver(log.Named("gracefulexit:observer"),
				satellite.DB.GracefulExit(),
				satellite.DB.OverlayCache(),
				satellite.Config.GracefulExit,
			),
			bloomfilter.NewObserver(log.Named("gc-bf"),
				bfConfig,
				satellite.DB.OverlayCache(),
			),
			checker.NewRangedLoopObserver(
				log.Named("repair:checker"),
				satellite.DB.RepairQueue(),
				satellite.Overlay.Service,
				satellite.Config.Checker,
			),
		})

		for i := 0; i < 5; i++ {
			_, err = service.RunOnce(ctx)
			require.NoError(t, err, "iteration %d", i+1)
		}
	})
}
