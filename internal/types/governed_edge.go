package types

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// GovernedEdgeBinding is Platform-owned product connection authority. Revision
// serializes product binding changes; DeploymentRevision records the Center
// node-control revision that was explicitly accepted for the current node.
type GovernedEdgeBinding struct {
	BindingID         string `json:"binding_id"`
	Revision          int64  `json:"revision"`
	DeploymentRevision int64  `json:"deployment_revision"`
	EnterpriseID      string `json:"enterprise_id"`
	EdgeNodeID        string `json:"edge_node_id"`
	SourceID          string `json:"source_id"`
	Enabled           bool   `json:"enabled"`
}

func (b GovernedEdgeBinding) Value() (driver.Value, error) { return json.Marshal(b) }
func (b *GovernedEdgeBinding) Scan(value any) error {
	raw, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("invalid governed Edge binding storage")
	}
	return json.Unmarshal(raw, b)
}

// GovernedEdgeConnection is server-owned connection authority. It is never an
// agent argument, public tenant setting, history payload or sandbox credential.
type GovernedEdgeConnection struct {
	EnterpriseID string `json:"-"`
	EdgeNodeID   string `json:"-"`
	SourceID     string `json:"-"`
	BindingID    string `json:"-"`
	Revision     int64  `json:"-"`
	DeploymentRevision int64 `json:"-"`
	BaseURL      string `json:"-"`
	Token        string `json:"-"`
}

func (GovernedEdgeConnection) String() string   { return "[governed Edge connection]" }
func (GovernedEdgeConnection) GoString() string { return "[governed Edge connection]" }
