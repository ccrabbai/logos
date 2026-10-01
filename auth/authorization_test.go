package auth

import (
	"context"
	"path/filepath"
	"testing"
)

func TestEnforceRBAC(t *testing.T) {
	model := filepath.Join("files", "model.conf")
	policy := filepath.Join("files", "policy.csv")

	authorizer, err := New(model, policy)
	if err != nil {
		t.Fatalf("New(%q, %q): %v", model, policy, err)
	}

	tests := []struct {
		name    string
		subject string
		method  string
		wantErr bool
	}{
		{
			name:    "admin can produce",
			subject: "root",
			method:  "/logsv1.Log/Produce",
		},
		{
			name:    "admin can consume",
			subject: "root",
			method:  "/logsv1.Log/Consume",
		},
		{
			name:    "writer can produce",
			subject: "client",
			method:  "/logsv1.Log/Produce",
		},
		{
			name:    "reader can consume",
			subject: "client",
			method:  "/logsv1.Log/Consume",
		},
		{
			name:    "writer cannot invoke unknown method",
			subject: "client",
			method:  "/logsv1.Log/Delete",
			wantErr: true,
		},
		{
			name:    "unknown subject denied",
			subject: "unknown-user",
			method:  "/logsv1.Log/Produce",
			wantErr: true,
		},
		{
			name:    "unknown service denied",
			subject: "root",
			method:  "/other.Service/Produce",
			wantErr: true,
		},
		{
			name:    "malformed method denied",
			subject: "root",
			method:  "Produce",
			wantErr: true,
		},
		{
			name:    "empty method denied",
			subject: "root",
			method:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := authorizer.EnforceRBAC(
				context.Background(),
				tt.method,
				func(context.Context) string {
					return tt.subject
				},
			)

			if (err != nil) != tt.wantErr {
				t.Fatalf("EnforceRBAC() error = %v, wantErr %v",
					err, tt.wantErr)
			}
		})
	}
}