package auth

import (
	"strings"
	"context"

	"github.com/casbin/casbin/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Authorizer struct {
	enforcer *casbin.Enforcer
}

func New(modelPath, policyPath string) (*Authorizer, error) {
	enforcer, err := casbin.NewEnforcer(modelPath, policyPath)
	if err != nil {
		return nil, err
	}
	return &Authorizer{enforcer: enforcer}, nil
}

func (a *Authorizer) EnforceRBAC(ctx context.Context, fullMethod string, subjectExtractor func(context.Context) string) error {
	sub := subjectExtractor(ctx)
	parts := strings.Split(fullMethod, "/") // fullMethod structurally looks like: "/logsv1.Log/Produce"
	if len(parts) < 3 {
		return status.Error(codes.InvalidArgument, "malformed gRPC target path")
	}

	obj := parts[1]                  // Extracts service name: "log.v1.Log"
	act := strings.ToLower(parts[2]) // Extracts action name downcased: "produce"

	allowed, err := a.enforcer.Enforce(sub, obj, act)
	if err != nil {
		return status.Errorf(codes.Internal, "security authorization platform failure: %v", err)
	}

	if !allowed {
		return status.Errorf(
			codes.PermissionDenied,
			"access denied: client identity %q is not authorized to execute %q on %q",
			sub, act, obj,
		)
	}

	// Permission verified successfully
	return nil
}
