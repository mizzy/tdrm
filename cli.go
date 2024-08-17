package tdrm

import (
	"sort"

	"github.com/urfave/cli/v2"
)

func (app *App) NewCLI() *cli.App {
	cliApp := &cli.App{
		Name:  "tdrm",
		Usage: "A command line tool to manage AWS ECS task definitions",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Value:   "tdrm.yaml",
				Usage:   "Load configuration from `FILE`",
				EnvVars: []string{"TDRM_CONFIG"},
			},
			&cli.StringFlag{
				Name:    "format",
				Value:   "table",
				Usage:   "plan output format (table, json)",
				EnvVars: []string{"TDRM_FORMAT"},
			},
		},
		Commands: []*cli.Command{
			app.NewPlanCommand(),
			app.NewDeleteCommand(),
		},
	}
	sort.Sort(cli.FlagsByName(cliApp.Flags))
	sort.Sort(cli.CommandsByName(cliApp.Commands))
	return cliApp
}

func (app *App) NewPlanCommand() *cli.Command {
	return &cli.Command{
		Name:  "plan",
		Usage: "List task definitions to delete.",
		Flags: []cli.Flag{&cli.StringFlag{}},
		Action: func(c *cli.Context) error {
			format, err := newOutputFormatFrom(c.String("format"))
			if err != nil {
				return err
			}
			return app.Run(
				c.Context,
				c.String("config"),
				Option{
					Format: format,
				},
			)
		},
	}
}

func (app *App) NewDeleteCommand() *cli.Command {
	return &cli.Command{
		Name:  "delete",
		Usage: "Delete task definitions.",
		Flags: []cli.Flag{&cli.StringFlag{}},
		Action: func(c *cli.Context) error {
			format, err := newOutputFormatFrom(c.String("format"))
			if err != nil {
				return err
			}
			return app.Run(
				c.Context,
				c.String("config"),
				Option{
					Delete: true,
					Format: format,
				},
			)
		},
	}
}
