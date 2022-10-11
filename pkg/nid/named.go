package nid

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/docker/libkv"
	"github.com/docker/libkv/store"
	"github.com/docker/libkv/store/consul"
	"github.com/pkg/errors"
	"github.com/sinuxlee/giny/pkg/kv"
)

const (
	timeFormat = "2006-01-02 15:04:05.000"
	nodePrefix = "node_"
	retryCount = 5
	bucketName = "nodeId"
)

type NodeNamed interface {
	GetNodeID(*NameHolder) (int, error)
}

// NameHolder ...
type NameHolder struct {
	LocalPath  string `json:"localPath"`
	LocalIP    string `json:"localIp"`
	ApplyTime  string `json:"applyTime"`
	ServiceKey string `json:"-"`
}

func (h *NameHolder) decode(data []byte) error {
	err := json.Unmarshal(data, h)
	return err
}

func (h *NameHolder) encode() ([]byte, error) {
	return json.MarshalIndent(h, "", "  ")
}

func NewConsulNamed(addr string) (NodeNamed, error) {
	consul.Register()
	kvStore, err := libkv.NewStore(
		store.CONSUL,
		[]string{addr},
		&store.Config{
			ConnectionTimeout: 10 * time.Second,
		},
	)

	if err != nil {
		return nil, err
	}

	return &nodeNamed{
		Store:      kvStore,
		retryCount: retryCount,
	}, nil
}

func NewEtcdNamed(addr string) (NodeNamed, error) {
	kvStore, err := libkv.NewStore(
		store.ETCD,
		[]string{addr},
		&store.Config{
			ConnectionTimeout: 10 * time.Second,
		},
	)

	if err != nil {
		return nil, err
	}

	return &nodeNamed{
		Store:      kvStore,
		retryCount: retryCount,
	}, nil
}

func NewBoltNamed(addr string) (NodeNamed, error) {
	kvStore, err := libkv.NewStore(
		store.BOLTDB,
		[]string{addr},
		&store.Config{
			ConnectionTimeout: 10 * time.Second,
			Bucket:            bucketName,
		},
	)

	if err != nil {
		return nil, err
	}

	return &nodeNamed{
		Store:      kvStore,
		retryCount: retryCount,
	}, nil
}

func NewRedisNamed(addr string) (NodeNamed, error) {
	kv.Register()
	kvStore, err := libkv.NewStore(
		kv.StoreRedis,
		[]string{addr},
		&store.Config{
			ConnectionTimeout: 10 * time.Second,
		},
	)

	if err != nil {
		return nil, err
	}

	return &nodeNamed{
		Store:      kvStore,
		retryCount: retryCount,
	}, nil
}

type nodeNamed struct {
	store.Store
	retryCount int
}

func (c *nodeNamed) GetNodeID(holder *NameHolder) (nodeID int, err error) {
	holder.LocalPath, _ = filepath.Abs(holder.LocalPath)
	if nodeID, err = c.recoverNodeID(holder); err != nil {
		return
	}

	if nodeID == 0 {
		nodeID, err = c.applyNodeID(holder)
	}

	return
}

// RecoverNodeID 恢复配置
func (c *nodeNamed) recoverNodeID(holder *NameHolder) (int, error) {
	kvPairs, err := c.List(holder.ServiceKey)
	if err != nil && !errors.Is(err, store.ErrKeyNotFound) {
		return 0, err
	}

	info := &NameHolder{}
	for _, pair := range kvPairs {
		if info.decode(pair.Value) != nil ||
			info.LocalIP != holder.LocalIP ||
			info.LocalPath != holder.LocalPath {
			continue
		}

		if err := c.tryHold(pair, holder); err != nil {
			return 0, err
		}

		return c.convertStringToID(pair.Key), nil
	}

	return 0, nil
}

func (c *nodeNamed) applyNodeID(holder *NameHolder) (int, error) {
	for i := 0; i < c.retryCount; i++ {
		pairs, err := c.List(holder.ServiceKey)
		if err != nil && !errors.Is(err, store.ErrKeyNotFound) {
			return 0, err
		}

		newID := c.makeNewID(pairs)
		err = c.tryHold(&store.KVPair{Key: c.makeStoreKey(holder.ServiceKey, newID)}, holder)
		if err == nil {
			return newID, nil
		}

		time.Sleep(time.Millisecond * 100)
	}

	return 0, errors.Errorf("try to hold %d times, but failed", c.retryCount)
}

func (c *nodeNamed) makeNewID(pairs []*store.KVPair) int {
	usedIDs := make([]int, 256)
	for _, pair := range pairs {
		nid := c.convertStringToID(pair.Key)
		if nid >= len(usedIDs) {
			tmp := usedIDs
			usedIDs := make([]int, nid*2)
			copy(usedIDs, tmp)
		}
		usedIDs[nid] = nid
	}

	newID := 1
	for ; newID < len(usedIDs); newID++ {
		if usedIDs[newID] == 0 {
			break
		}
	}

	return newID
}

func (c *nodeNamed) convertStringToID(s string) int {
	paths := strings.Split(s, "/")
	length := len(paths)
	if length == 0 {
		return 0
	}

	if id, err := strconv.Atoi(strings.TrimPrefix(paths[length-1], nodePrefix)); err == nil && id > 0 {
		return id
	}

	return 0
}

func (c *nodeNamed) makeStoreKey(prefix string, id int) string {
	return fmt.Sprintf("%v/nodeId/%v%02v", prefix, nodePrefix, id)
}

func (c *nodeNamed) tryHold(pair *store.KVPair, holder *NameHolder) error {
	newPair, err := c.Get(pair.Key)
	if err != nil && !errors.Is(err, store.ErrKeyNotFound) {
		return err
	}

	holder.ApplyTime = time.Now().Format(timeFormat)
	if pair.Value, err = holder.encode(); err != nil {
		return err
	}

	if newPair == nil {
		return c.Put(pair.Key, pair.Value, nil)
	}

	if pair.LastIndex != 0 && newPair.LastIndex != pair.LastIndex {
		return errors.New("try hold failed")
	}

	_, _, err = c.AtomicPut(pair.Key, pair.Value, pair, nil)
	return err
}
