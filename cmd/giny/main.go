package main

import (
	"crypto/md5"
	"encoding/hex"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/antonmedv/expr"
	helmet "github.com/danielkov/gin-helmet"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cast"
	"github.com/valyala/bytebufferpool"
)

// go:embed  static/*
// var static embed.FS

type userMeta struct {
	UserId int
	AppId  int
}

type api struct {
	Level int // private/third-party/public
}

type node struct {
	Addr string
}

type cluster struct {
	Nodes []node
}

type service struct {
	Cluster cluster
	Apis    []api
}

func logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}

func checkSign() gin.HandlerFunc {
	signKey := "X-Giny-Sign"

	keys := []string{
		"X-Giny-Ts",
		"X-Giny-Nonce",
		"X-Giny-Token",
		"X-Giny-Sum",
	}
	sort.Strings(keys)
	appSecret := "gateway#secret"

	return func(c *gin.Context) {
		sign := strings.ToUpper(c.GetHeader(signKey))
		if sign == "" {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 8000, "msg": "empty sign"})
			return
		}

		buf := bytebufferpool.Get()
		_, _ = buf.WriteString(c.Request.Method)
		_, _ = buf.WriteString(c.Request.URL.String())
		for _, k := range keys {
			_, _ = buf.WriteString(k)
			_, _ = buf.WriteString(c.GetHeader(k))
		}
		_, _ = buf.WriteString(appSecret)

		hash := md5.New()
		hash.Write(buf.Bytes())
		bytebufferpool.Put(buf)
		if strings.ToUpper(hex.EncodeToString(hash.Sum(nil))) != sign {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 8000, "msg": "invalid sign"})
			return
		}
	}
}

func metadata() gin.HandlerFunc {
	return func(c *gin.Context) {
		meta := &userMeta{
			AppId:  cast.ToInt(c.GetHeader("X-Giny-AppId")),
			UserId: cast.ToInt(c.GetHeader("X-Giny-UserId")),
		}
		c.Set("metadata", meta)
		c.Set("since", time.Now())
	}
}

func middleware() gin.HandlerFunc {
	pro, err := expr.Compile("UserId < 100 && AppId < 2000", expr.AsBool())
	if err != nil {
		return nil
	}

	return func(c *gin.Context) {
		data, exist := c.Get("metadata")
		if !exist {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 8000, "msg": "no metadata"})
			return
		}

		output, err := expr.Run(pro, data)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 8001, "msg": err.Error()})
			return
		}

		if !cast.ToBool(output) {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 8002, "msg": "vm return false"})
			return
		}
	}
}

func proxy() gin.HandlerFunc {
	cli := resty.New().SetTimeout(time.Second * 5).
		OnBeforeRequest(func(c *resty.Client, r *resty.Request) error {
			c.SetBaseURL("https://httpbin.org")
			return nil
		}).
		OnAfterResponse(func(c *resty.Client, r *resty.Response) error {
			log.Info().Str("req", r.Status()).Msg("resp")
			return nil
		})

	return func(c *gin.Context) {
		log.Info().Str("service", c.Param("service")).
			Str("res", c.Param("res")).
			Msg("request log")

		req := cli.R().SetBody(http.MaxBytesReader(c.Writer, c.Request.Body, http.DefaultMaxHeaderBytes/4)) // 256KB
		rsp, err := req.Execute(c.Request.Method, c.Param("res"))
		if err != nil {
			c.String(http.StatusBadGateway, err.Error())
			return
		}

		c.Data(rsp.StatusCode(), rsp.Header().Get("Content-Type"), rsp.Body())
	}
}

func main() {
	chain := gin.HandlersChain{metadata(), logger(), checkSign(), middleware(), proxy()}
	r := gin.Default()
	r.Use(helmet.Default())
	r.Use(cors.Default())
	r.Use(gzip.Gzip(gzip.DefaultCompression))

	r.GET("/health", func(ctx *gin.Context) {
		ctx.String(http.StatusOK, "It works")
	})

	r.Any("/:service/*res", chain...)

	if gin.IsDebugging() {
		pprof.Register(r)

		// if subFS, err := fs.Sub(static, "static"); err == nil {
		// 	r.StaticFS("/web", http.FS(subFS))
		// }

		// admin api
		api := r.Group("/api")
		api.GET("/list", func(ctx *gin.Context) {
			ctx.String(http.StatusOK, "api list")
		})

		// test api
		test := r.Group("/test")
		test.GET("/nothing", func(ctx *gin.Context) {
			ctx.String(http.StatusOK, "nothing")
		})
	}

	l, err := net.Listen("tcp", ":8080")
	if err != nil {
		return
	}

	server := &http.Server{
		Handler:           http.TimeoutHandler(r, time.Second*10, "timeout"),
		ReadTimeout:       time.Second * 5,
		ReadHeaderTimeout: time.Second * 2,
		WriteTimeout:      time.Second * 5,
		IdleTimeout:       time.Second,
		MaxHeaderBytes:    http.DefaultMaxHeaderBytes / 256, // 4KB
	}

	if err := server.Serve(l); err != nil {
		return
	}
}
