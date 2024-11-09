package config

import (
	"log/slog"
	"strings"

	"github.com/knadh/koanf"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/mitchellh/mapstructure"
)

type Config struct {
	Redis   Redis   `yaml:"redis"`
	Scraper Scraper `yaml:"scraper"`
	Spotify Spotify `yaml:"spotify"`
}

func (c *Config) SetDefault() {
	c.Redis.SetDefault()
	c.Scraper.SetDefault()
	c.Spotify.SetDefault()
}

func Load() (Config, error) {
	slog.Info("Loading configs")
	k := koanf.New(".")

	var conf Config
	conf.SetDefault()

	err := k.Load(file.Provider("./config.yaml"), yaml.Parser())
	if err != nil {
		return Config{}, err
	}

	unmarshalCfg := koanf.UnmarshalConf{
		Tag:       "yaml",
		FlatPaths: false,
		DecoderConfig: &mapstructure.DecoderConfig{
			DecodeHook: mapstructure.ComposeDecodeHookFunc(
				mapstructure.StringToTimeDurationHookFunc()),
			Metadata:         nil,
			Result:           &conf,
			WeaklyTypedInput: true,
			ErrorUnused:      true,
			TagName:          "yaml",
		},
	}

	err = k.UnmarshalWithConf("", &conf, unmarshalCfg)
	if err != nil {
		return Config{}, err
	}

	err = k.Load(env.ProviderWithValue("", ".", func(s string, v string) (string, any) {
		key := strings.ReplaceAll(strings.ToLower(s), "_", ".")
		// Check to exist if we have a configuration option already and see if it's a slice
		// If there is a comma in the value, split the value into a slice by the comma.
		if strings.Contains(v, ",") {
			return key, strings.Split(v, ",")
		}

		// Otherwise return the new key with the unaltered value
		return key, v
	}), nil)
	if err != nil {
		return Config{}, err
	}

	keys := make(map[string]any, len(k.Keys()))
	for _, key := range k.Keys() {
		keys[strings.ToLower(key)] = k.Get(key)
	}
	k.Delete("")
	err = k.Load(confmap.Provider(keys, "."), nil)
	if err != nil {
		return Config{}, err
	}

	unmarshalCfg.DecoderConfig.ErrorUnused = false
	unmarshalCfg.DecoderConfig.ZeroFields = true // Empty default slices/maps if a value is configured
	err = k.UnmarshalWithConf("", &conf, unmarshalCfg)
	if err != nil {
		return Config{}, err
	}

	return conf, nil
}
