package app

import (
	"flag"
	"sync"

	"github.com/go-redis/redis/v8"
	"github.com/rs/zerolog/log"
)

var (
	consulAddrKey = "consul"
	consulAddrDef = "127.0.0.1:8500"

	logLevelKey = "loglevel"
	logLevelDef = "info"

	printVersionKey = "version"
	printVersionDef = false
)

func init() {
	flag.StringVar(&consulAddrDef, consulAddrKey, "127.0.0.1:8500", "the consul address")
	flag.StringVar(&logLevelDef, logLevelKey, "info", "log level")
	flag.BoolVar(&printVersionDef, printVersionKey, false, "print program build version")

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
	sync.WaitGroup
	localIP string
	goEnv   string
	nodeID  int
	quit    chan bool
	reci    redis.UniversalClient
}

// Run ...
func (a *app) Run() error {
	log.Info().Msg("giny start successfully")

	return nil
}

// Stop ...
func (a *app) Stop() error {
	return nil
}
