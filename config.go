package tdrm

import (
	"errors"
	"io"
	"os"

	"github.com/goccy/go-yaml"
)

type Config struct {
	TaskDefinitions []*TaskdefConfig `yaml:"task_definitions"`
}

type TaskdefConfig struct {
	FamilyPrefix *string `yaml:"family_prefix,omitempty"`
	KeepCount    *int    `yaml:"keep_count,omitempty"`
}

func LoadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	c := &Config{}
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, err
	}

	for _, taskDef := range c.TaskDefinitions {
		if taskDef.FamilyPrefix == nil {
			return nil, errors.New("family_prefix is required")
		}

		if taskDef.KeepCount == nil {
			return nil, errors.New("keep_count is required")
		}
	}

	return c, nil
}
