package filter

import (
	"fmt"
	"net/http"
	"time"

	"github.com/coocood/freecache"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cast"
	"github.com/valyala/bytebufferpool"
)

// Replay ...
func Replay(r redis.UniversalClient) gin.HandlerFunc {
	duration := int64(10 * 60)
	cacheSize := int(duration * 5000)
	cache := freecache.NewCache(cacheSize)

	return func(c *gin.Context) {
		ts := cast.ToInt64(c.GetHeader(tsKey))
		if ts <= 0 {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 8000, "msg": "invalid ts"})
			return
		}

		if now := time.Now().Unix(); now-duration > ts || now+duration < ts {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 8000, "msg": "expired ts"})
			return
		}

		nonce := c.GetHeader(nonceKey)
		if nonce == "" {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 8000, "msg": "empty nonce"})
			return
		}

		uid := c.GetHeader(userIDKey)
		if cast.ToUint64(uid) <= 0 {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 8000, "msg": "invalid uid"})
			return
		}

		path := c.Request.RequestURI
		buf := bytebufferpool.Get()
		defer bytebufferpool.Put(buf)
		_, _ = fmt.Fprintf(buf, "giny:%v:%v:%v", path, uid, nonce)

		if r != nil {
			ret, err := r.SetNX(c.Request.Context(), buf.String(), "", time.Duration(duration)*time.Second).Result()
			if err != nil {
				log.Err(err).Msg("record replay key failed")
				return
			}

			// repeat request
			if !ret {
				c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 8000, "msg": "replay request"})
				return
			}
			return
		}

		if ret, err := cache.Get(buf.Bytes()); ret == nil && err == nil {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 8000, "msg": "replay request"})
			return
		}

		if err := cache.Set(buf.Bytes(), []byte{}, int(duration)); err != nil {
			log.Err(err).Msg("failed to cache key")
			return
		}
	}
}
