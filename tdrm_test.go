package tdrm

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

func TestRevisionKey(t *testing.T) {
	tests := []struct {
		ref  string
		want string
		ok   bool
	}{
		{ref: "batch:13", want: "batch:13", ok: true},
		{ref: "My_batch-1:123", want: "My_batch-1:123", ok: true},
		{ref: "arn:aws:ecs:us-east-1:123456789012:task-definition/batch:13", want: "batch:13", ok: true},
		{ref: "arn:aws-us-gov:ecs:us-gov-west-1:123456789012:task-definition/batch:13", want: "batch:13", ok: true},
		{ref: "arn:aws-cn:ecs:cn-north-1:123456789012:task-definition/batch:13", want: "batch:13", ok: true},
		{ref: ""},
		{ref: "batch"},
		{ref: "arn:aws:ecs:us-east-1:123456789012:task-definition/batch"},
		{ref: "batch:"},
		{ref: "batch:latest"},
		{ref: "batch:-1"},
		{ref: "batch:1:2"},
		{ref: "batch/name:1"},
		{ref: " batch:1"},
		{ref: "batch:1 "},
		{ref: "arn:aws:ecs:us-east-1:123456789012:service/batch:13"},
		{ref: "arn:aws:lambda:us-east-1:123456789012:task-definition/batch:13"},
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			got, ok := revisionKey(tt.ref)
			if got != tt.want || ok != tt.ok {
				t.Errorf("revisionKey(%q) = (%q, %t), want (%q, %t)", tt.ref, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestInUseAdd(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{ref: "batch:13", want: "batch:13"},
		{ref: "arn:aws:ecs:us-east-1:123456789012:task-definition/batch:13", want: "batch:13"},
		{ref: "batch", want: "batch"},
		{ref: "arn:aws:ecs:us-east-1:123456789012:task-definition/batch", want: "batch"},
		{ref: "arn:aws-us-gov:ecs:us-gov-west-1:123456789012:task-definition/My_batch-1", want: "My_batch-1"},
		{ref: ""},
		{ref: "batch:latest"},
		{ref: "arn:aws:ecs:us-east-1:123456789012:service/batch"},
	}
	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			got := make(inUseRevisions)
			got.add(tt.ref)
			want := make(inUseRevisions)
			if tt.want != "" {
				want[tt.want] = struct{}{}
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("inUse.add(%q) = %v, want %v", tt.ref, got, want)
			}
		})
	}
}

func TestExtractTaskDefinitionRefs(t *testing.T) {
	standard := "arn:aws:ecs:us-east-1:123456789012:task-definition/batch:13"
	gov := "arn:aws-us-gov:ecs:us-gov-west-1:123456789012:task-definition/My_batch-1:42"
	china := "arn:aws-cn:ecs:cn-north-1:123456789012:task-definition/batch:7"
	tests := []struct {
		name       string
		definition string
		want       []string
	}{
		{
			name:       "all literal references in the raw definition",
			definition: `{"References":["` + standard + `","` + gov + `","` + china + `"]}`,
			want:       []string{standard, gov, china},
		},
		{
			name:       "JSON-escaped slash outside TaskDefinition",
			definition: `{"td":"arn:aws:ecs:us-east-1:123456789012:task-definition\/batch:13"}`,
			want:       []string{standard},
		},
		{
			name:       "repeated references",
			definition: standard + " " + standard,
			want:       []string{standard, standard},
		},
		{
			name:       "short form",
			definition: `{"TaskDefinition":"batch:13"}`,
			want:       []string{"batch:13"},
		},
		{
			name:       "raw ARN and short form together",
			definition: `{"Other":"` + standard + `","TaskDefinition":"batch:14"}`,
			want:       []string{standard, "batch:14"},
		},
		{
			name:       "family only",
			definition: `{"TaskDefinition":"batch"}`,
			want:       []string{"batch"},
		},
		{
			name:       "family-only ARN",
			definition: `{"TaskDefinition":"arn:aws:ecs:us-east-1:123456789012:task-definition/batch"}`,
			want:       []string{"arn:aws:ecs:us-east-1:123456789012:task-definition/batch"},
		},
		{
			name: "nested Map ItemProcessor",
			definition: `{"States":{"Map":{"ItemProcessor":{"States":{` +
				`"Run":{"Parameters":{"TaskDefinition":"batch:13"}}}}}}}`,
			want: []string{"batch:13"},
		},
		{
			name:       "walk arrays and maps",
			definition: `[{"TaskDefinition":"batch:13"},{"Nested":[{"TaskDefinition":"other"}]}]`,
			want:       []string{"batch:13", "other"},
		},
		{
			name:       "ignore runtime references and other key names",
			definition: `{"TaskDefinition.$":"batch:13","taskDefinition":"batch:14","Other":"batch"}`,
		},
		{
			name:       "ignore non-string TaskDefinition values",
			definition: `{"TaskDefinition":13,"Nested":{"TaskDefinition":null}}`,
		},
		{
			name:       "invalid JSON still returns literal ARNs",
			definition: `{"TaskDefinition":"batch:13","Other":"` + standard,
			want:       []string{standard},
		},
		{
			name: "ignore other resources and invalid references",
			definition: `arn:aws:ecs:us-east-1:123456789012:service/batch:13 ` +
				`arn:aws:ecs:us-east-1:123:task-definition/batch:13 ` +
				`arn:aws:ecs:us-east-1:123456789012:task-definition/batch:latest`,
		},
		{name: "empty definition"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTaskDefinitionRefs(tt.definition)
			slices.Sort(got)
			want := slices.Clone(tt.want)
			slices.Sort(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("extractTaskDefinitionRefs() = %v, want %v", got, want)
			}
		})
	}
}

