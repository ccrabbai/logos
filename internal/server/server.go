package server

import (
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/credentials"
	"github.com/ccrabbai/logos/internal/util"

	api "github.com/ccrabbai/logos/api/logs/v1"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
)

type CommitLog interface {
	Append(context.Context, *api.Record) (uint64, error)
	Read(context.Context, uint64) (*api.Record, error)
}
type Authorizer interface{
	EnforceRBAC(ctx context.Context, fullMethod string, subjectExtractor func(context.Context) string) error
}

type Config struct {
	CommitLog CommitLog
	Authorizer Authorizer
}

type grpcServer struct {
	api.UnimplementedLogServer
	*Config 
}

// 🔐 COMPILE-TIME INTERFACE VALIDATION GUARD
// This line uses Go's blank identifier ('_') to verify that our custom 'grpcServer' struct
// completely satisfies every single method signature demanded by the generated 'api.LogServer' interface.
// If we misspell a method name or provide the wrong parameter types in subsequent steps, the  
// compiler will break immediately right here, protecting us from silent runtime deployment failures.
var _ api.LogServer = (*grpcServer)(nil)

// newgrpcServer is the factory constructor function used to initialize our network gateway type.
func newgrpcServer(config *Config) (srv *grpcServer, err error) {
	srv = &grpcServer{
		Config: config,
	}
	return srv, nil
}

// To be removed
func NewgrpcServer(config *Config) (srv *grpcServer, err error) {
	return newgrpcServer(config)
}

func NewGRPCServer(config *Config, opts ...grpc.ServerOption) (*grpc.Server, error) {
	opts = append(opts,
		// tracing
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		// Unary APIs
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(
			grpc_auth.UnaryServerInterceptor(authenticate), // Authenticate identity
			func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
				// Enforce permissions using the requested method info 
				if err := config.Authorizer.EnforceRBAC(ctx, info.FullMethod, subject); err != nil {
					return nil, err // Reject call instantly
				}
				return handler(ctx, req)
			},
		)),
		// Streaming APIs
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(
			grpc_auth.StreamServerInterceptor(authenticate), 
			func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
				ctx := ss.Context()
				if err := config.Authorizer.EnforceRBAC(ctx, info.FullMethod, subject); err != nil {
					return err // Terminate socket stream instantly
				}
				return handler(srv, ss)
			},
		)),
	)

	gsrv := grpc.NewServer(opts...)
	srv, err := newgrpcServer(config)
	if err != nil {
		return nil, err
	}

	api.RegisterLogServer(gsrv, srv)

	return gsrv, nil
}

func (gs *grpcServer) Produce(ctx context.Context, in *api.ProduceRequest) (*api.ProduceResponse, error) {
	if in == nil || in.Record == nil {
		return nil, status.Error(
			codes.InvalidArgument,
			"produce request and record must not be nil",
		)
	}

	in.Record.ProducerId = subject(ctx)
	offset, err := gs.CommitLog.Append(ctx, in.Record)
	if err != nil{
		return nil, err
	}
	return &api.ProduceResponse{Offset: offset}, nil
}

func (gs *grpcServer) Consume(ctx context.Context, in *api.ConsumeRequest) (*api.ConsumeResponse, error) {
	if in == nil {
		return nil, status.Error(
			codes.InvalidArgument,
			"consume request must not be nil",
		)
	}

	resp, err := gs.CommitLog.Read(ctx, in.Offset)
	if err != nil{
		return nil, err // status.Error(codes.NotFound, err.Error())
	}
	return &api.ConsumeResponse{Record: resp}, nil
}

func (gs *grpcServer) ConsumeStream(in *api.ConsumeRequest, stream grpc.ServerStreamingServer[api.ConsumeResponse]) error {
	for {
		select{
		case <- stream.Context().Done():
			return nil
		default:
			resp, err := gs.Consume(stream.Context(), in)
			switch err.(type){
			case nil:
			case util.ErrOffsetOutOfRange:
				continue
			default:
				return err
			}
			if err := stream.Send(resp); err != nil{
				return err //status.Error(codes.Unknown, err.Error())
			}
			in.Offset++
		}
	}
}

func (gs *grpcServer) ProduceStream(stream grpc.BidiStreamingServer[api.ProduceRequest, api.ProduceResponse]) error {
	for {
		req, err := stream.Recv()
		if err != nil{
			return err //status.Errorf(codes.NotFound,"cannot read from client: %v", err.Error())
		}

		resp, err := gs.Produce(stream.Context(), req)
		if err != nil{
			return err //status.Errorf(codes.Unknown, "cannot append req: %v", err.Error())
		}

		if err = stream.Send(resp); err != nil{
			return err //status.Error(codes.Unknown, err.Error())
		}
	}
}

func authenticate(ctx context.Context) (context.Context, error) {
    p, ok := peer.FromContext(ctx)
    if !ok {
        return nil, status.Error(
            codes.Unauthenticated,
            "missing peer information",
        )
    }

    if p.AuthInfo == nil {
        return nil, status.Error(
            codes.Unauthenticated,
            "missing authentication information",
        )
    }

    tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
    if !ok {
        return nil, status.Error(
            codes.Unauthenticated,
            "peer did not authenticate using TLS",
        )
    }

    if len(tlsInfo.State.VerifiedChains) == 0 ||
        len(tlsInfo.State.VerifiedChains[0]) == 0 {
        return nil, status.Error(
            codes.Unauthenticated,
            "peer certificate has no verified chain",
        )
    }

    subject := tlsInfo.State.VerifiedChains[0][0].Subject.CommonName
    if subject == "" {
        return nil, status.Error(
            codes.Unauthenticated,
            "peer certificate has no subject common name",
        )
    }

    ctx = context.WithValue(ctx, subjectContextKey{}, subject)
    return ctx, nil
}

func subject(ctx context.Context) string {
    val := ctx.Value(subjectContextKey{})
    if val == nil {
        return "intruder"
    }
    
    s, ok := val.(string)
    if !ok {
        return "intruder" // Return an empty string if the type matches incorrectly
    }
    return s
}

type subjectContextKey struct{}