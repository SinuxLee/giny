package filter

import (
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/valyala/bytebufferpool"
)

const (
	signKey  = "X-Giny-Sign"
	tsKey    = "X-Giny-Ts"
	nonceKey = "X-Giny-Nonce"
	tokenKey = "X-Giny-Token"
	sumKey   = "X-Giny-Sum"
)

var signKeys = []string{
	tsKey,
	nonceKey,
	tokenKey,
	sumKey,
}

func calcSign(req *http.Request, secret string) string {
	buf := bytebufferpool.Get()
	_, _ = buf.WriteString(req.Method)
	_, _ = buf.WriteString(req.URL.RequestURI())
	for _, k := range signKeys {
		_, _ = buf.WriteString(k)
		_, _ = buf.WriteString(req.Header.Get(k))
	}
	_, _ = buf.WriteString(secret)

	hash := md5.New()
	hash.Write(buf.Bytes())
	bytebufferpool.Put(buf)

	return strings.ToUpper(hex.EncodeToString(hash.Sum(nil)))
}

// CheckSign ...
func CheckSign(appSecret string) gin.HandlerFunc {
	sort.Strings(signKeys)

	return func(c *gin.Context) {
		sign := strings.ToUpper(c.GetHeader(signKey))
		if sign == "" {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 8000, "msg": "empty sign"})
			return
		}

		digest := calcSign(c.Request, appSecret)
		if sign != digest {
			log.Warn().Str("sign", sign).Str("digest", digest).Msg("invalid sign")
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 8000, "msg": "invalid sign"})
			return
		}
	}
}
