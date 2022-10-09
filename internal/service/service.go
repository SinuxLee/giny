package service

import (
	"github.com/sinuxlee/giny/internal/api"
	"github.com/sinuxlee/giny/internal/backend"
)

// Service ...
type Service struct {
	Cluster backend.Cluster
	API     []api.API
}

func (s *Service) Match() {

}
