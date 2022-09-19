package admin

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/docker/libkv"
	"github.com/docker/libkv/store"
	"github.com/gin-gonic/gin"
)

//go:embed static/*
var static embed.FS
var _ Admin = (*admin)(nil)

// Response ...
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"msg"`
	Data    interface{} `json:"data,omitempty"`
}

// Admin ...
type Admin interface {
	RegisterHandler(e *gin.Engine)
}

// NewAdmin ...
func NewAdmin() Admin {
	libkv.AddStore("redis", func(addrs []string, options *store.Config) (store.Store, error) {
		return nil, nil
	})

	s, err := libkv.NewStore(store.CONSUL, []string{""}, &store.Config{})
	if err != nil {
		return nil
	}

	return &admin{
		kv: s,
	}
}

type admin struct {
	kv store.Store
}

// Data ...
func (d *admin) Data(ctx *gin.Context, data interface{}) {
	d.Response(ctx, &Response{
		Code:    0,
		Message: "OK",
		Data:    data,
	})
}

func (d *admin) Response(ctx *gin.Context, resp *Response) {
	ctx.JSON(http.StatusOK, resp)
}

func (d *admin) RegisterHandler(r *gin.Engine) {
	if subFS, err := fs.Sub(static, "static"); err == nil {
		r.StaticFS("/ui", http.FS(subFS))
	}

	api := r.Group("/api")
	api.GET("/list", func(c *gin.Context) {
		c.String(http.StatusOK, "api list")
	})
}
