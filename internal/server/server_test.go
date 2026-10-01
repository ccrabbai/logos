package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"testing"

	api "github.com/ccrabbai/logos/api/logs/v1"
	"github.com/ccrabbai/logos/internal/util"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type fakeCommitLog struct {
	appendFn func(context.Context, *api.Record) (uint64, error)
	readFn   func(context.Context, uint64) (*api.Record, error)

	appended []*api.Record
}

func (f *fakeCommitLog) Append(ctx context.Context, record *api.Record) (uint64, error) {
	f.appended = append(f.appended, record)

	if f.appendFn != nil {
		return f.appendFn(ctx, record)
	}

	return uint64(len(f.appended) - 1), nil
}

func (f *fakeCommitLog) Read(ctx context.Context, offset uint64) (*api.Record, error) {
	if f.readFn != nil {
		return f.readFn(ctx, offset)
	}

	return &api.Record{
		Offset: offset,
		Value:  []byte("record"),
	}, nil
}

type fakeAuthorizer struct {
	err error

	calls      int
	lastMethod string
}

func (f *fakeAuthorizer) EnforceRBAC(
	ctx context.Context,
	fullMethod string,
	subjectExtractor func(context.Context) string,
) error {
	f.calls++
	f.lastMethod = fullMethod

	if f.err != nil {
		return f.err
	}

	return nil
}

func authenticatedContext(t *testing.T, commonName string) context.Context {
	t.Helper()

	cert := &x509.Certificate{
		Subject: pkix.Name{
			CommonName: commonName,
		},
	}

	tlsInfo := credentials.TLSInfo{
		State: tls.ConnectionState{
			VerifiedChains: [][]*x509.Certificate{
				{cert},
			},
		},
	}

	ctx := peer.NewContext(
		context.Background(),
		&peer.Peer{
			AuthInfo: tlsInfo,
		},
	)

	authenticated, err := authenticate(ctx)
	if err != nil {
		t.Fatalf("authenticate() error = %v", err)
	}

	return authenticated
}

func TestAuthenticate(t *testing.T) {
	t.Run("missing peer", func(t *testing.T) {
		_, err := authenticate(context.Background())

		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("expected Unauthenticated, got %v", status.Code(err))
		}
	})

	t.Run("missing authentication information", func(t *testing.T) {
		ctx := peer.NewContext(
			context.Background(),
			&peer.Peer{},
		)

		_, err := authenticate(ctx)

		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("expected Unauthenticated, got %v", status.Code(err))
		}
	})

	t.Run("non TLS authentication", func(t *testing.T) {
		ctx := peer.NewContext(
			context.Background(),
			&peer.Peer{
				AuthInfo: testAuthInfo{},
			},
		)

		_, err := authenticate(ctx)

		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("expected Unauthenticated, got %v", status.Code(err))
		}
	})

	t.Run("TLS without verified chain", func(t *testing.T) {
		ctx := peer.NewContext(
			context.Background(),
			&peer.Peer{
				AuthInfo: credentials.TLSInfo{},
			},
		)

		_, err := authenticate(ctx)

		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("expected Unauthenticated, got %v", status.Code(err))
		}
	})

	t.Run("TLS with empty certificate chain", func(t *testing.T) {
		tlsInfo := credentials.TLSInfo{}
		tlsInfo.State.VerifiedChains = [][]*x509.Certificate{
			{},
		}

		ctx := peer.NewContext(
			context.Background(),
			&peer.Peer{
				AuthInfo: tlsInfo,
			},
		)

		_, err := authenticate(ctx)

		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("expected Unauthenticated, got %v", status.Code(err))
		}
	})

	t.Run("certificate without common name", func(t *testing.T) {
		tlsInfo := credentials.TLSInfo{}
		tlsInfo.State.VerifiedChains = [][]*x509.Certificate{
			{
				&x509.Certificate{},
			},
		}

		ctx := peer.NewContext(
			context.Background(),
			&peer.Peer{
				AuthInfo: tlsInfo,
			},
		)

		_, err := authenticate(ctx)

		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("expected Unauthenticated, got %v", status.Code(err))
		}
	})

	t.Run("valid TLS identity", func(t *testing.T) {
		ctx := authenticatedContext(t, "client")

		authenticated, err := authenticate(ctx)
		if err != nil {
			t.Fatalf("authenticate returned error: %v", err)
		}

		if got := subject(authenticated); got != "client" {
			t.Fatalf("expected subject %q, got %q", "client", got)
		}
	})
}

