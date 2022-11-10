// Copyright (C) 2021 Storj Labs, Inc.
// See LICENSE for copying information.

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zeebo/clingy"
	"github.com/zeebo/errs"
	"golang.org/x/term"
)

type external struct {
	interactive bool // controls if interactive input is allowed
	quic        bool // if set, use the quic transport

	dirs struct {
		loaded  bool   // true if Setup has been called
		current string // current config directory
		legacy  string // old config directory
	}

	migration struct {
		migrated bool  // true if a migration has been attempted
		err      error // any error from the migration attempt
	}

	config struct {
		loaded bool                // true if the existing config file is successfully loaded
		values map[string][]string // the existing configuration
	}

	access struct {
		loaded      bool              // true if we've successfully loaded access.json
		defaultName string            // default access name to use from accesses
		accesses    map[string]string // map of all of the stored accesses
	}

	tracing struct {
		traceID      int64   // if non-zero, sets outgoing traces to the given id
		traceAddress string  // if non-zero, sampled spans are sent to this trace collector address.
		sample       float64 // the chance (number between 0 and 1.0) to send samples to the server.
		verbose      bool    // flag to print out tracing information (like the used trace id)
	}

	events struct {
		address string // if non-zero, events are sent to this address.
	}
}

func newExternal() *external {
	return &external{}
}

func (ex *external) Setup(f clingy.Flags) {
	ex.interactive = f.Flag(
		"interactive", "Controls if interactive input is allowed", true,
		clingy.Transform(strconv.ParseBool), clingy.Boolean,
		clingy.Advanced,
	).(bool)

	ex.quic = f.Flag(
		"quic", "If set, uses the quic transport", false,
		clingy.Transform(strconv.ParseBool), clingy.Boolean,
		clingy.Advanced,
	).(bool)

	ex.dirs.current = f.Flag(
		"config-dir", "Directory that stores the configuration",
		appDir(false, defaultUplinkSubdir()...),
	).(string)

	ex.dirs.legacy = f.Flag(
		"legacy-config-dir", "Directory that stores legacy configuration. Only used during migration",
		appDir(true, defaultUplinkSubdir()...),
		clingy.Advanced,
	).(string)

	ex.tracing.traceID = f.Flag(
		"trace-id", "Specify a trace id manually. This should be globally unique. "+
			"Usually you don't need to set it, and it will be automatically generated.", int64(0),
		clingy.Transform(transformInt64),
		clingy.Advanced,
	).(int64)

	ex.tracing.sample = f.Flag(
		"trace-sample", "The chance (between 0 and 1.0) to report tracing information. Set to 1 to always send it.", float64(0),
		clingy.Transform(transformFloat64),
		clingy.Advanced,
	).(float64)

	ex.tracing.verbose = f.Flag(
		"trace-verbose", "Flag to print out used trace ID", false,
		clingy.Transform(strconv.ParseBool),
		clingy.Advanced,
	).(bool)

	ex.tracing.traceAddress = f.Flag(
		"trace-addr", "Specify where to send traces", "agent.tracing.datasci.storj.io:5775",
		clingy.Advanced,
	).(string)

	ex.events.address = f.Flag(
		"events-addr", "Specify where to send events", "eventkitd.datasci.storj.io:9002",
		clingy.Advanced,
	).(string)

	ex.dirs.loaded = true
}

func transformInt64(x string) (int64, error) {
	return strconv.ParseInt(x, 0, 64)
}

func transformFloat64(x string) (float64, error) {
	return strconv.ParseFloat(x, 64)
}

func (ex *external) AccessInfoFile() string   { return filepath.Join(ex.dirs.current, "access.json") }
func (ex *external) ConfigFile() string       { return filepath.Join(ex.dirs.current, "config.ini") }
func (ex *external) legacyConfigFile() string { return filepath.Join(ex.dirs.legacy, "config.yaml") }

