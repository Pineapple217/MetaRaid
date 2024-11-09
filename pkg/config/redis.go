package config

type Redis struct {
	Database     int    `yaml:"database"`
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	PoolSize     int    `yaml:"poolSize"`
	MinIdleConns int    `yaml:"minIdleConns"`
}

func (r *Redis) SetDefault() {
	r.Port = 6379
	r.Host = "127.0.0.1"
	r.Database = 0
	r.PoolSize = 10
	r.MinIdleConns = 3
}
