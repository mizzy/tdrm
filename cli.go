package tdrm

import (
	"sort"

	"github.com/urfave/cli/v2"
)

func (app *App) NewCLI() *cli.App {
	cliApp := &cli.App{
		Name:  "tdrm",
		Usage: "A command line tool to manage AWS ECS task definitions",
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
			return app.Run(
				c.Context,
				c.String("config"),
				Option{},
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
			return app.Run(
				c.Context,
				c.String("config"),
				Option{Delete: true},
			)
		},
	}
}
