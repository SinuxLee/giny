package app

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/docker/libkv/store"
	"github.com/go-redis/redis/v8"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"github.com/sinuxlee/giny/internal/config"
	"golang.org/x/sync/errgroup"
)

var (
	serverName = "giny"

	storeAddrKey = "store"
	storeAddrDef = "127.0.0.1:6379"

	logLevelKey = "loglevel"
	logLevelDef = "info"

	storePrefixKey = "prefix"
	storePrefixDef = ""

	printVersionKey = "version"
	printVersionDef = false

	redisClusterKey = "cluster"
	redisClusterDef = false

	adminKey = "admin"
	adminDef = false
)

func init() {
	flag.StringVar(&storeAddrDef, storeAddrKey, storeAddrDef, "the kv store address")
	flag.StringVar(&logLevelDef, logLevelKey, logLevelDef, "log level")
	flag.StringVar(&storePrefixDef, storePrefixKey, storePrefixDef, "store key prefix")
	flag.BoolVar(&printVersionDef, printVersionKey, printVersionDef, "print program build version")
	flag.BoolVar(&redisClusterDef, redisClusterKey, redisClusterDef, "connect to redis cluster")
	flag.BoolVar(&adminDef, adminKey, adminDef, "enable api admin")

	flag.Parse()
}

// Option ...
type Option func(*app) error

// App ...
type App interface {
	Run() error
	Stop() error
}

// New ...
func New(opts ...Option) (App, error) {
	svc := &app{}

	// init app component
	for _, o := range opts {
		if err := o(svc); err != nil {
			return nil, err
		}
	}

	return svc, nil
}

type app struct {
	g        *errgroup.Group
	c        context.Context
	nodeID   int
	redisCli redis.UniversalClient
	kvStore  store.Store
	web      http.Server
}

// Run ...
func (a *app) Run() error {
	a.g, a.c = errgroup.WithContext(context.Background())
	a.g.Go(func() error {
		return a.web.ListenAndServe()
	})

	log.Info().Msg("giny start successfully")

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	<-ch
	signal.Stop(ch)
	close(ch)

	return nil
}

// Stop ...
func (a *app) Stop() error {
	ctx, cancel := context.WithTimeout(a.c, 10*time.Second)
	defer cancel()

	if err := a.web.Shutdown(ctx); err != nil && !errors.Is(err, context.Canceled) {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
	}

	if err := a.g.Wait(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
	}
	return nil
}

func (a *app) makeStoreKey(key string) string {
	if storePrefixDef == "" {
		return fmt.Sprintf("%v/%v", serverName, key)
	}
	return fmt.Sprintf("%v/%v/%v", storePrefixDef, serverName, key)
}

func (a *app) getStoreConf(key string, data interface{}, def interface{}) error {
	storeKey := a.makeStoreKey(key)
	kvPair, err := a.kvStore.Get(storeKey)
	if err != nil {
		if !errors.Is(err, store.ErrKeyNotFound) || a.nodeID > 1 {
			return err
		}

		// first startup
		value, err := json.MarshalIndent(def, "", "  ")
		if err != nil {
			return err
		}

		if err = a.kvStore.Put(storeKey, value, nil); err != nil {
			return err
		}

		if kvPair, err = a.kvStore.Get(storeKey); err != nil {
			return err
		}
	}

	err = json.Unmarshal(kvPair.Value, data)
	if err != nil {
		return err
	}

	return nil
}

func (a *app) watchStoreConf(key string, observer config.Handler) error {
	stopCh := make(chan struct{}, 1)
	kvChan, err := a.kvStore.Watch(a.makeStoreKey(key), stopCh)
	if err != nil {
		return errors.Wrapf(err, "watch store key: %v", a.makeStoreKey(key))
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

func (a *app) watchStoreConfTree(root string, observer config.Handler) error {
	stopCh := make(chan struct{}, 1)
	kvChan, err := a.kvStore.WatchTree(a.makeStoreKey(root), stopCh)
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
