package filter

import (
	"net/http"

	"github.com/antonmedv/expr"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cast"
)

// Expr ...
func Expr() gin.HandlerFunc {
	pro, err := expr.Compile("UserId < 100 && AppId < 2000", expr.AsBool())
	if err != nil {
		return nil
	}

	return func(c *gin.Context) {
		data, exist := c.Get(bizMetaKey)
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
