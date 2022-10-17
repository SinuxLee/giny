package admin

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/url"
	"reflect"
	"runtime"
	"strings"
	"time"

	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/docker/libkv/store"
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/rs/xid"
	"github.com/rs/zerolog/log"
	"github.com/sinuxlee/giny/internal/admin/docs"
	"github.com/sinuxlee/giny/internal/config"
	pkgerr "github.com/sinuxlee/giny/pkg/errors"
	"github.com/sinuxlee/giny/pkg/storeadapter"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"golang.org/x/crypto/bcrypt"
)

const (
	zone         = "giny admin"
	identityKey  = "ginyUserKey"
	casbinPrefix = "giny"
	superAdmin   = "root"
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
	RegisterHandler(e *gin.Engine) error
}

// New ...
func New(s store.Store, swagger string) (Admin, error) {
	policyData := make(storeadapter.Policy, 0)
	modelData := make([]storeadapter.Model, 0)
	conf := config.New(s, casbinPrefix)
	if err := conf.GetStoreConf(storeadapter.PolicyKey, &policyData, storeadapter.Policy{
		"p, root, /admin/api/*, GET|POST|PUT|DELETE",
	}); err != nil {
		return nil, errors.Wrap(err, "get casbin policy")
	}

	if err := conf.GetStoreConf(storeadapter.ModelKey, &modelData, []storeadapter.Model{
		{"r", "r", "sub, obj, act"},                //nolint:govet
		{"p", "p", "sub, obj, act"},                //nolint:govet
		{"e", "e", "some(where (p.eft == allow))"}, //nolint:govet
		{"m", "m", "r.sub == p.sub && keyMatch(r.obj, p.obj) && regexMatch(r.act, p.act)"}, //nolint:govet
	}); err != nil {
		return nil, errors.Wrap(err, "get casbin model")
	}

	m := model.NewModel()
	for _, d := range modelData {
		m.AddDef(d.Sec, d.Key, d.Value)
	}

	e, err := casbin.NewEnforcer(m, storeadapter.NewAdapter(s, casbinPrefix))
	if err != nil {
		return nil, errors.Wrap(err, "casbin new enforcer")
	}

	return &admin{
		enforcer:    e,
		conf:        conf,
		store:       s,
		swaggerHost: swagger,
	}, nil
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
// @BasePath /admin
// @query.collection.format multi

// @securityDefinitions.basic BasicAuth

// @securityDefinitions.apikey JWT
// @in header
// @name Authorization

// @x-extension-openapi {"example": "value on a json format"}

type admin struct {
	enforcer    *casbin.Enforcer
	conf        config.Configure
	store       store.Store
	swaggerHost string
}

// Data ...
func (d *admin) Data(c *gin.Context, data interface{}) {
	d.Response(c, &Response{
		Code:    0,
		Message: "OK",
		Data:    data,
	})
}

// Error ...
func (d *admin) Error(c *gin.Context, e error) {
	d.Response(c, &Response{
		Code:    pkgerr.Code(e),
		Message: pkgerr.Message(e),
	})
	c.Abort()
}

func (d *admin) Response(c *gin.Context, resp *Response) {
	c.JSON(http.StatusOK, resp)
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

// jwtPayload put user info into JWT payload
// @Summary Login Giny API Admin
// @Tags User
// @Description Login Admin, the API will return Token
// @Accept json
// @Produce json
// @Param body body login true "name and pass" default({"username":"root","password":"admin@giny"})
// @Success 200 {object} object{code=int,msg=string,data=object{token=string,expireAt=int}} "HTTP Response"
// @Router /auth/login [post]
func (d *admin) jwtPayload(data interface{}) jwt.MapClaims {
	if v, ok := data.(*User); ok {
		return jwt.MapClaims{
			identityKey: v.UserName,
		}
	}
	return jwt.MapClaims{}
}

// jwtIdentityHandler fetch user info from JWT
// @Summary Refresh Token
// @Tags User
// @Description return new token
// @Accept json
// @Produce json
// @Success 200 {object} object{code=int,msg=string,data=object{token=string,expireAt=int}} "HTTP Response"
// @Router /auth/refresh-token [get]
func (d *admin) jwtIdentityHandler(c *gin.Context) interface{} {
	claims := jwt.ExtractClaims(c)
	username, ok := claims[identityKey]
	if !ok {
		return nil
	}
	return &User{
		UserName: username.(string),
	}
}

// jwtAuthenticator Checks user's password
func (d *admin) jwtAuthenticator(c *gin.Context) (interface{}, error) {
	req := &login{}
	if err := c.ShouldBind(req); err != nil {
		return "", jwt.ErrMissingLoginValues
	}

	storePass := ""
	if req.Username == superAdmin {
		conf := &config.AdminConf{}
		if err := d.conf.GetStoreConf(config.AdminKey, conf, nil); err == nil {
			storePass = conf.RootPass
		}
	} else {
		if pair, err := d.store.Get(d.makeUserKey(req.Username)); err == nil {
			u := &User{}
			if err = json.Unmarshal(pair.Value, u); err == nil {
				storePass = u.Password
			}
		}
	}

	if err := bcrypt.CompareHashAndPassword([]byte(storePass), []byte(req.Password)); err != nil {
		return nil, jwt.ErrFailedAuthentication
	}

	return &User{
		UserName: req.Username,
	}, nil
}

// jwtAuthorizator Checks if the user can access this url
func (d *admin) jwtAuthorizator(data interface{}, c *gin.Context) bool {
	u, ok := data.(*User)
	if !ok {
		return false
	}

	pass, err := d.enforcer.Enforce(u.UserName, c.Request.URL.Path, c.Request.Method)
	if err != nil {
		return false
	}

	return pass
}

func (d *admin) jwtUnauthorized(c *gin.Context, code int, message string) {
	c.JSON(code, gin.H{
		"code": code,
		"msg":  message,
	})
}

// @Summary Logout Giny API Admin
// @Tags User
// @Description Logout Admin, let client reset cookie
// @Accept json
// @Produce json
// @Success 200 {object} object{code=int,msg=string} "HTTP Response"
// @Router /auth/logout [post]
func (d *admin) jwtLoginResponse(c *gin.Context, _ int, token string, expire time.Time) {
	d.Data(c, gin.H{
		"token":    token,
		"expireAt": expire.Unix(),
	})
}

func (d *admin) lockUser(c *gin.Context) {
	v, exist := c.Get(identityKey)
	user, ok := v.(*User)
	if !exist || !ok {
		return
	}

	locker, err := d.store.NewLock(user.UserName, &store.LockOptions{
		Value: xid.New().Bytes(),
		TTL:   time.Second * 10,
	})
	if err != nil {
		log.Err(err).Str("user", user.UserName).Msg("new lock failed")
		d.Error(c, pkgerr.Warp(-1, err))
		return
	}

	if _, err = locker.Lock(nil); err != nil {
		log.Err(err).Str("user", user.UserName).Msg("lock failed")
		d.Error(c, pkgerr.Warp(-1, err))
		return
	}

	c.Next()

	if err = locker.Unlock(); err != nil {
		log.Err(err).Str("user", user.UserName).Msg("unlock failed")
		return
	}
}

func (d *admin) RegisterHandler(r *gin.Engine) error {
	d.swaggerDocs(r)

	if subFS, err := fs.Sub(static, "static"); err == nil {
		r.StaticFS("/ui", http.FS(subFS))
	}

	conf := &config.AdminConf{}
	if err := d.conf.GetStoreConf(config.AdminKey, conf, &config.AdminConf{
		RootPass:      "$2a$10$GVsWE1SSkf.7qi284yElOOlCj8tMJDirt8qAvTwzXEyX1Ks41CC6C",
		Secret:        xid.New().String(),
		TokenExpire:   4,
		RefreshExpire: 24,
	}); err != nil {
		return err
	}

	authMiddleware, err := jwt.New(&jwt.GinJWTMiddleware{
		SendCookie:      true,
		CookieHTTPOnly:  true,
		Realm:           zone,
		Key:             []byte(conf.Secret),
		Timeout:         time.Hour * time.Duration(conf.TokenExpire),
		MaxRefresh:      time.Hour * time.Duration(conf.RefreshExpire),
		IdentityKey:     identityKey,
		PayloadFunc:     d.jwtPayload,
		IdentityHandler: d.jwtIdentityHandler,
		Authenticator:   d.jwtAuthenticator,
		Authorizator:    d.jwtAuthorizator,
		Unauthorized:    d.jwtUnauthorized,
		LoginResponse:   d.jwtLoginResponse,
		RefreshResponse: d.jwtLoginResponse,
		TokenLookup:     "header: Authorization, query: token, cookie: jwt",
		TimeFunc:        time.Now,
	})

	if err != nil {
		return err
	}

	if err = authMiddleware.MiddlewareInit(); err != nil {
		return err
	}

	ag := r.Group("/admin")

	auth := ag.Group("/auth")
	auth.POST("/login", authMiddleware.LoginHandler)
	auth.POST("/logout", authMiddleware.LogoutHandler)
	auth.GET("/refresh-token", authMiddleware.RefreshHandler)

	api := ag.Group("/api")
	api.Use(authMiddleware.MiddlewareFunc())
	api.Use(d.lockUser)

	api.GET("/user", d.getRedis)
	api.POST("/user", d.createRedis)
	api.PUT("/user", d.updateRedis)
	api.DELETE("/user", d.deleteRedis)

	api.GET("/redis", d.getRedis)
	api.POST("/redis", d.createRedis)
	api.PUT("/redis", d.updateRedis)
	api.DELETE("/redis", d.deleteRedis)

	api.GET("/gin", d.getGin)
	api.POST("/gin", d.createGin)
	api.PUT("/gin", d.updateGin)
	api.DELETE("/gin", d.deleteGin)

	// TODO: gen swagger comment
	for _, h := range r.Routes() {
		if strings.HasPrefix(h.Path, "/admin/api") {
			names := strings.Split(runtime.FuncForPC(reflect.ValueOf(h.HandlerFunc).Pointer()).Name(), ".")
			name := strings.TrimSuffix(names[len(names)-1], "-fm")
			log.Info().Str("method", h.Method).Interface("name", name).Msg(h.Path)
		}
	}

	return nil
}
