# tdrm

A command line tool to clean up Amazon ECS task definitions.

This tool works like this:

- Keep the newest *n* active revisions that are not in use, plus every in-use revision.
- Inactivate revisions other than revisions to keep.
- Delete inactive revisions that are not in use.


This tool is heavily inspired by [tdtidy](https://github.com/manabusakai/tdtidy) and [ecrm](https://github.com/fujiwara/ecrm).

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
protected_revisions:
  - arn:aws:ecs:us-east-1:123456789012:task-definition/metabase:13
  - foo-batch:7
```

`keep_count` keeps the newest specified number of active revisions that are not
in use. In-use revisions are always kept and do not count toward this limit.
For example, `keep_count: 2` with revision 5 in use keeps revisions 5, 4, and 3.
The summary's `keep` count includes in-use revisions.

Before scanning task definitions, tdrm collects revision references in the
configured AWS region from:

- Step Functions state machine definitions and all published versions, including
  versions referenced by aliases.
- EventBridge Scheduler ECS targets, including universal `RunTask` targets,
  across all schedule groups.
- EventBridge rule ECS targets on the default and custom event buses.
- ECS services, their deployments, and task sets across all clusters.

If any collection API fails, tdrm aborts before inactivating or deleting revisions.
Family-only references protect the latest active revision even with
`keep_count: 0`, without counting toward `keep_count`.

Use the optional top-level `protected_revisions` list for references that cannot
be detected automatically. Each entry must be a full task definition ARN with a
revision or `family:revision`; invalid entries cause a configuration error.
References resolved at runtime (such as Step Functions `TaskDefinition.$`),
`RunTask` calls from Lambda or scripts, and ARNs held only in Terraform state
cannot be detected. Add those revisions to `protected_revisions`.

## Required IAM permissions

Both `plan` and `delete` require these read permissions:

- `ecs:ListTaskDefinitionFamilies`
- `ecs:ListTaskDefinitions`
- `ecs:ListClusters`
- `ecs:ListServices`
- `ecs:DescribeServices`
- `states:ListStateMachines`
- `states:ListStateMachineVersions`
- `states:DescribeStateMachine`
- `scheduler:ListSchedules`
- `scheduler:GetSchedule`
- `events:ListEventBuses`
- `events:ListRules`
- `events:ListTargetsByRule`

The `delete` command also requires:

- `ecs:DeregisterTaskDefinition`
- `ecs:DeleteTaskDefinitions`

## Author

Copyright (c) 2024 Gosuke Miyashita

## LICENSE

MIT
