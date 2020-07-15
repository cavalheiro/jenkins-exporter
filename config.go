package main

import (
	"github.com/BurntSushi/toml"
)

// Config stores the values read from the TOML config
type Config struct {
	Jenkins jenkins
}

type jenkins struct {
	URL            string
	User           string
	Password       string
	Jobs           []string
	UpdateInterval uint64
}

// Load configuration
func loadConfig(pathToConfig string) (Config, error) {
	var config Config
	_, err := toml.DecodeFile(pathToConfig, &config)
	return config, err
}
