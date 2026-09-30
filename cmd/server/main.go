package main

import (
	"net"
	"log"
	"time"
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"github.com/ccrabbai/logos/auth"
	"google.golang.org/grpc/credentials"
	"github.com/ccrabbai/logos/internal/observability"

	clog "github.com/ccrabbai/logos/internal/log"
	ingrpc "github.com/ccrabbai/logos/internal/server"
	config "github.com/ccrabbai/logos/internal/config"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
)

func main(){
	ctx := context.Background()

	obsConfig, err := config.LoadObservabilityConfig()
	if err != nil {
		log.Fatalf("failed to load observability configuration: %v", err)
	}

	tp, err := observability.InitTracing(ctx, obsConfig)
	if err != nil {
		log.Fatalf("failed to initialize tracing: %v", err)
	}

	mp, err := observability.InitMetrics(ctx, obsConfig)
	if err != nil {
		_ = tp.Shutdown(ctx)
		log.Fatalf("failed to initialize metrics: %v", err)
	}

	metrics, err := observability.NewMetrics()
	if err != nil {
		_ = mp.Shutdown(ctx)
		_ = tp.Shutdown(ctx)
		slog.Error("failed to create metrics instruments: ", "error", err,)
		return
	}

	defer func() {
		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cancel()

		if err := mp.Shutdown(shutdownCtx); err != nil {
			log.Printf("failed to shut down metrics: %v", err)
		}
		if err := tp.Shutdown(shutdownCtx); err != nil {
			log.Printf("failed to shut down tracing: %v", err)
		}
	}()

	var lc net.ListenConfig
	listener, err := lc.Listen(context.Background(), "tcp", ":50051")
	if err != nil{
		slog.Error("failed to listen", "error", err,)
		return
	}
	slog.Info("starting Logos server", "address", listener.Addr().String(),)

	// mTLS config
	serverTLSConfig, err := config.SetupTLSConfig(config.TLSConfig{
		CertFile: config.ServerCertFile,
		KeyFile: config.ServerKeyFile,
		CAFile: config.CAFile,
		ServerAddress: listener.Addr().String(),
		Server: true, //mTLS
	})
	if err != nil{
		slog.Error("failed to get server certificate", "error", err)
		return
	}
	
	serverCreds := credentials.NewTLS(serverTLSConfig)

	authorizer, err := auth.New(config.ACLModelFile, config.ACLPolicyFile)
	if err != nil{
		slog.Error("failed to get authorization credentials", "error", err)
		return
	}

	commitLog, err := clog.NewLog("store_test_files", config.Config{}, metrics)
	if err != nil{
		slog.Error("failed to instantiate log","error", err)
		return
	}

	svr, err := ingrpc.NewGRPCServer(&ingrpc.Config{CommitLog: commitLog, Authorizer: authorizer}, grpc.Creds(serverCreds))
	if err != nil{
		slog.Error("unable to instantiate ingrpc","error", err)
		return
	}

	healthSvr := health.NewServer()
	healthv1.RegisterHealthServer(svr, healthSvr)

	// This blocks indefinitely, handling incoming RPC calls
	if err := svr.Serve(listener); err != nil{
		slog.Error("failed to serve","error", err)
		return
	}
}