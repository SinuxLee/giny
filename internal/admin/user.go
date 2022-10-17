package admin

import (
	"encoding/json"
	"path"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	pkgerr "github.com/sinuxlee/giny/pkg/errors"
	"golang.org/x/crypto/bcrypt"
)

type login struct {
	Username string `form:"username" json:"username" binding:"required"`
	Password string `form:"password" json:"password" binding:"required"`
}

type User struct {
	UserName string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (d *admin) makeUserKey(key string) string {
	return path.Join("giny", "user", key)
}

func (d *admin) getUser(c *gin.Context) {

}

func (d *admin) createUser(c *gin.Context) {
	req := &User{}
	if err := c.ShouldBind(req); err != nil {
		d.Error(c, pkgerr.Warp(-1, err))
		return
	}

	key := d.makeUserKey(req.UserName)
	if exist, err := d.store.Exists(key); err != nil || !exist {
		log.Err(err).Str("username", req.UserName).Msg("user doesn't exist")
		d.Error(c, pkgerr.Warp(-1, err))
		return
	}

	pass, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		d.Error(c, pkgerr.Warp(-1, err))
		return
	}

	req.Password = string(pass)

	data, err := json.Marshal(req)
	if err != nil {
		return
	}

	if err = d.store.Put(key, data, nil); err != nil {
		return
	}

	d.Data(c, nil)
}

func (d *admin) updateUser(c *gin.Context) {

}

func (d *admin) deleteUser(c *gin.Context) {

}
