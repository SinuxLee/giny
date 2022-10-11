package kv

import (
	"context"
	"crypto/sha1" // nolint:gosec
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/docker/libkv"
	"github.com/docker/libkv/store"
	"github.com/go-redis/redis/v8"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

const (
	StoreRedis store.Backend = "redis"

	limitValueSize = 1 << 12 // 4K
	scriptDelLock  = "delLock"
	scriptCASDel   = "casDel"
	scriptCASPut   = "casPut"
	scriptIdxGet   = "idxGet"
	scriptPut      = "put"
	scriptDel      = "del"
)

var (
	errDiffKey     = errors.New("not same key")
	errTooLarge    = errors.New("too large value")
	errNoDirAtomic = errors.New("no support atomic dir ops")

	scripts = map[string]*redis.Script{
		scriptPut: redis.NewScript(`
		local key = KEYS[1]
		local data = ARGV[1]
		local expire = ARGV[2]
		local ret = nil

		if tonumber(expire) > 0 then
			ret = redis.call('set', key, data, 'ex', expire)
		else
			ret = redis.call('set', key, data)
		end
	
		local crc = string.sub(redis.call('dump', key),-8)
		redis.call('publish',key, cjson.encode({["data"]=data,["crc"]=crc}))

		return ret
	`),
		scriptDel: redis.NewScript(`
		local count = 0
		
		for _, key in pairs(KEYS) do
			if 1 == redis.call('exists',key) then
				redis.call('publish',key, "{}")
				count = count + redis.call("DEL", key)
			end
		end
		return count
	`),
		scriptIdxGet: redis.NewScript(`
		local key = KEYS[1]
		local value = redis.call('get', key)
		if value then
			local dump = redis.call('dump', key)
			return {value, string.sub(dump,-8)}
		else 
			return {}
		end
	`),
		scriptDelLock: redis.NewScript(`
		local key = KEYS[1]
		local token = ARGV[1]
		
		local value = redis.call("GET", key)
		if value == token then
			return redis.call("DEL", key)
		else
			return 0
		end
	`),
		scriptCASDel: redis.NewScript(`
		local key = KEYS[1]
		local crc = ARGV[1]
		
		local dump = redis.call('dump', key)
		if dump and string.sub(dump,-8) == crc then
			redis.call('publish',key, "{}")
			return redis.call("DEL", key)
		end

		return 0
	`),
		scriptCASPut: redis.NewScript(`
		local key = KEYS[1]
		local crc = ARGV[1]
		local data = ARGV[2]
		local expire = ARGV[3]

		local dump = redis.call('dump', key)
		if dump and string.sub(dump,-8) == crc then
			if tonumber(expire) > 0 then
				redis.call('set', key, data, 'ex', expire)
			else
				redis.call('set', key, data)
			end
		
			crc = string.sub(redis.call('dump', key),-8)
			redis.call('publish',key, cjson.encode({["data"]=data,["crc"]=crc}))
			return crc
		end

		return ""
	`),
	}
)

func Register() {
	libkv.AddStore(StoreRedis, New)
}

// New create a store which base on redis
func New(addr []string, options *store.Config) (store.Store, error) {
	var c redis.UniversalClient
	var user, pass, bucket string
	if options != nil {
		user, pass, bucket = options.Username, options.Password, options.Bucket
	}

	if length := len(addr); length <= 0 {
		return nil, errors.New("empty addr")
	} else if length > 1 {
		c = redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:    addr,
			Username: user,
			Password: pass,
		})
	} else {
		c = redis.NewClient(&redis.Options{
			Addr:     addr[0],
			Username: user,
			Password: pass,
		})
	}

	if err := c.Ping(context.TODO()).Err(); err != nil {
		return nil, err
	}

	for _, s := range scripts {
		ret, err := s.Exists(context.TODO(), c).Result()
		if err != nil {
			return nil, err
		}

		if !ret[0] {
			if str, err := s.Load(context.TODO(), c).Result(); err != nil {
				return nil, err
			} else if str != s.Hash() {
				return nil, errors.New("diff sha1hex str")
			}
		}
	}

	return &redisKV{
		cli:     c,
		bucket:  bucket,
		scripts: scripts,
	}, nil
}

// PubSubMsg ...
type PubSubMsg struct {
	Data string `json:"data"`
	CRC  string `json:"crc"`
}

