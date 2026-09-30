package main

import (
	"fmt"
	"log"
	"time"
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"github.com/ccrabbai/logos/internal/config"
	"github.com/ccrabbai/logos/internal/observability"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"

	apiv1 "github.com/ccrabbai/logos/api/logs/v1"
)


func main(){
	ctx := context.Background()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	obsConfig, err := config.LoadObservabilityConfig()
	if err != nil {
		log.Fatalf("failed to load observability configuration: %v", err)
	}

	tp, err := observability.InitTracing(ctx, obsConfig)
	if err != nil {
		log.Fatalf("failed to initialize tracing: %v", err)
	}
	defer func() {
		if err := tp.Shutdown(shutdownCtx); err != nil {
			log.Printf("failed to shutdown tracing: %v", err)
		}
	}()

	mp, err := observability.InitMetrics(ctx, obsConfig)
	if err != nil {
		log.Fatalf("failed to initialize metrics: %v", err)
	}
	defer func(){
		if err := mp.Shutdown(shutdownCtx); err != nil {
			log.Printf("failed to shutdown metrics: %v", err)
		}
	}()

	logger := observability.NewLogger()
	slog.SetDefault(logger)

	// mTLS config
	clientTLSConfig, err := config.SetupTLSConfig(config.TLSConfig{
		CertFile: config.ClientCertFile, //mTLS
		KeyFile: config.ClientKeyFile, //mTLS
		CAFile: config.CAFile,
		Server: false,
	})
	if err != nil{
		slog.Error("failed to get client certificate", "error", err)
	}

	clientCreds := credentials.NewTLS(clientTLSConfig)

	conn, err := grpc.NewClient(
		"localhost:50051", 
		grpc.WithTransportCredentials(clientCreds),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil{
		slog.Error("failed to get new client connection","error", err)
	}

	defer conn.Close() 

	client := apiv1.NewLogClient(conn)

	for i := range 10{
		resp, err := client.Produce(
			context.Background(), 
			&apiv1.ProduceRequest{
				Record: &apiv1.Record{
					Value: fmt.Append(nil, "We are back with another level of grit and ginger -- ",i),
				},
			},
		)
		if err != nil{
			slog.InfoContext(context.Background(), "unable to produce", "error", err)
			return
		}
		slog.InfoContext(context.Background(), "produce completed","offset", resp.Offset,)
		log.Println(resp)
	}

	for i := range 20{
		readResp, err := client.Consume(
				context.Background(), 
				&apiv1.ConsumeRequest{Offset: uint64(i),},
			)
		if err != nil{
			slog.Error("unable to consume", "error", err)
			return
		}

		log.Println(readResp)
	}
}