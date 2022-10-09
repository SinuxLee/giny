package filter

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/rs/xid"
	"github.com/spf13/cast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

const (
	replayPath    = "/giny/replay"
	contentHeader = "Content-Type"
)

type ReplaySuite struct {
	suite.Suite
	ts *httptest.Server
	rs *miniredis.Miniredis
}

func (s *ReplaySuite) SetupSuite() {
	rand.Seed(time.Now().Unix())

	s.rs = miniredis.RunT(s.T())
	assert.NotNil(s.T(), s.rs)

	rdb := redis.NewClient(&redis.Options{
		Addr: s.rs.Addr(),
	})
	assert.NotNil(s.T(), rdb)

	r := gin.New()
	r.Use(Replay(rdb)).GET(replayPath, func(c *gin.Context) {
		c.String(http.StatusOK, "giny")
	})

	s.ts = httptest.NewServer(r)
	assert.NotNil(s.T(), s.ts)
}

func (s *ReplaySuite) TearDownSuite() {
	s.ts.Close()
	s.T().Log(s.rs.Keys())
}

func (s *ReplaySuite) newRequest() *http.Request {
	req, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%v%v?name=sinux", s.ts.URL, replayPath),
		nil,
	)

	assert.Nil(s.T(), err)
	return req
}

func (s *ReplaySuite) parseResponse(rsp *http.Response) string {
	defer rsp.Body.Close()
	res, err := ioutil.ReadAll(rsp.Body)
	assert.Nil(s.T(), err)

	t := rsp.Header.Get(contentHeader)
	switch {
	case strings.Contains(t, gin.MIMEJSON):
		h := gin.H{}
		err = json.Unmarshal(res, &h)
		assert.Nil(s.T(), err)

		msg, ok := h["msg"]
		assert.True(s.T(), ok, true)

		return msg.(string)
	case strings.Contains(t, gin.MIMEPlain):
		return string(res)

	default:
		s.T().Fatalf("can't handle this type: %v", t)
	}

	return ""
}

// TestReplayOK ...
func (s *ReplaySuite) TestReplayOK() {
	req := s.newRequest()
	req.Header.Set(userIDKey, cast.ToString(rand.Uint32()))
	req.Header.Set(nonceKey, xid.New().String())
	req.Header.Set(tsKey, cast.ToString(time.Now().Unix()))

	rsp, err := http.DefaultClient.Do(req)
	assert.Nil(s.T(), err)

	assert.Equal(s.T(), "giny", s.parseResponse(rsp))
}

// TestReplayNoTS ...
func (s *ReplaySuite) TestReplayNoTS() {
	req := s.newRequest()
	rsp, err := http.DefaultClient.Do(req)
	assert.Nil(s.T(), err)

	assert.Equal(s.T(), "invalid ts", s.parseResponse(rsp))
}

// TestReplayInvalidTS ...
func (s *ReplaySuite) TestReplayInvalidTS() {
	req := s.newRequest()
	req.Header.Set(tsKey, cast.ToString(time.Now().Add(time.Minute*20).Unix()))

	rsp, err := http.DefaultClient.Do(req)
	assert.Nil(s.T(), err)

	assert.Equal(s.T(), "expired ts", s.parseResponse(rsp))
}

// TestReplayNoNonce ...
func (s *ReplaySuite) TestReplayNoNonce() {
	req := s.newRequest()
	req.Header.Set(userIDKey, cast.ToString(rand.Uint32()))
	req.Header.Set(tsKey, cast.ToString(time.Now().Unix()))

	rsp, err := http.DefaultClient.Do(req)
	assert.Nil(s.T(), err)

	assert.Equal(s.T(), "empty nonce", s.parseResponse(rsp))
}

// TestReplayNoUID ...
func (s *ReplaySuite) TestReplayNoUID() {
	req := s.newRequest()
	req.Header.Set(nonceKey, xid.New().String())
	req.Header.Set(tsKey, cast.ToString(time.Now().Unix()))

	rsp, err := http.DefaultClient.Do(req)
	assert.Nil(s.T(), err)

	assert.Equal(s.T(), "invalid uid", s.parseResponse(rsp))
}

// TestReplayNoUID ...
func (s *ReplaySuite) TestReplayRequest() {
	req := s.newRequest()
	req.Header.Set(userIDKey, cast.ToString(rand.Uint32()))
	req.Header.Set(nonceKey, xid.New().String())
	req.Header.Set(tsKey, cast.ToString(time.Now().Unix()))

	rsp, err := http.DefaultClient.Do(req)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), "giny", s.parseResponse(rsp))

	rsp, err = http.DefaultClient.Do(req)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), "replay request", s.parseResponse(rsp))

	req.Header.Set(nonceKey, xid.New().String())
	rsp, err = http.DefaultClient.Do(req)
	assert.Nil(s.T(), err)
	assert.Equal(s.T(), "giny", s.parseResponse(rsp))
}

func TestReplay(t *testing.T) {
	suite.Run(t, new(ReplaySuite))
}

func BenchmarkReplay(b *testing.B) {
	r := gin.New()
	r.Use(Replay(nil)).GET(replayPath, func(c *gin.Context) {
		c.String(http.StatusOK, "giny")
	})

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, replayPath, nil)
	if err != nil {
		panic(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.ServeHTTP(w, req)
	}

	b.StopTimer()
}
