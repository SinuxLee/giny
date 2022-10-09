package backend

import (
	"sync"
)

// Cluster ...
type Cluster struct {
	ID    string
	Nodes sync.Map // map[string]*Node
}

// GetNodes Get All Nodes in the Cluster
func (c *Cluster) GetNodes() []*Node {
	nodes := make([]*Node, 0, 16)
	c.Nodes.Range(func(_, node interface{}) bool {
		if n, ok := node.(*Node); ok && n != nil {
			nodes = append(nodes, n)
		}
		return true
	})

	return nodes
}
