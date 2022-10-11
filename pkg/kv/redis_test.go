package kv

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/docker/libkv"
	"github.com/docker/libkv/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

const (
	defaultKey  = "key"
	defaultVale = "giny"
)

type RedisKVSuite struct {
	suite.Suite
	rs     *miniredis.Miniredis
	store  store.Store
	stopCh chan struct{}
}

func (s *RedisKVSuite) SetupSuite() {
	Register()

	s.stopCh = make(chan struct{}, 2)

	s.rs = miniredis.RunT(s.T())
	assert.NotNil(s.T(), s.rs)

	var err error
	s.store, err = libkv.NewStore(StoreRedis, []string{"127.0.0.1:6379"}, nil)
	assert.Nil(s.T(), err)

	tree, err := s.store.WatchTree("/", make(chan struct{}, 1))
	assert.Nil(s.T(), err)
	go func() {
		for {
			select {
			case <-s.stopCh:
				return
			case pairs, ok := <-tree:
				if !ok {
					return
				}

				for _, pair := range pairs {
					s.T().Logf("watch tree, key:%v,value:%v,index:%v", pair.Key, pair.Value, pair.LastIndex)
				}
			}
		}
	}()

	watch, err := s.store.Watch(defaultKey, make(chan struct{}, 1))
	assert.Nil(s.T(), err)
	go func() {
		for {
			select {
			case <-s.stopCh:
				return
			case pair, ok := <-watch:
				if !ok {
					return
				}

				s.T().Logf("watch key, key:%v,value:%v,index:%v", pair.Key, pair.Value, pair.LastIndex)
			}
		}
	}()
}

func (s *RedisKVSuite) TearDownSuite() {
	close(s.stopCh)
	s.T().Log(s.rs.Keys())
}

func (s *RedisKVSuite) TestStore() {
	assert.Nil(s.T(), s.store.Put(defaultKey, []byte(defaultVale), nil))

	pair, err := s.store.Get(defaultKey)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), string(pair.Value), defaultVale)

	ok, err := s.store.Exists(defaultKey)
	assert.Nil(s.T(), err)
	assert.True(s.T(), ok)

	pairs, err := s.store.List(defaultKey)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), len(pairs), 1)
	assert.Equal(s.T(), string(pairs[0].Value), defaultVale)

	assert.Nil(s.T(), s.store.DeleteTree(defaultKey))

	assert.Nil(s.T(), s.store.Delete(defaultKey))

	ok, err = s.store.Exists(defaultKey)
	assert.Nil(s.T(), err)
	assert.False(s.T(), ok)
}

func (s *RedisKVSuite) TestAtomic() {
	assert.Nil(s.T(), s.store.Put(defaultKey, []byte(defaultVale), nil))

	pair, err := s.store.Get(defaultKey)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), string(pair.Value), defaultVale)

	_, newPair, err := s.store.AtomicPut(defaultKey, []byte(defaultKey), pair, nil)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), string(newPair.Value), defaultKey)

	ret, err := s.store.AtomicDelete(defaultKey, newPair)
	assert.Nil(s.T(), err)
	assert.True(s.T(), ret)
}

func TestRedis(t *testing.T) {
	suite.Run(t, new(RedisKVSuite))

	// client.ConfigSet(ctx, "notify-keyspace-events", "KEA")
}