// Dynamic is called by clingy to look up values for global flags not specified on the command
// line. This call lets us fill in values from config files or environment variables.
func (ex *external) Dynamic(name string) (vals []string, err error) {
	key := "UPLINK_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	if val, ok := os.LookupEnv(key); ok {
		return []string{val}, nil
	}

	// if we have not yet loaded the directories, we should not try to migrate
	// and load the current config.
	if !ex.dirs.loaded {
		return nil, nil
	}

	// allow errors from migration and configuration loading so that calls to
	// `uplink setup` can happen and write out a new configuration.
	if err := ex.migrate(); err != nil {
		return nil, nil //nolint
	}
	if err := ex.loadConfig(); err != nil {
		return nil, nil //nolint
	}

	return ex.config.values[name], nil
}

func (ex *external) analyticsEnabled() bool {
	// N.B.: saveInitialConfig prompts the user if they want analytics enabled.
	// In the past, even after prompting for this, we did not write out their
	// answer in the config. Instead, what has historically happened is that
	// if the user said yes, we wrote out an empty config, and if the user
	// said no, we wrote out:
	//
	//     [metrics]
	//     addr =
	//
	// So, if the new value (analytics.enabled) exists at all, we prefer that.
	// Otherwise, we need to check for the existence of metrics.addr and if it
	// is an empty value to determine if analytics are disabled. At some point
	// in the future after enough upgrades have happened, perhaps we can switch
	// to just analytics.enabled. Unfortunately, an entirely empty config file
	// is precisely the config file we've been writing out if a user opts in
	// to analytics, so we are only going to have analytics disabled if (a)
	// analytics.enabled says so, or absent that, if the config file's final
	// specification for metrics.addr is the empty string.
	val, err := ex.Dynamic("analytics.enabled")
	if err != nil {
		return false
	}
	if len(val) > 0 {
		enabled, err := strconv.ParseBool(val[len(val)-1])
		if err != nil {
			return false
		}
		return enabled
	}
	val, err = ex.Dynamic("metrics.addr")
	if err != nil {
		return false
	}
	return len(val) == 0 || val[len(val)-1] != ""
}

// Wrap is called by clingy with the command to be executed.
func (ex *external) Wrap(ctx context.Context, cmd clingy.Command) (err error) {
	if err := ex.migrate(); err != nil {
		return err
	}
	if err := ex.loadConfig(); err != nil {
		return err
	}
	if !ex.config.loaded {
		if err := saveInitialConfig(ctx, ex); err != nil {
			return err
		}
	}

	// N.B.: Tracing is currently disabled by default (sample == 0, traceID == 0) and is
	// something a user can only opt into. as a result, we don't check ex.analyticsEnabled()
	// in this if statement. If we do ever start turning on trace samples by default, we
	// will need to make sure we only do so if ex.analyticsEnabled().
	//if ex.tracing.traceAddress != "" && (ex.tracing.sample > 0 || ex.tracing.traceID > 0) {
	err = initTracer()
	if err != nil {
		return err
	}
	//err = initMeter()
	//if err != nil {
	//	return err
	//}
	//}

	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("uplink")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()
	return cmd.Execute(ctx)
}

func initTracer() error {
	ctx := context.Background()

	traceClient := otlptracegrpc.NewClient(
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint("192.168.1.69:4317"))
	sctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	traceExp, err := otlptrace.New(sctx, traceClient)
	if err != nil {
		return err
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithAttributes(
			// the service name used to display traces in backends
			semconv.ServiceNameKey.String("uplink"),
		),
	)
	if err != nil {
		return err
	}

	bsp := sdktrace.NewBatchSpanProcessor(traceExp)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)

	// set global propagator to tracecontext (the default is no-op).
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	otel.SetTracerProvider(tracerProvider)
	return nil
}

func initMeter() error {
	// The exporter embeds a default OpenTelemetry Reader and
	// implements prometheus.Collector, allowing it to be used as
	// both a Reader and Collector.
	wrappedRegisterer := prometheus.WrapRegistererWithPrefix("storj_", prometheus.NewRegistry())
	exporter, err := otelprom.New(otelprom.WithRegisterer(wrappedRegisterer), otelprom.WithoutUnits())
	if err != nil {
		log.Fatal(err)
	}
	global.SetMeterProvider(metric.NewMeterProvider(metric.WithReader(exporter)))

	// Start the prometheus HTTP server and pass the exporter Collector to it
	go serveMetrics()
	return nil
}

