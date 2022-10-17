package storeadapter

import (
	"encoding/json"
	"path"

	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
	"github.com/docker/libkv/store"
)

const (
	PolicyKey = "casbinPolicy"
	ModelKey  = "casbinModel"
)

type Model struct {
	Sec   string `json:"sec"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Policy []string

func NewAdapter(s store.Store, p string) *Adapter {
	return &Adapter{
		store:  s,
		prefix: p,
	}
}

type Adapter struct {
	store  store.Store
	prefix string
}

func (a *Adapter) makeKey(key string) string {
	return path.Join(a.prefix, key)
}

func (a *Adapter) LoadPolicy(model model.Model) error {
	policy := make(Policy, 0)
	pair, err := a.store.Get(a.makeKey(PolicyKey))
	if err != nil {
		return err
	}

	if err = json.Unmarshal(pair.Value, &policy); err != nil {
		return err
	}

	for _, p := range policy {
		if err = persist.LoadPolicyLine(p, model); err != nil {
			return err
		}
	}

	return nil
}

func (a *Adapter) SavePolicy(model model.Model) error {
	panic("implement me")
}

func (a *Adapter) AddPolicy(sec string, ptype string, rule []string) error {
	panic("implement me")
}

func (a *Adapter) RemovePolicy(sec string, ptype string, rule []string) error {
	panic("implement me")
}

func (a *Adapter) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
	panic("implement me")
}
