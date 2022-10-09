package filter

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cast"
)

const (
	appIDKey  = "X-Giny-Aid"
	userIDKey = "X-Giny-Uid"

	sinceMetaKey = "since"
	bizMetaKey   = "biz"
)

type metadata struct {
	UserID int
	AppID  int
}

// Metadata ...
func Metadata() gin.HandlerFunc {
	return func(c *gin.Context) {
		meta := &metadata{
			UserID: cast.ToInt(c.GetHeader(appIDKey)),
			AppID:  cast.ToInt(c.GetHeader(userIDKey)),
		}
		c.Set(bizMetaKey, meta)
		c.Set(sinceMetaKey, time.Now())
	}
}
