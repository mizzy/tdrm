package tdrm

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/Songmu/prompter"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

type App struct {
	ecs    *ecs.Client
	region string
}

type Option struct {
	Delete bool
	Force  bool
	Format outputFormat
}

type TaskDefinition struct {
	Family     string
	ToInactive []string
	ToDelete   []string
}

func New(ctx context.Context, region string) (*App, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region), config.WithRetryer(func() aws.Retryer {
		return retry.AddWithMaxAttempts(retry.NewStandard(), 10)
	}))
	if err != nil {
		return nil, err
	}

	return &App{
		region: cfg.Region,
		ecs:    ecs.NewFromConfig(cfg),
	}, nil
}

func (app *App) Run(ctx context.Context, path string, opt Option) error {
	c, err := LoadConfig(path)
	if err != nil {
		return err
	}

	var families []string

	p := ecs.NewListTaskDefinitionFamiliesPaginator(app.ecs, &ecs.ListTaskDefinitionFamiliesInput{})
	for p.HasMorePages() {
		res, err := p.NextPage(ctx)
		if err != nil {
			return err
		}

		for _, family := range res.Families {
			families = append(families, family)
		}
	}

	summaries := SummaryTable{}
	var taskDefinitions []*TaskDefinition

	for _, taskDefConfig := range c.TaskDefinitions {
		familyPrefix := fmt.Sprintf("^%s$", strings.Replace(*taskDefConfig.FamilyPrefix, "*", ".*", -1))
		re := regexp.MustCompile(familyPrefix)

		for _, family := range families {
			if re.Match([]byte(family)) {
				scanned := false
				for _, taskDef := range taskDefinitions {
					if taskDef.Family == family {
						scanned = true
					}
				}

				if !scanned {
					summary, taskDefinition, err := app.scanTaskDefinition(ctx, family, *taskDefConfig.KeepCount)
					if err != nil {
						return err
					}
					summaries = append(summaries, summary)
					taskDefinitions = append(taskDefinitions, taskDefinition)
				}
			}
		}
	}

	err = summaries.print(os.Stdout, opt.Format)
	if err != nil {
		return err
	}

	if !opt.Delete {
		return nil
	}

	for _, taskDef := range taskDefinitions {
		if !opt.Force && len(taskDef.ToInactive) > 0 {
			if !prompter.YN(fmt.Sprintf("Do you inactivate %d revisions on %s?", len(taskDef.ToInactive), taskDef.Family), false) {
				return errors.New("aborted")
			}
		}

		for _, toInactive := range taskDef.ToInactive {
			parts := strings.Split(toInactive, "/")
			revision := parts[len(parts)-1]
			log.Printf("[info] inactivating %s\n", revision)
			_, err = app.ecs.DeregisterTaskDefinition(ctx, &ecs.DeregisterTaskDefinitionInput{
				TaskDefinition: aws.String(toInactive),
			})
			if err != nil {
				return err
			}
		}

		if !opt.Force && len(taskDef.ToDelete) > 0 {
			if !prompter.YN(fmt.Sprintf("Do you delete %d revisions on %s?", len(taskDef.ToDelete), taskDef.Family), false) {
				return errors.New("aborted")
			}
		}

		chunkSize := 10
		for i := 0; i < len(taskDef.ToDelete); i += chunkSize {
			end := i + chunkSize
			if end > len(taskDef.ToDelete) {
				end = len(taskDef.ToDelete)
			}

			chunk := taskDef.ToDelete[i:end]

			log.Printf("[info] deleting %d / %d of %s\n", end, len(taskDef.ToDelete), taskDef.Family)
			_, err = app.ecs.DeleteTaskDefinitions(ctx, &ecs.DeleteTaskDefinitionsInput{
				TaskDefinitions: chunk,
			})
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (app *App) scanTaskDefinition(ctx context.Context, family string, keepCount int) (*Summary, *TaskDefinition, error) {
	summary := &Summary{TaskDefinition: family}

	log.Printf("[info] getting revisions of task definition %s\n", family)

	activeRevisions, err := app.getRevisions(ctx, family, types.TaskDefinitionStatusActive)
	if err != nil {
		return nil, nil, err
	}

	inactiveRevisions, err := app.getRevisions(ctx, family, types.TaskDefinitionStatusInactive)
	if err != nil {
		return nil, nil, err
	}

	taskDef := &TaskDefinition{
		Family:   family,
		ToDelete: inactiveRevisions,
	}

	if len(activeRevisions) > keepCount {
		taskDef.ToInactive = activeRevisions[keepCount:]
	}

	summary.ActiveRevisions = len(activeRevisions)
	summary.InactiveRevisions = len(inactiveRevisions)
	summary.ToInactive = len(taskDef.ToInactive)
	summary.ToDelete = len(taskDef.ToDelete)
	summary.Keep = summary.ActiveRevisions - summary.ToInactive

	return summary, taskDef, nil
}

func (app *App) getRevisions(ctx context.Context, family string, status types.TaskDefinitionStatus) ([]string, error) {
	p := ecs.NewListTaskDefinitionsPaginator(app.ecs, &ecs.ListTaskDefinitionsInput{
		FamilyPrefix: &family,
		Status:       status,
		Sort:         types.SortOrderDesc,
	})

	var revisions []string
	for p.HasMorePages() {
		res, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, tdArn := range res.TaskDefinitionArns {
			revisions = append(revisions, tdArn)
		}
	}

	return revisions, nil
}
