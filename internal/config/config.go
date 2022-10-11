package config

import (
	"fmt"
)

const (
	ClusterMode    = "cluster"
	StandaloneMode = "standalone"
)

const (
	RedisKey = "redis"
)

type Observer interface {
	OnChanged(key string, data []byte) error
}

type Handler func(key string, data []byte) error

func (f Handler) OnChanged(key string, data []byte) error {
	return f(key, data)
}

type RedisConf struct {
	Mode     string `json:"mode"`     // redis mode (cluster/standalone)
	Addr     string `json:"addr"`     // redis address
	Username string `json:"username"` // username for login redis
	Password string `json:"password"` // password for login redis
}

func (r *RedisConf) String() string {
	return fmt.Sprintf("mode:%v addr:%v password:%v", r.Mode, r.Addr, r.Password)
}

type WebConf struct {
	GinMode string   `json:"ginMode"`
	Port    string   `json:"port"`
	Filter  []string `json:"filter"`
}