type redisLock struct {
	*store.LockOptions
	cli    redis.UniversalClient
	key    string
	script *redis.Script
}

// Lock ...
func (l *redisLock) Lock(_ chan struct{}) (<-chan struct{}, error) {
	var v []byte
	var expire time.Duration
	if l.LockOptions != nil {
		v = l.LockOptions.Value
		expire = l.LockOptions.TTL
	}

	ok, err := l.cli.SetNX(context.TODO(), l.key, v, expire).Result()
	if err == nil && !ok {
		err = errors.New("lock error")
	}

	return make(chan struct{}), err
}

// Unlock ...
func (l *redisLock) Unlock() error {
	var v []byte
	if l.LockOptions != nil && len(l.LockOptions.Value) > 0 {
		v = l.LockOptions.Value
	}

	ret, err := l.script.Run(context.TODO(), l.cli, []string{l.key}, v).Bool()
	if err != nil {
		return err
	}

	if !ret {
		return errors.New("unlock error")
	}

	return nil
}

type redisKV struct {
	cli     redis.UniversalClient
	bucket  string
	scripts map[string]*redis.Script
}

func (r *redisKV) makeKey(key string) string {
	return path.Join(fmt.Sprintf("{kv}/%v", r.bucket), key)
}

func (r *redisKV) trimPrefix(key string) string {
	return strings.TrimPrefix(key, r.makeKey("")+"/")
}

func (r *redisKV) makeDir(dir string) string {
	return r.makeKey(dir) + "*"
}

func (r *redisKV) makeChannelPattern(dir string) string {
	return path.Join(fmt.Sprintf("{kv}/%v", r.bucket), dir) + "*"
}

func (r *redisKV) sha1Hex(v []byte) string {
	hash := sha1.New() // nolint:gosec
	_, _ = hash.Write(v)
	return hex.EncodeToString(hash.Sum(nil))
}

func (r *redisKV) parseMessage(msg *redis.Message) (*store.KVPair, error) {
	key := r.trimPrefix(msg.Channel)
	if key == "" {
		log.Warn().Str("fullKey", msg.Channel).Msg("receive empty key")
		return nil, errors.New("key is empty")
	}

	m := &PubSubMsg{}
	if err := json.Unmarshal([]byte(msg.Payload), m); err != nil {
		log.Err(err).Str("fullKey", msg.Channel).Str("data", msg.Payload).Msg("can't unmarshal data")
		return nil, err
	}

	pair := &store.KVPair{Key: key}

	// CRC is empty, if key id deleted
	if m.CRC != "" {
		pair.Value = []byte(m.Data)
		pair.LastIndex = binary.LittleEndian.Uint64([]byte(m.CRC))
	}

	return pair, nil
}

// Put set key and value with option
func (r *redisKV) Put(key string, value []byte, options *store.WriteOptions) error {
	if len(value) > limitValueSize {
		return errTooLarge
	}

	expire := 0
	if options != nil {
		if options.IsDir {
			return nil
		}

		expire = int(options.TTL / time.Second)
	}

	return r.scripts[scriptPut].Run(context.TODO(), r.cli, []string{r.makeKey(key)}, value, expire).Err()
}

// Get obtain value by key form store
func (r *redisKV) Get(key string) (*store.KVPair, error) {
	ret, err := r.scripts[scriptIdxGet].Run(context.TODO(), r.cli, []string{r.makeKey(key)}).StringSlice()
	if err != nil {
		return nil, err
	}

	if len(ret) < 2 {
		return nil, store.ErrKeyNotFound
	}

	return &store.KVPair{
		Key:       key,
		Value:     []byte(ret[0]),
		LastIndex: binary.LittleEndian.Uint64([]byte(ret[1])),
	}, nil
}

// Delete ...
func (r *redisKV) Delete(key string) error {
	return r.scripts[scriptDel].Run(context.TODO(), r.cli, []string{r.makeKey(key)}).Err()
}

// Exists ...
func (r *redisKV) Exists(key string) (bool, error) {
	ret, err := r.cli.Exists(context.TODO(), r.makeKey(key)).Result()
	return ret == 1, err
}

