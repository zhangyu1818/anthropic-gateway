package config

import (
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	defaultListen = ":4000"

	AuthTypeXAPIKey = "x-api-key"
	AuthTypeBearer  = "bearer"
)

type Config struct {
	Listen    string       `yaml:"listen"`
	ModelList []ModelRoute `yaml:"model_list"`
	index     map[string]int
}

type ModelRoute struct {
	ModelName string         `yaml:"model_name"`
	Params    UpstreamParams `yaml:"params"`
}

type UpstreamParams struct {
	Model    string `yaml:"model"`
	APIBase  string `yaml:"api_base"`
	APIKey   string `yaml:"api_key"`
	AuthType string `yaml:"auth_type"`
}

func Load(path string) (*Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	expanded := os.ExpandEnv(string(content))
	cfg := &Config{}
	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if strings.TrimSpace(c.Listen) == "" {
		c.Listen = defaultListen
	}

	for i := range c.ModelList {
		if strings.TrimSpace(c.ModelList[i].Params.AuthType) == "" {
			c.ModelList[i].Params.AuthType = AuthTypeXAPIKey
		}
	}
}

func (c *Config) Validate() error {
	if len(c.ModelList) == 0 {
		return fmt.Errorf("model_list is required")
	}

	index := make(map[string]int, len(c.ModelList))
	for i, route := range c.ModelList {
		modelName := strings.TrimSpace(route.ModelName)
		if modelName == "" {
			return fmt.Errorf("model_list[%d].model_name is required", i)
		}
		if _, exists := index[modelName]; exists {
			return fmt.Errorf("duplicate model_name: %s", modelName)
		}

		if strings.TrimSpace(route.Params.Model) == "" {
			return fmt.Errorf("model_list[%d].params.model is required", i)
		}
		if strings.TrimSpace(route.Params.APIBase) == "" {
			return fmt.Errorf("model_list[%d].params.api_base is required", i)
		}
		u, err := url.Parse(route.Params.APIBase)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return fmt.Errorf("model_list[%d].params.api_base is invalid: %s", i, route.Params.APIBase)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("model_list[%d].params.api_base must use http/https", i)
		}
		if strings.TrimSpace(route.Params.APIKey) == "" {
			return fmt.Errorf("model_list[%d].params.api_key is required", i)
		}

		authType := strings.ToLower(strings.TrimSpace(route.Params.AuthType))
		switch authType {
		case AuthTypeXAPIKey, AuthTypeBearer:
		default:
			return fmt.Errorf("model_list[%d].params.auth_type must be x-api-key or bearer", i)
		}

		c.ModelList[i].ModelName = modelName
		c.ModelList[i].Params.AuthType = authType
		index[modelName] = i
	}

	c.index = index
	return nil
}

func (c *Config) RouteByModel(modelName string) (ModelRoute, bool) {
	idx, ok := c.index[strings.TrimSpace(modelName)]
	if !ok {
		return ModelRoute{}, false
	}
	return c.ModelList[idx], true
}

func (c *Config) ModelNames() []string {
	names := make([]string, 0, len(c.ModelList))
	for _, route := range c.ModelList {
		names = append(names, route.ModelName)
	}
	sort.Strings(names)
	return names
}
