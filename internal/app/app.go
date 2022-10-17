package app

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
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
	serviceName = "giny"

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
	conf     config.Configure
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
