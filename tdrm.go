package tdrm

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
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

	for _, taskDef := range c.TaskDefinitions {
		familyPrefix := fmt.Sprintf("^%s$", strings.Replace(taskDef.FamilyPrefix, "*", ".*", -1))
		re := regexp.MustCompile(familyPrefix)

		for _, family := range families {
			if re.Match([]byte(family)) {
				err = app.scanTaskDefinitions(ctx, family, opt)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (app *App) scanTaskDefinitions(ctx context.Context, family string, opt Option) error {
	p := ecs.NewListTaskDefinitionsPaginator(app.ecs, &ecs.ListTaskDefinitionsInput{
		FamilyPrefix: &family,
		Status:       types.TaskDefinitionStatusActive,
	})

	var activeTaskDefinitions []*types.TaskDefinition

	for p.HasMorePages() {
		res, err := p.NextPage(ctx)
		if err != nil {
			return err
		}

		for _, tdArn := range res.TaskDefinitionArns {
			td, err := app.ecs.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
				TaskDefinition: &tdArn,
			})
			if err != nil {
				return err
			}
			activeTaskDefinitions = append(activeTaskDefinitions, td.TaskDefinition)
		}
	}

	return nil
}
