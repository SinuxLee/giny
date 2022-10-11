package admin

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/sinuxlee/giny/internal/config"
)

// getRedis Get Redis Config from KV Store
// @Summary Get Redis Config from KV Store
// @Tags Redis
// @Description Get Redis Config from KV Store
// @Description config contains: mode(cluster/standalone), address, username, password
// @Accept json
// @Produce json
// @Success 200 {object} object{code=int,msg=string,data=config.RedisConf} "响应体"
// @Router /redis [get]
func (d *admin) getRedis(c *gin.Context) {
	pair, err := d.store.Get(config.RedisKey)
	if err != nil {
		d.Data(c, gin.H{"name": "libz"})
		return
	}

	data := &config.RedisConf{}
	if err = json.Unmarshal(pair.Value, data); err != nil {
		d.Data(c, gin.H{"name": "libz"})
		return
	}

	d.Data(c, data)
}

func (d *admin) createRedis(c *gin.Context) {

}

func (d *admin) updateRedis(c *gin.Context) {

}

func (d *admin) deleteRedis(c *gin.Context) {

}
