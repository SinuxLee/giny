package app

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	helmet "github.com/danielkov/gin-helmet"
	"github.com/docker/libkv"
	"github.com/docker/libkv/store"
	"github.com/duke-git/lancet/netutil"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	limits "github.com/gin-contrib/size"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sinuxlee/giny/internal/admin"
	"github.com/sinuxlee/giny/internal/config"
	"github.com/sinuxlee/giny/pkg/filter"
	"github.com/sinuxlee/giny/pkg/kv"
	"github.com/sinuxlee/giny/pkg/nid"
)

const (
	timeFormat = "2006-01-02 15:04:05.000"
)

// Version ...
func Version(info string) Option {
	return func(a *app) error {
		if printVersionDef {
			_, _ = fmt.Fprintf(os.Stderr, "%v build info: %v\n", serverName,
				strings.ReplaceAll(info, "_", "\n"))
			os.Exit(0)
		}

		return nil
	}
}

// NodeID ...
func NodeID() Option {
	return func(a *app) error {
		path, _ := os.Getwd()
		named, err := nid.NewRedisNamed(storeAddrDef)
		if err != nil {
			return err
		}

		if a.nodeID, err = named.GetNodeID(&nid.NameHolder{
			LocalPath:  path,
			LocalIP:    netutil.GetInternalIp(),
			ServiceKey: serverName,
		}); err != nil {
			return err
		}

		if a.nodeID == 0 {
			err = errors.New("can't get node id from store")
			return err
		}

		return nil
	}
}

// Logger ...
func Logger() Option {
	return func(a *app) error {
		level, err := zerolog.ParseLevel(logLevelDef)
		if err != nil {
			level = zerolog.DebugLevel
		}

		zerolog.TimestampFieldName = "ts"
		zerolog.MessageFieldName = "msg"
		zerolog.LevelFieldName = "lvl"
		zerolog.TimeFieldFormat = timeFormat
		simpleHook := zerolog.HookFunc(func(e *zerolog.Event, level zerolog.Level, msg string) {
			if _, file, line, ok := runtime.Caller(4); ok {
				// 取文件名
				idx := strings.LastIndexByte(file, '/')
				if idx == -1 {
					e.Str("file", fmt.Sprintf("%s:%d", file, line))
					return
				}

				// 取包名
				idx = strings.LastIndexByte(file[:idx], '/')
				if idx == -1 {
					e.Str("file", fmt.Sprintf("%s:%d", file[:idx], line))
					return
				}

				// 返回包名和文件名
				e.Str("file", fmt.Sprintf("%s:%d", file[idx+1:], line))
			}
		})

		log.Logger = zerolog.New(os.Stdout).Level(level).Hook(simpleHook).With().Timestamp().
			Int("id", a.nodeID).Int("pid", os.Getpid()).Logger()
		log.Info().Msg("Init logger successfully.")

		return nil
	}
}

// KVStore ...
func KVStore() Option {
	return func(a *app) (err error) {
		if a.kvStore, err = libkv.NewStore(
			kv.StoreRedis,
			[]string{storeAddrDef},
			&store.Config{
				ConnectionTimeout: 10 * time.Second,
			}); err != nil {
			return errors.Wrap(err, "new kv store")
		}

		return nil
	}
}

// Redis ...
func Redis() Option {
	return func(a *app) error {
		conf := &config.RedisConf{}
		if err := a.getStoreConf(config.RedisKey, conf, &config.RedisConf{
			Mode:     config.StandaloneMode,
			Addr:     "127.0.0.1:6379",
			Username: "",
			Password: "",
		}); err != nil {
			return errors.Wrap(err, "get redis config")
		}

		if conf.Mode == config.ClusterMode {
			a.redisCli = redis.NewClusterClient(&redis.ClusterOptions{
				Addrs:    []string{conf.Addr},
				PoolSize: 100,
				Username: conf.Username,
				Password: conf.Password,
			})
		} else {
			a.redisCli = redis.NewClient(&redis.Options{
				Addr:     conf.Addr,
				PoolSize: 100,
				Username: conf.Username,
				Password: conf.Password,
			})
		}

		return nil
	}
}

// Handler ...
func Handler() Option {
	return func(a *app) error {
		var r *gin.Engine
		conf := &config.WebConf{}
		if err := a.getStoreConf("web", conf, &config.WebConf{
			GinMode: "debug",
			Port:    ":8086",
			Filter: []string{
				"cors",
				"gzip",
				"limit",
			},
		}); err != nil {
			return errors.Wrap(err, "get web config")
		}

		gin.SetMode(conf.GinMode)
		switch gin.Mode() {
		case gin.DebugMode:
			r = gin.Default()
		case gin.TestMode:
			r = gin.New()
			r.Use(gin.LoggerWithWriter(gin.DefaultErrorWriter))
			r.Use(gin.Recovery())
		case gin.ReleaseMode:
			fallthrough
		default:
			r = gin.New()
			r.Use(gin.Recovery())
		}

		filters := map[string]gin.HandlerFunc{
			"cors":  cors.Default(),
			"gzip":  gzip.Gzip(gzip.DefaultCompression, gzip.WithDecompressFn(gzip.DefaultDecompressHandle)),
			"limit": limits.RequestSizeLimiter(http.DefaultMaxHeaderBytes / 4), // 256KB
		}

		middle := make([]gin.HandlerFunc, 0, len(conf.Filter))
		for _, name := range conf.Filter {
			if f, ok := filters[name]; ok {
				middle = append(middle, f)
			}
		}

		r.Use(helmet.Default())
		r.Use(middle...)
		r.Any("/:service/*res",
			filter.Metadata(),
			filter.Logger(),
			filter.CheckSign("giny"),
			filter.Expr(),
			filter.Replay(a.redisCli),
			filter.Proxy(),
		)

		r.GET("/ts", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ts": time.Now().Unix()})
		})

		if adminDef {
			addr := netutil.GetInternalIp() + conf.Port
			admin.New(a.kvStore, addr).RegisterHandler(r)
		}

		a.web = http.Server{
			Addr:              conf.Port,
			Handler:           http.TimeoutHandler(r, time.Second*10, "timeout"),
			ReadTimeout:       time.Second * 5,
			ReadHeaderTimeout: time.Second * 2,
			WriteTimeout:      time.Second * 5,
			IdleTimeout:       time.Second,
			MaxHeaderBytes:    http.DefaultMaxHeaderBytes / 256, // 4KB
		}

		return nil
	}
}
