package filter

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog/log"
)

// Proxy ...
func Proxy() gin.HandlerFunc {
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

		req := cli.R().SetBody(c.Request.Body)
		rsp, err := req.Execute(c.Request.Method, c.Request.RequestURI)
		if err != nil {
			c.String(http.StatusBadGateway, err.Error())
			return
		}

		c.Data(rsp.StatusCode(), rsp.Header().Get("Content-Type"), rsp.Body())
	}
}
