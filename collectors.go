package tdrm

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/scheduler"
	"github.com/aws/aws-sdk-go-v2/service/sfn"
)

func (app *App) collectStepFunctionsRevisions(ctx context.Context, inUse inUseRevisions) error {
	log.Printf("[info] getting in-use revisions from Step Functions\n")
	p := sfn.NewListStateMachinesPaginator(app.sfn, &sfn.ListStateMachinesInput{})
	for p.HasMorePages() {
		res, err := p.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, machine := range res.StateMachines {
			if err := app.collectStateMachineDefinition(ctx, machine.StateMachineArn, inUse); err != nil {
				return err
			}
			// The SDK does not provide a paginator for state machine versions.
			input := &sfn.ListStateMachineVersionsInput{StateMachineArn: machine.StateMachineArn}
			for {
				versions, err := app.sfn.ListStateMachineVersions(ctx, input)
				if err != nil {
					return err
				}
				for _, version := range versions.StateMachineVersions {
					if err := app.collectStateMachineDefinition(ctx, version.StateMachineVersionArn, inUse); err != nil {
						return err
					}
				}
				if aws.ToString(versions.NextToken) == "" {
					break
				}
				input.NextToken = versions.NextToken
			}
		}
	}
	return nil
}

func (app *App) collectStateMachineDefinition(ctx context.Context, arn *string, inUse inUseRevisions) error {
	res, err := app.sfn.DescribeStateMachine(ctx, &sfn.DescribeStateMachineInput{StateMachineArn: arn})
	if err != nil {
		return err
	}
	for _, ref := range extractTaskDefinitionRefs(aws.ToString(res.Definition)) {
		inUse.add(ref)
	}
	return nil
}

func (app *App) collectSchedulerRevisions(ctx context.Context, inUse inUseRevisions) error {
	log.Printf("[info] getting in-use revisions from EventBridge Scheduler\n")
	p := scheduler.NewListSchedulesPaginator(app.scheduler, &scheduler.ListSchedulesInput{})
	for p.HasMorePages() {
		res, err := p.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, schedule := range res.Schedules {
			if schedule.Target == nil || !strings.Contains(aws.ToString(schedule.Target.Arn), ":ecs:") {
				continue
			}
			detail, err := app.scheduler.GetSchedule(ctx, &scheduler.GetScheduleInput{
				Name:      schedule.Name,
				GroupName: schedule.GroupName,
			})
			if err != nil {
				return err
			}
			if detail.Target != nil {
				if detail.Target.EcsParameters != nil {
					inUse.add(aws.ToString(detail.Target.EcsParameters.TaskDefinitionArn))
				}
				for _, ref := range extractTaskDefinitionRefs(aws.ToString(detail.Target.Input)) {
					inUse.add(ref)
				}
			}
		}
	}
	return nil
}

func (app *App) collectEventBridgeRevisions(ctx context.Context, inUse inUseRevisions) error {
	log.Printf("[info] getting in-use revisions from EventBridge rules\n")
	// EventBridge has no SDK paginators. ListEventBuses includes the default bus.
	input := &eventbridge.ListEventBusesInput{}
	for {
		res, err := app.eventbridge.ListEventBuses(ctx, input)
		if err != nil {
			return err
		}
		for _, bus := range res.EventBuses {
			if err := app.collectEventBusRevisions(ctx, bus.Name, inUse); err != nil {
				return err
			}
		}
		if aws.ToString(res.NextToken) == "" {
			break
		}
		input.NextToken = res.NextToken
	}
	return nil
}

func (app *App) collectEventBusRevisions(ctx context.Context, bus *string, inUse inUseRevisions) error {
	input := &eventbridge.ListRulesInput{EventBusName: bus}
	for {
		res, err := app.eventbridge.ListRules(ctx, input)
		if err != nil {
			return err
		}
		for _, rule := range res.Rules {
			targetInput := &eventbridge.ListTargetsByRuleInput{Rule: rule.Name, EventBusName: bus}
			for {
				targets, err := app.eventbridge.ListTargetsByRule(ctx, targetInput)
				if err != nil {
					return err
				}
				for _, target := range targets.Targets {
					if target.EcsParameters != nil {
						inUse.add(aws.ToString(target.EcsParameters.TaskDefinitionArn))
					}
				}
				if aws.ToString(targets.NextToken) == "" {
					break
				}
				targetInput.NextToken = targets.NextToken
			}
		}
		if aws.ToString(res.NextToken) == "" {
			break
		}
		input.NextToken = res.NextToken
	}
	return nil
}

func (app *App) collectServiceRevisions(ctx context.Context, inUse inUseRevisions) error {
	log.Printf("[info] getting in-use revisions from ECS services\n")
	clusters := ecs.NewListClustersPaginator(app.ecs, &ecs.ListClustersInput{})
	for clusters.HasMorePages() {
		res, err := clusters.NextPage(ctx)
		if err != nil {
			return err
		}
		for _, cluster := range res.ClusterArns {
			services := ecs.NewListServicesPaginator(app.ecs, &ecs.ListServicesInput{Cluster: aws.String(cluster)})
			for services.HasMorePages() {
				page, err := services.NextPage(ctx)
				if err != nil {
					return err
				}
				for i := 0; i < len(page.ServiceArns); i += 10 {
					end := min(i+10, len(page.ServiceArns))
					detail, err := app.ecs.DescribeServices(ctx, &ecs.DescribeServicesInput{
						Cluster:  aws.String(cluster),
						Services: page.ServiceArns[i:end],
					})
					if err != nil {
						return err
					}
					for _, failure := range detail.Failures {
						if aws.ToString(failure.Reason) != "MISSING" {
							return fmt.Errorf("describe service %s in %s: %s", aws.ToString(failure.Arn), cluster, aws.ToString(failure.Reason))
						}
					}
					for _, service := range detail.Services {
						inUse.add(aws.ToString(service.TaskDefinition))
						for _, deployment := range service.Deployments {
							inUse.add(aws.ToString(deployment.TaskDefinition))
						}
						for _, taskSet := range service.TaskSets {
							inUse.add(aws.ToString(taskSet.TaskDefinition))
						}
					}
				}
			}
		}
	}
	return nil
}