func serveMetrics() {
	log.Printf("serving metrics at localhost:9153/metrics")
	http.Handle("/metrics", promhttp.Handler())
	err := http.ListenAndServe(":9153", nil)
	if err != nil {
		fmt.Printf("error serving http: %v", err)
		return
	}
}

func tracked(ctx context.Context, cb func(context.Context)) (done func()) {
	ctx, cancel := context.WithCancel(ctx)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		cb(ctx)
		wg.Done()
	}()

	return func() {
		cancel()
		wg.Wait()
	}
}

// PromptInput gets a line of input text from the user and returns an error if
// interactive mode is disabled.
func (ex *external) PromptInput(ctx context.Context, prompt string) (input string, err error) {
	if !ex.interactive {
		return "", errs.New("required user input in non-interactive setting")
	}
	fmt.Fprint(clingy.Stdout(ctx), prompt, " ")
	var buf []byte
	var tmp [1]byte
	for {
		_, err := clingy.Stdin(ctx).Read(tmp[:])
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return "", errs.Wrap(err)
		} else if tmp[0] == '\n' {
			break
		}
		buf = append(buf, tmp[0])
	}
	return string(bytes.TrimSpace(buf)), nil
}

// PromptInput gets a line of secret input from the user twice to ensure that
// it is the same value, and returns an error if interactive mode is disabled
// or if the prompt cannot be put into a mode where the typing is not echoed.
func (ex *external) PromptSecret(ctx context.Context, prompt string) (secret string, err error) {
	if !ex.interactive {
		return "", errs.New("required secret input in non-interactive setting")
	}

	fh, ok := clingy.Stdin(ctx).(interface{ Fd() uintptr })
	if !ok {
		return "", errs.New("unable to request secret from stdin")
	}
	fd := int(fh.Fd())

	for {
		fmt.Fprint(clingy.Stdout(ctx), prompt, " ")

		first, err := term.ReadPassword(fd)
		if err != nil {
			return "", errs.New("unable to request secret from stdin: %w", err)
		}
		fmt.Fprintln(clingy.Stdout(ctx))

		fmt.Fprint(clingy.Stdout(ctx), "Again: ")

		second, err := term.ReadPassword(fd)
		if err != nil {
			return "", errs.New("unable to request secret from stdin: %w", err)
		}
		fmt.Fprintln(clingy.Stdout(ctx))

		if string(first) != string(second) {
			fmt.Fprintln(clingy.Stdout(ctx), "Values did not match. Try again.")
			fmt.Fprintln(clingy.Stdout(ctx))
			continue
		}

		return string(first), nil
	}
}

func defaultUplinkSubdir() []string {
	switch runtime.GOOS {
	case "windows", "darwin":
		return []string{"Storj", "Uplink"}
	default:
		return []string{"storj", "uplink"}
	}
}

// appDir returns best base directory for the currently running operating system. It
// has a legacy bool to have it return the same values that storj.io/common/fpath.ApplicationDir
// would have returned.
func appDir(legacy bool, subdir ...string) string {
	var appdir string
	home := os.Getenv("HOME")

	switch runtime.GOOS {
	case "windows":
		// Windows standards: https://msdn.microsoft.com/en-us/library/windows/apps/hh465094.aspx?f=255&MSPPError=-2147217396
		for _, env := range []string{"AppData", "AppDataLocal", "UserProfile", "Home"} {
			val := os.Getenv(env)
			if val != "" {
				appdir = val
				break
			}
		}
	case "darwin":
		// Mac standards: https://developer.apple.com/library/archive/documentation/FileManagement/Conceptual/FileSystemProgrammingGuide/MacOSXDirectories/MacOSXDirectories.html
		appdir = filepath.Join(home, "Library", "Application Support")
	case "linux":
		fallthrough
	default:
		// Linux standards: https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html
		if legacy {
			appdir = os.Getenv("XDG_DATA_HOME")
			if appdir == "" && home != "" {
				appdir = filepath.Join(home, ".local", "share")
			}
		} else {
			appdir = os.Getenv("XDG_CONFIG_HOME")
			if appdir == "" && home != "" {
				appdir = filepath.Join(home, ".config")
			}
		}
	}
	return filepath.Join(append([]string{appdir}, subdir...)...)
}
