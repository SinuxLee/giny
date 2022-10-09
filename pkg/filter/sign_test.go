package filter

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/rs/xid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

const (
	appSecret = "secret"
	signPath  = "/giny/sign"
)

func init() {
	log.Logger = zerolog.New(io.Discard)
	gin.SetMode(gin.ReleaseMode)
}

type SignatureSuite struct {
	suite.Suite
	ts *httptest.Server
}

func (s *SignatureSuite) SetupSuite() {
	r := gin.New()
	r.Use(CheckSign(appSecret)).GET(signPath, func(c *gin.Context) {
		c.String(http.StatusOK, "giny")
	})

	s.ts = httptest.NewServer(r)
}

func (s *SignatureSuite) TearDownSuite() {
	s.ts.Close()
}

func (s *SignatureSuite) newRequest() *http.Request {
	req, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("%v%v?name=sinux&age=18&special=!@$^&*()_+#frame", s.ts.URL, signPath),
		nil,
	)

	assert.Nil(s.T(), err)
	return req
}

func (s *SignatureSuite) parseResponse(rsp *http.Response) string {
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

func setSignHeader(req *http.Request) {
	token, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, &jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		ID:        xid.New().String(),
	}).SignedString([]byte(appSecret))
	ts := cast.ToString(time.Now().Unix())
	nonce := xid.New().String()

	req.Header.Set(tsKey, ts)
	req.Header.Set(nonceKey, nonce)
	req.Header.Set(tokenKey, token)

	sign := calcSign(req, appSecret)
	req.Header.Set(signKey, sign)
}

// TestCheckSignOK ...
func (s *SignatureSuite) TestCheckSignOK() {
	req := s.newRequest()
	setSignHeader(req)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		s.T().Fatal(err)
	}

	resp, _ := ioutil.ReadAll(res.Body)
	_ = res.Body.Close()
	assert.Equal(s.T(), "giny", string(resp))
}

// TestCheckSignBad ...
func (s *SignatureSuite) TestCheckSignBad() {
	req, err := http.NewRequest(
		http.MethodGet,
		fmt.Sprintf("%v%v?name=sinux&age=18&special=!@$^&*()_+#frame", s.ts.URL, signPath),
		nil,
	)
	if err != nil {
		panic(err)
	}
	req.Header.Set(signKey, xid.New().String())
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		s.T().Fatal(err)
	}

	resp, _ := ioutil.ReadAll(res.Body)
	_ = res.Body.Close()

	h := gin.H{}
	_ = json.Unmarshal(resp, &h)
	assert.Equal(s.T(), "invalid sign", h["msg"])
}

// TestCheckSignNoHeader ...
func (s *SignatureSuite) TestCheckSignNoHeader() {
	res, err := http.Get(fmt.Sprintf("%v%v", s.ts.URL, signPath))
	if err != nil {
		s.T().Fatal(err)
	}

	resp, _ := ioutil.ReadAll(res.Body)
	_ = res.Body.Close()

	h := gin.H{}
	_ = json.Unmarshal(resp, &h)
	assert.Equal(s.T(), "empty sign", h["msg"])
}

func TestCheckSign(t *testing.T) {
	suite.Run(t, new(SignatureSuite))
}

func BenchmarkCheckSign(b *testing.B) {
	r := gin.New()
	r.Use(CheckSign(appSecret)).GET(signPath, func(c *gin.Context) {
		c.String(http.StatusOK, "giny")
	})

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodGet, signPath, nil)
	if err != nil {
		panic(err)
	}
	setSignHeader(req)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.ServeHTTP(w, req)
	}

	b.StopTimer()
}
