package nid

import (
	"testing"

	"github.com/duke-git/lancet/netutil"
	"github.com/stretchr/testify/assert"
)

func TestRedisNamed(t *testing.T) {
	nid, err := NewRedisNamed("127.0.0.1:6379")
	assert.Nil(t, err)

	id, err := nid.GetNodeID(&NameHolder{
		LocalPath:  "/tmp/giny",
		LocalIP:    netutil.GetInternalIp(),
		ServiceKey: "giny",
	})

	assert.Nil(t, err)
	assert.NotZero(t, id)
}

func TestConsulNamed(t *testing.T) {
	nid, err := NewConsulNamed("127.0.0.1:8500")
	assert.Nil(t, err)

	id, err := nid.GetNodeID(&NameHolder{
		LocalPath:  "/tmp/giny123",
		LocalIP:    netutil.GetInternalIp(),
		ServiceKey: "giny",
	})

	assert.Nil(t, err)
	assert.NotZero(t, id)
}
