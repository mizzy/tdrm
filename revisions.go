package tdrm

import (
	"encoding/json"
	"regexp"
	"strings"
)

type inUseRevisions map[string]struct{}

const taskDefinitionARNPrefix = `arn:aws[a-z-]*:ecs:[a-z0-9-]+:\d{12}:task-definition/`

var (
	revisionKeyPattern       = regexp.MustCompile(`^(?:` + taskDefinitionARNPrefix + `)?([A-Za-z0-9_-]+:\d+)$`)
	familyKeyPattern         = regexp.MustCompile(`^(?:` + taskDefinitionARNPrefix + `)?([A-Za-z0-9_-]+)$`)
	taskDefinitionARNPattern = regexp.MustCompile(taskDefinitionARNPrefix + `[A-Za-z0-9_-]+:\d+`)
)

func revisionKey(ref string) (string, bool) {
	match := revisionKeyPattern.FindStringSubmatch(ref)
	if match == nil {
		return "", false
	}
	return match[1], true
}

func extractTaskDefinitionRefs(doc string) []string {
	var root any
	if err := json.Unmarshal([]byte(doc), &root); err != nil {
		return taskDefinitionARNPattern.FindAllString(doc, -1)
	}
	var refs []string
	var walk func(any)
	walk = func(node any) {
		switch node := node.(type) {
		case map[string]any:
			for key, value := range node {
				if key == "TaskDefinition" {
					if ref, ok := value.(string); ok {
						refs = append(refs, ref)
					}
				}
				walk(value)
			}
		case []any:
			for _, value := range node {
				walk(value)
			}
		case string:
			refs = append(refs, taskDefinitionARNPattern.FindAllString(node, -1)...)
		}
	}
	walk(root)
	return refs
}

func (inUse inUseRevisions) add(ref string) {
	if key, ok := revisionKey(ref); ok {
		inUse[key] = struct{}{}
		return
	}
	if match := familyKeyPattern.FindStringSubmatch(ref); match != nil {
		inUse[match[1]] = struct{}{}
	}
}

func selectToInactive(active []string, keepCount int, inUse inUseRevisions) []string {
	var toInactive []string
	kept := 0
	for i, ref := range active {
		key, _ := revisionKey(ref)
		if _, protected := inUse[key]; protected {
			continue
		}
		if i == 0 {
			family, _, _ := strings.Cut(key, ":")
			if _, protected := inUse[family]; protected {
				continue
			}
		}
		if kept < keepCount {
			kept++
			continue
		}
		toInactive = append(toInactive, ref)
	}
	return toInactive
}

func selectToDelete(inactive []string, inUse inUseRevisions) []string {
	var toDelete []string
	for _, ref := range inactive {
		key, _ := revisionKey(ref)
		if _, protected := inUse[key]; !protected {
			toDelete = append(toDelete, ref)
		}
	}
	return toDelete
}
