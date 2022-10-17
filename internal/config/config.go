package config

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"

	"github.com/docker/libkv/store"
)

const (
	ClusterMode    = "cluster"
	StandaloneMode = "standalone"
)

const (
	RedisKey = "redis"
	AdminKey = "admin"
)

var _ Configure = (*config)(nil)

type Observer interface {
	OnChanged(key string, data []byte) error
}

type Handler func(key string, data []byte) error

func (f Handler) OnChanged(key string, data []byte) error {
	return f(key, data)
}

func New(s store.Store, prefix string) Configure {
	return &config{
		store:     s,
		keyPrefix: prefix,
	}
}

type Configure interface {
	GetStoreConf(key string, data interface{}, def interface{}) error
	WatchStoreConf(key string, observer Observer) error
	WatchStoreConfTree(root string, observer Observer) error
}

type config struct {
	store     store.Store
	keyPrefix string
}

func (c *config) makeStoreKey(key string) string {
	return path.Join(c.keyPrefix, key)
}

func (c *config) GetStoreConf(key string, data interface{}, def interface{}) error {
	storeKey := c.makeStoreKey(key)
	kvPair, err := c.store.Get(storeKey)
	if err != nil {
		if !errors.Is(err, store.ErrKeyNotFound) {
			return err
		}

		// first startup
		value, err := json.MarshalIndent(def, "", "  ")
		if err != nil {
			return err
		}

		if err = c.store.Put(storeKey, value, nil); err != nil {
			return err
		}

		if kvPair, err = c.store.Get(storeKey); err != nil {
			return err
		}
	}

	err = json.Unmarshal(kvPair.Value, data)
	if err != nil {
		return err
	}

	return nil
}

func (c *config) WatchStoreConf(key string, observer Observer) error {
	stopCh := make(chan struct{}, 1)
	kvChan, err := c.store.Watch(c.makeStoreKey(key), stopCh)
	if err != nil {
		return errors.Wrapf(err, "watch store key: %v", c.makeStoreKey(key))
	}

	go func() {
		defer close(stopCh)
		for kv := range kvChan {
			// TODO: compare last index
			_ = observer.OnChanged(key, kv.Value)
		}
	}()

	return nil
}

func (c *config) WatchStoreConfTree(root string, observer Observer) error {
	stopCh := make(chan struct{}, 1)
	kvChan, err := c.store.WatchTree(c.makeStoreKey(root), stopCh)
	if err != nil {
		return err
	}

	go func() {
		defer close(stopCh)

		for kv := range kvChan {
			for _, pair := range kv {
				if pair.Value == nil {
					continue
				}

				idx := strings.LastIndex(pair.Key, root) + len(root)
				if err = observer.OnChanged(strings.TrimPrefix(pair.Key[idx:], "/"), pair.Value); err != nil {
					log.Err(err).Str("key", pair.Key).Str("value", string(pair.Value)).Msg("watch store dir")
				}
			}
		}
	}()

	return nil
}

type AdminConf struct {
	RootPass      string `json:"rootPass"`
	Secret        string `json:"secret"`
	TokenExpire   int8   `json:"tokenExpire"`
	RefreshExpire int8   `json:"refreshExpire"`
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
