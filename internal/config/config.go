package config

import "github.com/spf13/viper"

type Config struct {
	NotesDir           string `mapstructure:"notes-dir"`
	IndexPath          string `mapstructure:"index-path"`
	IndexType          string `mapstructure:"index-type"`
	LargeFileThreshold int    `mapstructure:"large-file-threshold"`
}

// Load reads configuration from viper and returns a Config struct
func Load() (*Config, error) {
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
