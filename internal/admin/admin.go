package admin

import (
	"embed"
	"io/fs"
	"net/http"
	"net/url"
	"strings"

	"github.com/docker/libkv/store"
	"github.com/gin-gonic/gin"
	"github.com/sinuxlee/giny/internal/admin/docs"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
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

// New ...
func New(s store.Store, swaggerAddr string) Admin {
	return &admin{
		store:       s,
		swaggerHost: swaggerAddr,
	}
}

// swagger doc
// @title Giny API Admin
// @version 1.0
// @description API Admin for Giny
// @termsOfService https://github.com/sinuxlee/giny

// @tag.name admin
// @tag.description Some APIs to operate Key and Value in Store

// @contact.name sinux
// @contact.url https://github.com/sinuxlee/giny
// @contact.email pingfan14@gmail.com

// @license.name MIT
// @license.url https://choosealicense.com/licenses/mit/

// @schemes http
// @host localhost:8086
// @BasePath /api
// @query.collection.format multi

// @securityDefinitions.basic BasicAuth

// @securityDefinitions.apikey TokenAuth
// @in header
// @name Authorization

// @x-extension-openapi {"example": "value on a json format"}

type admin struct {
	store       store.Store
	swaggerHost string
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

func (d *admin) swaggerDocs(r *gin.Engine) {
	docs.SwaggerInfo.Host = d.swaggerHost
	u := url.URL{
		Scheme: "http",
		Host:   d.swaggerHost,
		Path:   "/swagger/doc.json",
	}

	r.GET("/swagger/*any", func(c *gin.Context) {
		if strings.TrimPrefix(c.Param("any"), "/") == "" {
			c.Redirect(http.StatusTemporaryRedirect, "/swagger/index.html")
			c.Abort()
		}
	}, ginSwagger.WrapHandler(
		swaggerFiles.Handler,
		ginSwagger.URL(u.String())),
	)
}

func (d *admin) RegisterHandler(r *gin.Engine) {
	d.swaggerDocs(r)

	if subFS, err := fs.Sub(static, "static"); err == nil {
		r.StaticFS("/ui", http.FS(subFS))
	}

	api := r.Group("/api")

	api.GET("/redis", d.getRedis)
	api.POST("/redis", d.createRedis)
	api.PUT("/redis", d.updateRedis)
	api.DELETE("/redis", d.deleteRedis)

	api.GET("/gin", d.getGin)
	api.POST("/gin", d.createGin)
	api.PUT("/gin", d.updateGin)
	api.DELETE("/gin", d.deleteGin)
}
