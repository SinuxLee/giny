package api

import (
	"regexp"

	"github.com/pkg/errors"

	"github.com/bmatcuk/doublestar"
	"github.com/rs/zerolog/log"
)

const (
	Private    Level = iota // for auth, payment etc service
	Inner                   // for platform service
	ThirdParty              // for other company
	Public                  // for player
	MaxLevel                // bound
)

const (
	GlobStyle  = "glob"  // glob pattern style
	RegexStyle = "regex" // regex pattern style
)

type Level uint8

func New(lv Level, style string, pattern string) (*API, error) {
	if lv >= MaxLevel {
		return nil, errors.New("invalid api level")
	}

	if style != GlobStyle && style != RegexStyle {
		return nil, errors.New("invalid pattern style")
	}

	api := &API{
		lv:      lv,
		style:   style,
		pattern: pattern,
	}

	if style == RegexStyle {
		reg, err := regexp.Compile(pattern)
		if err != nil {
			return nil, errors.Wrapf(err, "%v style", style)
		}
		api.Regexp = reg
	}

	return api, nil
}

// API ...
type API struct {
	*regexp.Regexp
	lv      Level
	style   string // glob, regex
	pattern string // like: api/**, ^api/.*$
}

func (a *API) IsMatch(url string) bool {
	switch a.style {
	case GlobStyle:
		match, err := doublestar.Match(a.pattern, url)
		if err != nil {
			log.Err(err).Str("style", a.style).Str("pattern", a.pattern).Msg("api match error")
		}
		return match
	case RegexStyle:
		return a.MatchString(url)
	}

	log.Warn().Str("style", a.style).Msg("unknown pattern style")
	return false
}
