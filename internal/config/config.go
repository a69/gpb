package config

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Tenants  []TenantConfig `yaml:"tenants"`
	Logging  LoggingConfig  `yaml:"logging"`
}

type ServerConfig struct {
	Listen        string `yaml:"listen"`
	WebhookPath   string `yaml:"webhook_path"`
	WebhookSecret string `yaml:"webhook_secret"`
}

type TenantConfig struct {
	Name            string `yaml:"name"`
	GitHubToken     string `yaml:"github_token"`
	GitHubProjectID string `yaml:"github_project_id"`
	BaleToken       string `yaml:"bale_token"`
	GroupChatID     string `yaml:"group_chat_id"`
	CronSpec        string `yaml:"cron_spec"`
	UrgencyDays     int    `yaml:"urgency_days"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

var envVarRE = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	data = []byte(interpolateEnv(string(data)))

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Server.Listen == "" {
		cfg.Server.Listen = ":8080"
	}
	if cfg.Server.WebhookPath == "" {
		cfg.Server.WebhookPath = "/webhook"
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
	for i := range cfg.Tenants {
		if cfg.Tenants[i].UrgencyDays == 0 {
			cfg.Tenants[i].UrgencyDays = 2
		}
		if cfg.Tenants[i].CronSpec == "" {
			cfg.Tenants[i].CronSpec = "0 9 * * *"
		}
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	if len(c.Tenants) == 0 {
		return fmt.Errorf("at least one tenant is required")
	}
	for i, t := range c.Tenants {
		if t.Name == "" {
			return fmt.Errorf("tenant %d: name is required", i)
		}
		if t.GitHubToken == "" {
			return fmt.Errorf("tenant %q: github_token is required", t.Name)
		}
		if t.GitHubProjectID == "" {
			return fmt.Errorf("tenant %q: github_project_id is required", t.Name)
		}
		if t.BaleToken == "" {
			return fmt.Errorf("tenant %q: bale_token is required", t.Name)
		}
		if t.GroupChatID == "" {
			return fmt.Errorf("tenant %q: group_chat_id is required", t.Name)
		}
	}
	return nil
}

func interpolateEnv(s string) string {
	return envVarRE.ReplaceAllStringFunc(s, func(match string) string {
		// match looks like ${VAR_NAME}
		key := match[2 : len(match)-1]
		return os.Getenv(key)
	})
}
