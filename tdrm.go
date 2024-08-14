package tdrm

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
)

type App struct {
	ecs    *ecs.Client
	region string
}

type Option struct {
	Delete bool
	Force  bool
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

	for _, taskDef := range c.TaskDefinitions {
		familyPrefix := taskDef.FamilyPrefix

		if *familyPrefix == "*" {
			familyPrefix = nil
		}

		p := ecs.NewListTaskDefinitionsPaginator(app.ecs, &ecs.ListTaskDefinitionsInput{
			FamilyPrefix: familyPrefix,
		})

		for p.HasMorePages() {
			res, err := p.NextPage(ctx)
			if err != nil {
				return err
			}

			for _, tdArn := range res.TaskDefinitionArns {
				fmt.Println(tdArn)
			}
		}

	}

	return nil
}
