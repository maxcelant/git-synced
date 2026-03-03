package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type ProviderConfig struct {
	Name          string   `yaml:"name"`
	Token         string   `yaml:"token"`
	BaseURL       string   `yaml:"base_url"`
	LookbackHours int      `yaml:"lookback_hours"`
	State         string   `yaml:"state"`
	Authors       []string `yaml:"authors"`
	Repos         []string `yaml:"repos"`
}

type Config struct {
	Format    string           `yaml:"format"`
	Providers []ProviderConfig `yaml:"providers"`
}

func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("opening config: %w", err)
	}
	defer f.Close()

	var cfg Config
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}

	if cfg.Format == "" {
		cfg.Format = "yaml"
	}
	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		if p.BaseURL == "" {
			p.BaseURL = "https://gitlab.com"
		}
		if p.LookbackHours <= 0 {
			p.LookbackHours = 24
		}
		if p.State == "" {
			p.State = "opened"
		}
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if len(c.Providers) == 0 {
		return fmt.Errorf("at least one provider is required in config")
	}
	for _, p := range c.Providers {
		if p.Token == "" {
			return fmt.Errorf("provider %q: token is required", p.Name)
		}
		if len(p.Authors) == 0 {
			return fmt.Errorf("provider %q: at least one author is required", p.Name)
		}
		if len(p.Repos) == 0 {
			return fmt.Errorf("provider %q: at least one repo is required", p.Name)
		}
	}
	return nil
}
