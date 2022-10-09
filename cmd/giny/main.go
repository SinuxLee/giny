package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	helmet "github.com/danielkov/gin-helmet"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	limits "github.com/gin-contrib/size"
	"github.com/gin-gonic/gin"
	"github.com/sinuxlee/giny/pkg/filter"
	"golang.org/x/sync/errgroup"
)

var version = "No Build Info"

func main() {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(helmet.Default())
	r.Use(cors.Default())
	r.Use(gzip.Gzip(gzip.DefaultCompression))
	r.Use(limits.RequestSizeLimiter(http.DefaultMaxHeaderBytes / 4)) // 256KB

	r.Any("/:service/*res",
		filter.Metadata(),
		filter.Logger(),
		filter.CheckSign("giny"),
		filter.Expr(),
		filter.Proxy(),
	)

	r.GET("/ts", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ts": time.Now().Unix()})
	})

	l, err := net.Listen("tcp", ":8080")
	if err != nil {
		panic(err)
	}

	g, c := errgroup.WithContext(context.Background())
	srv := http.Server{
		Handler:           http.TimeoutHandler(r, time.Second*10, "timeout"),
		ReadTimeout:       time.Second * 5,
		ReadHeaderTimeout: time.Second * 2,
		WriteTimeout:      time.Second * 5,
		IdleTimeout:       time.Second,
		MaxHeaderBytes:    http.DefaultMaxHeaderBytes / 256, // 4KB
		BaseContext: func(_ net.Listener) context.Context {
			return c
		},
	}

	g.Go(func() error {
		return srv.Serve(l)
	})

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT)
	<-ch

	signal.Stop(ch)
	close(ch)

	ctx, cancel := context.WithTimeout(c, 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil && !errors.Is(err, context.Canceled) {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
	}

	if err := g.Wait(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
	}
}
