package tdrm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/scheduler"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

func fakeAWS(t *testing.T, handle func(*http.Request) (int, string)) aws.Config {
	t.Helper()
	return aws.Config{
		Region:      "us-east-1",
		Credentials: aws.AnonymousCredentials{},
		HTTPClient: smithyhttp.ClientDoFunc(func(req *http.Request) (*http.Response, error) {
			status, body := handle(req)
			return &http.Response{
				StatusCode: status,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}
}

func TestBuildInUse(t *testing.T) {
	responses := map[string]string{
		"AWSStepFunctions.ListStateMachines":        `{"stateMachines":[{"stateMachineArn":"arn:aws:states:us-east-1:123456789012:stateMachine:batch"}]}`,
		"AWSStepFunctions.DescribeStateMachine":     `{"definition":"{\"TaskDefinition\":\"batch:2\"}"}`,
		"AWSStepFunctions.ListStateMachineVersions": `{}`,
		"/schedules":                                          `{"Schedules":[{"Name":"test","GroupName":"default","Target":{"Arn":"arn:aws:ecs:us-east-1:123456789012:cluster/default"}}]}`,
		"/schedules/test":                                     `{"Target":{"EcsParameters":{"TaskDefinitionArn":"batch:3"}}}`,
		"AWSEvents.ListEventBuses":                            `{"EventBuses":[{"Name":"default"}]}`,
		"AWSEvents.ListRules":                                 `{"Rules":[{"Name":"test"}]}`,
		"AWSEvents.ListTargetsByRule":                         `{"Targets":[{"EcsParameters":{"TaskDefinitionArn":"batch:4"}}]}`,
		"AmazonEC2ContainerServiceV20141113.ListClusters":     `{"clusterArns":["arn:aws:ecs:us-east-1:123456789012:cluster/default"]}`,
		"AmazonEC2ContainerServiceV20141113.ListServices":     `{"serviceArns":["arn:aws:ecs:us-east-1:123456789012:service/default/test"]}`,
		"AmazonEC2ContainerServiceV20141113.DescribeServices": `{"services":[{"taskDefinition":"batch:5"}]}`,
	}
	for _, tt := range []struct {
		name        string
		failRequest string
	}{
		{name: "all sources"},
		{name: "Step Functions", failRequest: "AWSStepFunctions.ListStateMachines"},
		{name: "EventBridge Scheduler", failRequest: "/schedules"},
		{name: "EventBridge rules", failRequest: "AWSEvents.ListEventBuses"},
		{name: "ECS service", failRequest: "AmazonEC2ContainerServiceV20141113.ListClusters"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := fakeAWS(t, func(req *http.Request) (int, string) {
				key := req.Header.Get("X-Amz-Target")
				if key == "" {
					key = req.URL.Path
				}
				body, ok := responses[key]
				if !ok {
					t.Fatalf("unexpected request %s", key)
				}
				status := http.StatusOK
				if key == tt.failRequest {
					status = http.StatusForbidden
					body = `{"message":"denied"}`
				}
				return status, body
			})
			app := &App{
				sfn:         sfn.NewFromConfig(cfg),
				scheduler:   scheduler.NewFromConfig(cfg),
				eventbridge: eventbridge.NewFromConfig(cfg),
				ecs:         ecs.NewFromConfig(cfg),
			}
			inUse, err := app.buildInUse(context.Background(), []string{"batch:1"})
			if tt.failRequest != "" {
				if err == nil || !strings.Contains(err.Error(), tt.name) {
					t.Fatalf("buildInUse() error = %v, want %s", err, tt.name)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := inUseRevisions{"batch:1": {}, "batch:2": {}, "batch:3": {}, "batch:4": {}, "batch:5": {}}
			if !maps.Equal(inUse, want) {
				t.Errorf("in-use revisions = %v, want %v", inUse, want)
			}
		})
	}
}

func TestCollectStepFunctionsRevisions(t *testing.T) {
	const machineARN = "arn:aws:states:us-east-1:123456789012:stateMachine:batch"
	responses := map[string]string{
		"machines/": fmt.Sprintf(`{"stateMachines":[{"stateMachineArn":%q}]}`, machineARN),
		"versions/" + machineARN + "/": fmt.Sprintf(
			`{"stateMachineVersions":[{"stateMachineVersionArn":%q}],"nextToken":"versions-2"}`, machineARN+":1"),
		"versions/" + machineARN + "/versions-2": fmt.Sprintf(
			`{"stateMachineVersions":[{"stateMachineVersionArn":%q}]}`, machineARN+":2"),
		"definition/" + machineARN:        `{"definition":"{\"TaskDefinition\":\"arn:aws:ecs:us-east-1:123456789012:task-definition/batch:1\"}"}`,
		"definition/" + machineARN + ":1": `{"definition":"{\"TaskDefinition\":\"batch:2\"}"}`,
		"definition/" + machineARN + ":2": `{"definition":"{\"TaskDefinition\":\"arn:aws:ecs:us-east-1:123456789012:task-definition/batch:3\"}"}`,
	}
	app := &App{sfn: sfn.NewFromConfig(fakeAWS(t, func(req *http.Request) (int, string) {
		var input map[string]string
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		var key string
		switch req.Header.Get("X-Amz-Target") {
		case "AWSStepFunctions.ListStateMachines":
			key = "machines/" + input["nextToken"]
		case "AWSStepFunctions.ListStateMachineVersions":
			key = "versions/" + input["stateMachineArn"] + "/" + input["nextToken"]
		case "AWSStepFunctions.DescribeStateMachine":
			key = "definition/" + input["stateMachineArn"]
		default:
			t.Fatalf("unexpected Step Functions request %s", req.Header.Get("X-Amz-Target"))
		}
		body, ok := responses[key]
		if !ok {
			t.Fatalf("unexpected Step Functions request %s", key)
		}
		delete(responses, key)
		return http.StatusOK, body
	}))}
	inUse := make(inUseRevisions)
	if err := app.collectStepFunctionsRevisions(context.Background(), inUse); err != nil {
		t.Fatal(err)
	}
	want := inUseRevisions{"batch:1": {}, "batch:2": {}, "batch:3": {}}
	if !maps.Equal(inUse, want) {
		t.Errorf("collected revisions = %v, want %v", inUse, want)
	}
	if len(responses) != 0 {
		t.Errorf("unrequested responses: %v", responses)
	}
}

func TestCollectSchedulerRevisions(t *testing.T) {
	tests := []struct {
		name      string
		targetARN string
		detail    string
		wantGets  int
		want      inUseRevisions
	}{
		{
			name:      "ECS target collects parameters and input",
			targetARN: "arn:aws:ecs:us-east-1:123456789012:cluster/default",
			detail: `{"Target":{"EcsParameters":{"TaskDefinitionArn":` +
				`"arn:aws:ecs:us-east-1:123456789012:task-definition/batch:13"},` +
				`"Input":"{\"TaskDefinition\":\"batch:14\"}"}}`,
			wantGets: 1,
			want:     inUseRevisions{"batch:13": {}, "batch:14": {}},
		},
		{
			name:      "universal ECS target collects family-only input",
			targetARN: "arn:aws:scheduler:::aws-sdk:ecs:runTask",
			detail:    `{"Target":{"Input":"{\"TaskDefinition\":\"batch\"}"}}`,
			wantGets:  1,
			want:      inUseRevisions{"batch": {}},
		},
		{name: "skip SQS", targetARN: "arn:aws:sqs:us-east-1:123456789012:queue"},
		{name: "skip other universal targets", targetARN: "arn:aws:scheduler:::aws-sdk:lambda:invoke"},
		{name: "skip missing target"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gets := 0
			app := &App{scheduler: scheduler.NewFromConfig(fakeAWS(t, func(req *http.Request) (int, string) {
				body := "{}"
				switch req.URL.Path {
				case "/schedules":
					target := ""
					if tt.targetARN != "" {
						target = fmt.Sprintf(`,"Target":{"Arn":%q}`, tt.targetARN)
					}
					body = `{"Schedules":[{"Name":"test","GroupName":"default"` + target + `}]}`
				case "/schedules/test":
					gets++
					if tt.detail != "" {
						body = tt.detail
					}
				default:
					t.Fatalf("unexpected scheduler request %s", req.URL.Path)
				}
				return http.StatusOK, body
			}))}
			inUse := make(inUseRevisions)
			if err := app.collectSchedulerRevisions(context.Background(), inUse); err != nil {
				t.Fatal(err)
			}
			if gets != tt.wantGets {
				t.Errorf("GetSchedule calls = %d, want %d", gets, tt.wantGets)
			}
			if !maps.Equal(inUse, tt.want) {
				t.Errorf("collected revisions = %v, want %v", inUse, tt.want)
			}
		})
	}
}

func TestCollectEventBridgeRevisions(t *testing.T) {
	responses := map[string]string{
		"buses/":               `{"EventBuses":[{"Name":"default"}],"NextToken":"buses-2"}`,
		"buses/buses-2":        `{"EventBuses":[{"Name":"custom"}]}`,
		"rules/default/":       `{"Rules":[{"Name":"default-rule"}]}`,
		"rules/custom/":        `{"Rules":[{"Name":"first"}],"NextToken":"rules-2"}`,
		"rules/custom/rules-2": `{"Rules":[{"Name":"second"}]}`,
		"targets/default/default-rule/": `{"Targets":[{"EcsParameters":{` +
			`"TaskDefinitionArn":"arn:aws:ecs:us-east-1:123456789012:task-definition/batch:1"}}]}`,
		"targets/custom/first/": `{"Targets":[{"EcsParameters":{` +
			`"TaskDefinitionArn":"arn:aws:ecs:us-east-1:123456789012:task-definition/batch:2"}}],"NextToken":"targets-2"}`,
		"targets/custom/first/targets-2": `{"Targets":[{"EcsParameters":{` +
			`"TaskDefinitionArn":"arn:aws:ecs:us-east-1:123456789012:task-definition/batch:3"}}]}`,
		"targets/custom/second/": `{"Targets":[{"EcsParameters":{` +
			`"TaskDefinitionArn":"arn:aws:ecs:us-east-1:123456789012:task-definition/batch:4"}}]}`,
	}
	app := &App{eventbridge: eventbridge.NewFromConfig(fakeAWS(t, func(req *http.Request) (int, string) {
		var input map[string]string
		if err := json.NewDecoder(req.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		var key string
		switch req.Header.Get("X-Amz-Target") {
		case "AWSEvents.ListEventBuses":
			key = "buses/" + input["NextToken"]
		case "AWSEvents.ListRules":
			key = "rules/" + input["EventBusName"] + "/" + input["NextToken"]
		case "AWSEvents.ListTargetsByRule":
			key = "targets/" + input["EventBusName"] + "/" + input["Rule"] + "/" + input["NextToken"]
		default:
			t.Fatalf("unexpected EventBridge request %s", req.Header.Get("X-Amz-Target"))
		}
		body, ok := responses[key]
		if !ok {
			t.Fatalf("unexpected EventBridge request %s", key)
		}
		delete(responses, key)
		return http.StatusOK, body
	}))}
	inUse := make(inUseRevisions)
	if err := app.collectEventBridgeRevisions(context.Background(), inUse); err != nil {
		t.Fatal(err)
	}
	want := inUseRevisions{"batch:1": {}, "batch:2": {}, "batch:3": {}, "batch:4": {}}
	if !maps.Equal(inUse, want) {
		t.Errorf("collected revisions = %v, want %v", inUse, want)
	}
	if len(responses) != 0 {
		t.Errorf("unrequested responses: %v", responses)
	}
}

func TestCollectServiceRevisions(t *testing.T) {
	tests := []struct {
		name     string
		failures string
		wantErr  bool
	}{
		{name: "service deployments and task sets", failures: `[]`},
		{name: "skip missing service", failures: `[{"arn":"gone","reason":"MISSING"}]`},
		{name: "abort other failures", failures: `[{"arn":"failed","reason":"ACCESS_DENIED"}]`, wantErr: true},
		{
			name: "missing does not hide other failures",
			failures: `[{"arn":"gone","reason":"MISSING"},` +
				`{"arn":"failed","reason":"ACCESS_DENIED"}]`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &App{ecs: ecs.NewFromConfig(fakeAWS(t, func(req *http.Request) (int, string) {
				var body string
				switch req.Header.Get("X-Amz-Target") {
				case "AmazonEC2ContainerServiceV20141113.ListClusters":
					body = `{"clusterArns":["arn:aws:ecs:us-east-1:123456789012:cluster/default"]}`
				case "AmazonEC2ContainerServiceV20141113.ListServices":
					body = `{"serviceArns":["arn:aws:ecs:us-east-1:123456789012:service/default/test"]}`
				case "AmazonEC2ContainerServiceV20141113.DescribeServices":
					body = `{"services":[{"taskDefinition":"batch:3",` +
						`"deployments":[{"taskDefinition":"batch:2"}],` +
						`"taskSets":[{"taskDefinition":"batch:1"}]}],"failures":` + tt.failures + `}`
				default:
					t.Fatalf("unexpected ECS request %s", req.Header.Get("X-Amz-Target"))
				}
				return http.StatusOK, body
			}))}
			inUse := make(inUseRevisions)
			err := app.collectServiceRevisions(context.Background(), inUse)
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "ACCESS_DENIED") {
					t.Fatalf("collectServiceRevisions() error = %v, want ACCESS_DENIED", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := inUseRevisions{"batch:3": {}, "batch:2": {}, "batch:1": {}}
			if !maps.Equal(inUse, want) {
				t.Errorf("collected revisions = %v, want %v", inUse, want)
			}
		})
	}
}
