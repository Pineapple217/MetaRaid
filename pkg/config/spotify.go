package config

import "time"

type Spotify struct {
	Clients []struct {
		ClientId     string `yaml:"clientId"`
		ClientSecret string `yaml:"clientSecret"`
		Name         string `yaml:"name"`
	} `yaml:"clients"`
	MaxRetryDuration time.Duration `yaml:"maxRetryDuration"`
}

func (s *Spotify) SetDefault() {
	s.MaxRetryDuration = time.Hour
}
