package tdrm

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"os"
	"regexp"
	"strings"
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
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
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

	for _, taskDef := range c.TaskDefinitions {
		familyPrefix := fmt.Sprintf("^%s$", strings.Replace(taskDef.FamilyPrefix, "*", ".*", -1))
		re := regexp.MustCompile(familyPrefix)

		for _, family := range families {
			if re.Match([]byte(family)) {
				summary, taskDefinition, err := app.scanTaskDefinition(ctx, family, taskDef.KeepCount)
				if err != nil {
					return err
				}
				summaries = append(summaries, summary)
				taskDefinitions = append(taskDefinitions, taskDefinition)
			}
		}
	}

	err = summaries.print(os.Stdout, opt.Format)
	if err != nil {
		return err
	}

	return nil
}

func (app *App) scanTaskDefinition(ctx context.Context, family string, keepCount int) (*Summary, *TaskDefinition, error) {
	summary := &Summary{TaskDefinition: family}

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
