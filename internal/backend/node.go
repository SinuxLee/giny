package backend

const (
	NodeOffline  NodeState = iota
	NodeOnline             = 1
	NodeMaintain           = 2
)

type NodeState int

// Node ...
type Node struct {
	ID          string    `json:"id"`
	State       NodeState `json:"state"`
	Schema      string    `json:"schema"`
	Addr        string    `json:"addr"`
	Ignore      bool      `json:"ignore"`
	FailTimeout int64     `json:"failTimeout"`
	MaxFails    int       `json:"maxFails"`
	Weight      int       `json:"weight"`
	Tag         []string  `json:"tag"`
}
