package filter

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cast"
)

const (
	appIDKey  = "X-Giny-App-Id"
	userIDKey = "X-Giny-User-Id"
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
		c.Set("metadata", meta)
		c.Set("since", time.Now())
	}
}
