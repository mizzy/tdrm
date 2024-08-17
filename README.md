# tdrm

A command line tool to clean up Amazon ECS task definitions.

This tool works like this:

- Keep *n* active revisions.
- Inactivate revisions other than revisions to keep.
- Delete inactive revisions.

## Usage

```
NAME:
   tdrm - A command line tool to manage AWS ECS task definitions

USAGE:
   tdrm [global options] command [command options]

VERSION:
   current

COMMANDS:
   delete   Delete task definitions.
   plan     List task definitions to delete.
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --config FILE, -c FILE  Load configuration from FILE (default: "tdrm.yaml") [$TDRM_CONFIG]
   --format value          plan output format (table, json) (default: "table") [$TDRM_FORMAT]
   --help, -h              show help
   --version, -v           print the version
```

## Configurations

Configuration file is YAML format.The default file name is `tdrm.yaml`.

```yaml
task_definitions:
  - family_prefix: metabase
    keep_count: 10
  - family_prefix: foo*
    keep_count: 20
  - family_prefix: bar*
    keep_count: 30
```

## Author

Copyright (c) 2024 Gosuke Miyashita

## LICENSE

MIT
