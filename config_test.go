package tdrm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigProtectedRevisions(t *testing.T) {
	tests := []struct {
		name      string
		protected string
		wantErr   bool
	}{
		{name: "omitted"},
		{
			name: "ARNs and family revisions",
			protected: "protected_revisions:\n" +
				"  - arn:aws:ecs:us-east-1:123456789012:task-definition/batch:13\n" +
				"  - batch:14\n",
		},
		{name: "family only", protected: "protected_revisions: [batch]\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "tdrm.yaml")
			data := tt.protected + "task_definitions:\n  - family_prefix: batch\n    keep_count: 2\n"
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadConfig(path)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "protected_revisions") {
					t.Fatalf("LoadConfig() error = %v, want protected_revisions validation error", err)
				}
			} else if err != nil {
				t.Fatalf("LoadConfig() error = %v", err)
			}
		})
	}
}
