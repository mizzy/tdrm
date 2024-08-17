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
	for _, taskDef := range c.TaskDefinitions {
		familyPrefix := fmt.Sprintf("^%s$", strings.Replace(taskDef.FamilyPrefix, "*", ".*", -1))
		re := regexp.MustCompile(familyPrefix)

		for _, family := range families {
			if re.Match([]byte(family)) {
				summary, err := app.scanTaskDefinition(ctx, family, opt)
				if err != nil {
					return err
				}
				summaries = append(summaries, summary)
			}
		}
	}

	return summaries.print(os.Stdout, opt.Format)
}

func (app *App) scanTaskDefinition(ctx context.Context, family string, opt Option) (*Summary, error) {
	summary := &Summary{TaskDefinition: family}

	p := ecs.NewListTaskDefinitionsPaginator(app.ecs, &ecs.ListTaskDefinitionsInput{
		FamilyPrefix: &family,
		Status:       types.TaskDefinitionStatusActive,
	})

	var activeRevisions []*types.TaskDefinition

	for p.HasMorePages() {
		res, err := p.NextPage(ctx)
		if err != nil {
			return nil, err
		}

		for _, tdArn := range res.TaskDefinitionArns {
			td, err := app.ecs.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
				TaskDefinition: &tdArn,
			})

			if err != nil {
				return nil, err
			}
			activeRevisions = append(activeRevisions, td.TaskDefinition)
		}
	}

	summary.ActiveRevisions = len(activeRevisions)

	return summary, nil
}
