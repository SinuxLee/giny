package filter

import (
	"bytes"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/valyala/bytebufferpool"
)

type logWriter struct {
	gin.ResponseWriter
	body *bytebufferpool.ByteBuffer
}

func (w logWriter) Write(b []byte) (int, error) {
	_, _ = w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w logWriter) WriteString(s string) (int, error) {
	_, _ = w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// Logger ...
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		rw := &logWriter{body: bytebufferpool.Get(), ResponseWriter: c.Writer}
		defer bytebufferpool.Put(rw.body)

		begin := time.Now()
		c.Writer = rw
		body := make([]byte, 0, 8)
		if c.Request.Body != nil {
			body, _ = c.GetRawData()
			_ = c.Request.Body.Close()
			c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
		}

		if len(body) == 0 {
			body = append(body, '{', '}')
		}

		c.Next()

		log.Info().Str("remote", c.Request.RemoteAddr).
			Str("method", c.Request.Method).
			Str("uri", c.Request.URL.String()).
			Interface("header", c.Request.Header).
			RawJSON("body", body).
			Interface("rspHeader", rw.Header()).
			Int("statusCode", rw.Status()).
			Int("contentLen", rw.Size()).
			RawJSON("response", rw.body.Bytes()).
			TimeDiff("cost", time.Now(), begin).
			Msg("logger")
	}
}