// Watch ...
func (r *redisKV) Watch(key string, stopCh <-chan struct{}) (<-chan *store.KVPair, error) {
	watchCh := make(chan *store.KVPair)
	pb := r.cli.Subscribe(context.TODO(), r.makeKey(key))

	go func() {
		defer close(watchCh)

		for {
			select {
			case <-stopCh:
				return
			case msg, ok := <-pb.Channel():
				if !ok {
					return
				}

				pair, err := r.parseMessage(msg)
				if err != nil {
					continue
				}

				watchCh <- pair
			}
		}
	}()

	return watchCh, nil
}

// WatchTree ...
func (r *redisKV) WatchTree(dir string, stopCh <-chan struct{}) (<-chan []*store.KVPair, error) {
	watchCh := make(chan []*store.KVPair)
	pb := r.cli.PSubscribe(context.TODO(), r.makeChannelPattern(dir))

	go func() {
		defer close(watchCh)

		for {
			select {
			case <-stopCh:
				return

			case msg, ok := <-pb.Channel():
				if !ok {
					return
				}

				pair, err := r.parseMessage(msg)
				if err != nil {
					continue
				}

				watchCh <- []*store.KVPair{pair}
			}
		}
	}()

	return watchCh, nil
}

// NewLock ...
func (r *redisKV) NewLock(key string, options *store.LockOptions) (store.Locker, error) {
	return &redisLock{
		LockOptions: options,
		cli:         r.cli,
		key:         r.makeKey(key),
		script:      r.scripts[scriptDelLock],
	}, nil
}

// List ...
func (r *redisKV) List(dir string) ([]*store.KVPair, error) {
	cur := uint64(0)
	pairs := make([]*store.KVPair, 0, 16)
	for {
		keys, cur, err := r.cli.Scan(context.TODO(), cur, r.makeDir(dir), 100).Result()
		if err != nil {
			return nil, err
		}

		for _, k := range keys {
			v, err := r.Get(r.trimPrefix(k))
			if err != nil {
				log.Warn().Err(err).Msg("can't get value")
				continue
			}

			pairs = append(pairs, v)
		}

		if cur == 0 {
			break
		}
	}

	return pairs, nil
}

// DeleteTree ...
func (r *redisKV) DeleteTree(dir string) error {
	cur := uint64(0)
	tree := make([]string, 0, 16)
	for {
		keys, cur, err := r.cli.Scan(context.TODO(), cur, r.makeDir(dir), 100).Result()
		if err != nil {
			return err
		}

		tree = append(tree, keys...)
		if cur == 0 {
			break
		}
	}

	return r.scripts[scriptDel].Run(context.TODO(), r.cli, tree).Err()
}

// AtomicPut ...
func (r *redisKV) AtomicPut(key string, value []byte, pre *store.KVPair, options *store.WriteOptions) (bool, *store.KVPair, error) {
	if len(value) > limitValueSize {
		return false, nil, errTooLarge
	}

	if pre == nil {
		return false, nil, store.ErrPreviousNotSpecified
	}

	if key != pre.Key {
		return false, nil, errDiffKey
	}

	expire := 0
	if options != nil {
		if options.IsDir {
			return false, nil, errNoDirAtomic
		}
		expire = int(options.TTL / time.Second)
	}

	crc := make([]byte, 8)
	binary.LittleEndian.PutUint64(crc, pre.LastIndex)
	txt, err := r.scripts[scriptCASPut].Run(context.TODO(), r.cli, []string{r.makeKey(key)}, crc, value, expire).Text()

	if err != nil {
		return false, nil, err
	}

	if txt == "" {
		return false, nil, store.ErrKeyModified
	}

	return true, &store.KVPair{
		Key:       key,
		Value:     value,
		LastIndex: binary.LittleEndian.Uint64([]byte(txt)),
	}, nil
}

// AtomicDelete ...
func (r *redisKV) AtomicDelete(key string, pre *store.KVPair) (bool, error) {
	if pre == nil {
		return false, store.ErrPreviousNotSpecified
	}

	if key != pre.Key {
		return false, errDiffKey
	}

	crc := make([]byte, 8)
	binary.LittleEndian.PutUint64(crc, pre.LastIndex)
	ret, err := r.scripts[scriptCASDel].Run(context.TODO(), r.cli, []string{r.makeKey(key)}, crc).Bool()

	if err != nil {
		return false, err
	}

	if !ret {
		return false, store.ErrKeyModified
	}

	return true, nil
}

func (r *redisKV) Close() {
	_ = r.cli.Close()
}
