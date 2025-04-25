package main

import (
	"context"
	_ "embed" // blank import for embed support.
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"

	"code.cloudfoundry.org/go-envstruct"
	"code.cloudfoundry.org/lager/v3"
	"github.com/Infra-Red/geoip-processor/internal/server"
	"github.com/oschwald/geoip2-golang"
	"google.golang.org/grpc"
)

var (
	//go:embed GeoIP2-Country.mmdb
	geoIpCountryDatabase []byte
	//go:embed GeoIP2-City.mmdb
	geoIpCityDatabase []byte
)

type config struct {
	Addr                string   `env:"ADDR,                   report"`
	BlockedCountryCodes []string `env:"BLOCKED_COUNTRY_CODES,  report"` // TODO: also support subdivisions
	MaxConStreams       int      `env:"MAX_CONCURRENT_STREAMS, report"`
	// non-env
	blockedCountryCodesLookupMap map[string]struct{}
	logger                       lager.Logger
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logger := lager.NewLogger("geoip-processor")
	logLevelFromEnvString, ok := os.LookupEnv("LOG_LEVEL")
	if !ok {
		logger.RegisterSink(lager.NewPrettySink(os.Stdout, lager.INFO)) // default to INFO level
	} else {
		logLevel, err := lager.LogLevelFromString(logLevelFromEnvString)
		if err != nil {
			panic(err)
		}
		logger.RegisterSink(lager.NewPrettySink(os.Stdout, logLevel))
	}

	cfg, err := getConfig()
	if err != nil {
		logger.Fatal("load-config", err)
	}
	cfg.logger = logger

	if err := run(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
		cfg.logger.Fatal("error-running-server", err)
	}
}

func run(ctx context.Context, cfg *config) error {
	db, err := geoip2.FromBytes(geoIpCountryDatabase)
	if err != nil {
		return err
	}
	defer db.Close()

	lis, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return err
	}

	opts := []grpc.ServerOption{grpc.MaxConcurrentStreams(uint32(cfg.MaxConStreams))}
	s := grpc.NewServer(opts...)

	srv := server.NewServer(cfg.logger, db, cfg.blockedCountryCodesLookupMap)
	srv.RegisterServer(s)

	errChan := make(chan error, 1)
	go func() {
		errChan <- s.Serve(lis)
	}()

	cfg.logger.Info("starting-server", lager.Data{"address": cfg.Addr})
	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		cfg.logger.Info("stopping-server")
		s.GracefulStop()
		return ctx.Err()
	}
}

func getConfig() (*config, error) {
	cfg := config{
		Addr:          "localhost:8000",
		MaxConStreams: 1000,
	}

	if err := envstruct.Load(&cfg); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Populate the map from the slice for better lookup performance
	cfg.blockedCountryCodesLookupMap = make(map[string]struct{}, len(cfg.BlockedCountryCodes))
	for _, item := range cfg.BlockedCountryCodes {
		cfg.blockedCountryCodesLookupMap[item] = struct{}{}
	}

	if err := envstruct.WriteReport(&cfg); err != nil {
		return nil, fmt.Errorf("failed to write config report: %w", err)
	}

	// TODO: allow config of which http header to extract req IP from (XFF, x-real-ip, etc)
	// TODO: allow config of which http header to inject country code in
	return &cfg, nil
}
