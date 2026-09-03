package config

import (
	"errors"
	"path"

	"github.com/spf13/viper"
)

func Load(configPath string, cfg *Config) error {
	configName := path.Base(configPath)
	ext := path.Ext(configPath)
	dir := path.Dir(configPath)

	if configPath == "" {
		return errors.New("config path is empty")
	}
	if ext != ".yaml" {
		return errors.New("only accept .yaml file")
	}

	v := viper.New()
	v.SetConfigType("yaml")
	v.SetConfigName(configName)
	v.AddConfigPath(dir)

	if err := v.ReadInConfig(); err != nil {
		return err
	}

	// Unmarshal the configuration into the struct
	if err := v.Unmarshal(cfg); err != nil {
		return err
	}

	return nil
}
