package log

import (
	"bytes"
	"context"
	"net"
	"os"
	"testing"

	"github.com/ccrabbai/logos/internal/config"
	"github.com/ccrabbai/logos/internal/observability"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	api "github.com/ccrabbai/logos/api/logs/v1"
	ingrpc "github.com/ccrabbai/logos/internal/server"
)

const bufSize = 1024 * 1024

func TestQAi_gRPC_ServerPipeline(t *testing.T) {
	dir, err := os.MkdirTemp("", "grpc-qa-test-*")
	if err != nil {
		t.Fatalf("GRPC-QA-SETUP-ERR: Failed to create temp folder: %v", err)
	}
	defer os.RemoveAll(dir)

	// Inject the newly isolated config container structures
	commitLog, err := NewLog(dir, config.Config{}, &observability.Metrics{})
	if err != nil {
		t.Fatalf("GRPC-QA-SETUP-ERR: Failed to initialize log subsystem: %v", err)
	}

	lis := bufconn.Listen(bufSize)
	
	// Ensure your ingrpc.Config signature struct matches your server constructor bindings perfectly
	srvImpl, err := ingrpc.NewgrpcServer(&ingrpc.Config{CommitLog: commitLog})
	if err != nil {
		t.Fatalf("GRPC-QA-SETUP-ERR: Failed to instantiate gRPC implementation: %v", err)
	}
	
	baseGrpcServer := grpc.NewServer()
	api.RegisterLogServer(baseGrpcServer, srvImpl)

	go func() {
		if err := baseGrpcServer.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			t.Errorf("GRPC-QA-ERR: Server encountered a serving failure: %v", err)
		}
	}()
	defer baseGrpcServer.GracefulStop()

	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet", 
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}), 
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("GRPC-QA-CONN-ERR: Failed to dial in-memory network pipeline: %v", err)
	}
	defer conn.Close()

	client := api.NewLogClient(conn)

	writePayload := []byte("INTEGRATION-TEST-NETWORK-PAYLOAD")
	produceResp, err := client.Produce(ctx, &api.ProduceRequest{
		Record: &api.Record{Value: writePayload},
	})
	if err != nil {
		t.Fatalf("TC-RPC-001 FAILED: Server rejected healthy Produce request: %v", err)
	}

	expectedOffset := uint64(0) 
	if produceResp.Offset != expectedOffset {
		t.Errorf("TC-RPC-001 FAILED: Expected assigned offset %d, captured %d", expectedOffset, produceResp.Offset)
	}

	consumeResp, err := client.Consume(ctx, &api.ConsumeRequest{
		Offset: produceResp.Offset,
	})
	if err != nil {
		t.Fatalf("TC-RPC-002 FAILED: Server failed to process safe Consume request: %v", err)
	}

	if !bytes.Equal(consumeResp.Record.Value, writePayload) {
		t.Errorf("TC-RPC-002 FAILED: Payload data corruption over wire. Sent %s, read %s", writePayload, consumeResp.Record.Value)
	}

	_, err = client.Consume(ctx, &api.ConsumeRequest{
		Offset: 9999, 
	})
	if err == nil {
		t.Errorf("TC-RPC-003 FAILED: Server read from non-existent slot 9999 without returning error blocks")
	} else {
		st, ok := status.FromError(err)
		if !ok {
			t.Errorf("TC-RPC-003 FAILED: Error response does not contain readable gRPC status payloads")
		}
		if st.Code() != codes.NotFound {
			t.Logf("INFO: Server code mapped out-of-bounds error to code string: %s (Expected: NotFound)", st.Code())
		}
	}
}