func TestSubject(t *testing.T) {
	t.Run("missing subject returns intruder", func(t *testing.T) {
		if got := subject(context.Background()); got != "intruder" {
			t.Fatalf("expected intruder, got %q", got)
		}
	})

	t.Run("authenticated subject", func(t *testing.T) {
		ctx := context.WithValue(
			context.Background(),
			subjectContextKey{},
			"client",
		)

		if got := subject(ctx); got != "client" {
			t.Fatalf("expected client, got %q", got)
		}
	})

	t.Run("wrong subject type returns intruder", func(t *testing.T) {
		ctx := context.WithValue(
			context.Background(),
			subjectContextKey{},
			123,
		)

		if got := subject(ctx); got != "intruder" {
			t.Fatalf("expected intruder, got %q", got)
		}
	})
}

func TestProduce(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		log := &fakeCommitLog{}
		srv := &grpcServer{
			Config: &Config{
				CommitLog: log,
			},
		}

		_, err := srv.Produce(authenticatedContext(t, "client"), nil)

		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
		}
	})

	t.Run("nil record", func(t *testing.T) {
		log := &fakeCommitLog{}
		srv := &grpcServer{
			Config: &Config{
				CommitLog: log,
			},
		}

		_, err := srv.Produce(
			authenticatedContext(t, "client"),
			&api.ProduceRequest{},
		)

		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
		}
	})

	t.Run("authenticated producer identity is applied", func(t *testing.T) {
		log := &fakeCommitLog{
			appendFn: func(ctx context.Context, record *api.Record) (uint64, error) {
				return 42, nil
			},
		}

		srv := &grpcServer{
			Config: &Config{
				CommitLog: log,
			},
		}

		record := &api.Record{
			ProducerId: "forged-producer",
			Value:      []byte("hello"),
		}

		resp, err := srv.Produce(
			authenticatedContext(t, "client"),
			&api.ProduceRequest{
				Record: record,
			},
		)

		if err != nil {
			t.Fatalf("Produce returned error: %v", err)
		}

		if resp.Offset != 42 {
			t.Fatalf("expected offset 42, got %d", resp.Offset)
		}

		if record.ProducerId != "client" {
			t.Fatalf(
				"expected producer identity %q, got %q",
				"client",
				record.ProducerId,
			)
		}

		if len(log.appended) != 1 {
			t.Fatalf("expected one append, got %d", len(log.appended))
		}
	})
}

func TestConsume(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		srv := &grpcServer{
			Config: &Config{
				CommitLog: &fakeCommitLog{},
			},
		}

		_, err := srv.Consume(
			authenticatedContext(t, "client"),
			nil,
		)

		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", status.Code(err))
		}
	})

	t.Run("reads requested offset", func(t *testing.T) {
		const expectedOffset uint64 = 7

		log := &fakeCommitLog{
			readFn: func(ctx context.Context, offset uint64) (*api.Record, error) {
				if offset != expectedOffset {
					t.Fatalf(
						"expected Read offset %d, got %d",
						expectedOffset,
						offset,
					)
				}

				return &api.Record{
					Offset: offset,
					Value:  []byte("hello"),
				}, nil
			},
		}

		srv := &grpcServer{
			Config: &Config{
				CommitLog: log,
			},
		}

		resp, err := srv.Consume(
			authenticatedContext(t, "client"),
			&api.ConsumeRequest{
				Offset: expectedOffset,
			},
		)

		if err != nil {
			t.Fatalf("Consume returned error: %v", err)
		}

		if resp.Record == nil {
			t.Fatal("expected record, got nil")
		}

		if resp.Record.Offset != expectedOffset {
			t.Fatalf(
				"expected record offset %d, got %d",
				expectedOffset,
				resp.Record.Offset,
			)
		}
	})
}

type fakeConsumeStream struct {
	ctx context.Context

	sent []*api.ConsumeResponse
}

func (s *fakeConsumeStream) Send(resp *api.ConsumeResponse) error {
	s.sent = append(s.sent, resp)
	return nil
}

func (s *fakeConsumeStream) SetHeader(metadata.MD) error {
	return nil
}

func (s *fakeConsumeStream) SendHeader(metadata.MD) error {
	return nil
}

func (s *fakeConsumeStream) SetTrailer(metadata.MD) {}

func (s *fakeConsumeStream) Context() context.Context {
	return s.ctx
}

func (s *fakeConsumeStream) SendMsg(any) error {
	return nil
}

func (s *fakeConsumeStream) RecvMsg(any) error {
	return nil
}

func TestConsumeStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var reads int

	log := &fakeCommitLog{
		readFn: func(ctx context.Context, offset uint64) (*api.Record, error) {
			reads++

			if reads > 3 {
				cancel()
				return nil, util.ErrOffsetOutOfRange{}
			}

			return &api.Record{
				Offset: offset,
				Value:  []byte{byte(offset)},
			}, nil
		},
	}

	srv := &grpcServer{
		Config: &Config{
			CommitLog: log,
		},
	}

	stream := &fakeConsumeStream{
		ctx: ctx,
	}

	err := srv.ConsumeStream(
		&api.ConsumeRequest{
			Offset: 10,
		},
		stream,
	)

	if err != nil {
		t.Fatalf("ConsumeStream returned error: %v", err)
	}

	if len(stream.sent) != 3 {
		t.Fatalf(
			"expected 3 streamed records, got %d",
			len(stream.sent),
		)
	}

	for i, resp := range stream.sent {
		expected := uint64(10 + i)

		if resp.Record == nil {
			t.Fatalf("response %d contains nil record", i)
		}

		if resp.Record.Offset != expected {
			t.Fatalf(
				"response %d: expected offset %d, got %d",
				i,
				expected,
				resp.Record.Offset,
			)
		}
	}
}

type fakeProduceStream struct {
	ctx context.Context

	requests  []*api.ProduceRequest
	requestAt int

	responses []*api.ProduceResponse
}

func (s *fakeProduceStream) Recv() (*api.ProduceRequest, error) {
	if s.requestAt >= len(s.requests) {
		return nil, io.EOF
	}

	req := s.requests[s.requestAt]
	s.requestAt++

	return req, nil
}

func (s *fakeProduceStream) Send(resp *api.ProduceResponse) error {
	s.responses = append(s.responses, resp)
	return nil
}

func (s *fakeProduceStream) SetHeader(metadata.MD) error {
	return nil
}

func (s *fakeProduceStream) SendHeader(metadata.MD) error {
	return nil
}

func (s *fakeProduceStream) SetTrailer(metadata.MD) {}

func (s *fakeProduceStream) Context() context.Context {
	return s.ctx
}

func (s *fakeProduceStream) SendMsg(any) error {
	return nil
}

func (s *fakeProduceStream) RecvMsg(any) error {
	return nil
}

func TestProduceStream(t *testing.T) {
	log := &fakeCommitLog{
		appendFn: func(
			ctx context.Context,
			record *api.Record,
		) (uint64, error) {
			return uint64(len(record.Value)), nil
		},
	}

	srv := &grpcServer{
		Config: &Config{
			CommitLog: log,
		},
	}

	stream := &fakeProduceStream{
		ctx: authenticatedContext(t, "client"),
		requests: []*api.ProduceRequest{
			{
				Record: &api.Record{
					ProducerId: "ignored",
					Value:      []byte("one"),
				},
			},
			{
				Record: &api.Record{
					ProducerId: "ignored",
					Value:      []byte("two"),
				},
			},
		},
	}

	err := srv.ProduceStream(stream)

	if err != io.EOF {
		t.Fatalf("expected io.EOF after client stream ended, got %v", err)
	}

	if len(stream.responses) != 2 {
		t.Fatalf(
			"expected 2 responses, got %d",
			len(stream.responses),
		)
	}

	if stream.responses[0].Offset != 3 {
		t.Fatalf("expected first offset 3, got %d", stream.responses[0].Offset)
	}

	if stream.responses[1].Offset != 3 {
		t.Fatalf("expected second offset 3, got %d", stream.responses[1].Offset)
	}

	if len(log.appended) != 2 {
		t.Fatalf("expected 2 appended records, got %d", len(log.appended))
	}

	for i, record := range log.appended {
		if record.ProducerId != "client" {
			t.Fatalf(
				"record %d: expected producer client, got %q",
				i,
				record.ProducerId,
			)
		}
	}
}

func TestNewGRPCServer(t *testing.T) {
	authorizer := &fakeAuthorizer{}
	log := &fakeCommitLog{}

	server, err := NewGRPCServer(&Config{
		CommitLog: log,
		Authorizer: authorizer,
	})

	if err != nil {
		t.Fatalf("NewGRPCServer returned error: %v", err)
	}

	if server == nil {
		t.Fatal("expected non-nil gRPC server")
	}

	server.Stop()
}

func TestNewgrpcServer(t *testing.T) {
	config := &Config{
		CommitLog: &fakeCommitLog{},
		Authorizer: &fakeAuthorizer{},
	}

	srv, err := NewgrpcServer(config)
	if err != nil {
		t.Fatalf("NewgrpcServer returned error: %v", err)
	}

	if srv == nil {
		t.Fatal("expected non-nil grpcServer")
	}

	if srv.Config != config {
		t.Fatal("expected server to retain supplied config")
	}
}

type testAuthInfo struct{}

func (testAuthInfo) AuthType() string {
	return "test"
}

// Compile-time checks for the fake stream implementations.
var (
	_ grpc.ServerStreamingServer[api.ConsumeResponse] = (*fakeConsumeStream)(nil)
	_ grpc.BidiStreamingServer[api.ProduceRequest, api.ProduceResponse] = (*fakeProduceStream)(nil)
)