func TestSelectToInactive(t *testing.T) {
	const prefix = "arn:aws:ecs:us-east-1:123456789012:task-definition/batch:"
	active := []string{prefix + "5", prefix + "4", prefix + "3", prefix + "2", prefix + "1"}
	tests := []struct {
		name      string
		active    []string
		keepCount int
		inUse     inUseRevisions
		want      []string
	}{
		{
			name: "no protected revisions", active: active, keepCount: 2,
			want: []string{prefix + "3", prefix + "2", prefix + "1"},
		},
		{
			name: "in-use revisions do not count toward keep count", active: active, keepCount: 2,
			inUse: inUseRevisions{"batch:5": {}, "batch:3": {}},
			want:  []string{prefix + "1"},
		},
		{
			name: "old in-use revision is kept", active: active, keepCount: 2,
			inUse: inUseRevisions{"batch:1": {}},
			want:  []string{prefix + "3", prefix + "2"},
		},
		{
			name: "zero keep count still protects in-use revisions", active: active, keepCount: 0,
			inUse: inUseRevisions{"batch:5": {}, "batch:3": {}},
			want:  []string{prefix + "4", prefix + "2", prefix + "1"},
		},
		{
			name: "zero keep count without protected revisions", active: active, keepCount: 0,
			want: active,
		},
		{
			name: "family-only reference protects latest with zero keep count", active: active, keepCount: 0,
			inUse: inUseRevisions{"batch": {}},
			want:  active[1:],
		},
		{
			name: "family-only protection does not count toward keep count", active: active, keepCount: 1,
			inUse: inUseRevisions{"batch": {}, "batch:3": {}},
			want:  []string{prefix + "2", prefix + "1"},
		},
		{
			name: "other family does not protect latest", active: active, keepCount: 0,
			inUse: inUseRevisions{"other": {}},
			want:  active,
		},
		{
			name: "all revisions in use", active: active, keepCount: 0,
			inUse: inUseRevisions{"batch:5": {}, "batch:4": {}, "batch:3": {}, "batch:2": {}, "batch:1": {}},
		},
		{
			name: "fewer unprotected revisions than keep count", active: active, keepCount: 4,
			inUse: inUseRevisions{"batch:5": {}, "batch:3": {}},
		},
		{name: "fewer active revisions than keep count", active: active[:2], keepCount: 3},
		{name: "empty active revisions", keepCount: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := selectToInactive(tt.active, tt.keepCount, tt.inUse); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("selectToInactive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSelectToDelete(t *testing.T) {
	const prefix = "arn:aws:ecs:us-east-1:123456789012:task-definition/batch:"
	inactive := []string{prefix + "3", prefix + "2", prefix + "1"}
	tests := []struct {
		name     string
		inactive []string
		inUse    inUseRevisions
		want     []string
	}{
		{name: "no protected revisions", inactive: inactive, want: inactive},
		{
			name: "in-use inactive revision is kept", inactive: inactive,
			inUse: inUseRevisions{"batch:2": {}},
			want:  []string{prefix + "3", prefix + "1"},
		},
		{
			name: "all inactive revisions in use", inactive: inactive,
			inUse: inUseRevisions{"batch:3": {}, "batch:2": {}, "batch:1": {}},
		},
		{
			name: "family-only reference protects active revisions only", inactive: inactive,
			inUse: inUseRevisions{"batch": {}}, want: inactive,
		},
		{name: "empty inactive revisions"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := selectToDelete(tt.inactive, tt.inUse); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("selectToDelete() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestScanTaskDefinitionProtectsInUseRevisions(t *testing.T) {
	const prefix = "arn:aws:ecs:us-east-1:123456789012:task-definition/batch:"
	app := &App{ecs: ecs.NewFromConfig(fakeAWS(t, func(req *http.Request) (int, string) {
		var input struct{ Status string }
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		var revisions []string
		switch input.Status {
		case "ACTIVE":
			revisions = []string{prefix + "5", prefix + "4"}
		case "INACTIVE":
			revisions = []string{prefix + "3", prefix + "2"}
		default:
			t.Fatalf("unexpected revision status %q", input.Status)
		}
		body, err := json.Marshal(map[string][]string{"taskDefinitionArns": revisions})
		if err != nil {
			t.Fatal(err)
		}
		return http.StatusOK, string(body)
	}))}
	summary, taskDef, err := app.scanTaskDefinition(context.Background(), "batch", 0, inUseRevisions{"batch": {}, "batch:3": {}})
	if err != nil {
		t.Fatal(err)
	}
	wantTaskDef := &TaskDefinition{Family: "batch", ToInactive: []string{prefix + "4"}, ToDelete: []string{prefix + "2"}}
	if !reflect.DeepEqual(taskDef, wantTaskDef) {
		t.Errorf("scanTaskDefinition() task definition = %+v, want %+v", taskDef, wantTaskDef)
	}
	wantSummary := &Summary{TaskDefinition: "batch", ActiveRevisions: 2, InactiveRevisions: 2, ToInactive: 1, ToDelete: 1, Keep: 1}
	if !reflect.DeepEqual(summary, wantSummary) {
		t.Errorf("scanTaskDefinition() summary = %+v, want %+v", summary, wantSummary)
	}
}
