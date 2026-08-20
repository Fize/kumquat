package dto

import (
	"time"

	"github.com/fize/kumquat/apiserver/pkg/model"
)

// ModuleDTO is the only wire representation of a Module. In particular,
// ParentID is the stable public ID rather than the database key.
type ModuleDTO struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	ParentID  string      `json:"parent_id,omitempty"`
	Level     int         `json:"level"`
	Sort      int         `json:"sort"`
	Path      string      `json:"path"`
	Children  []ModuleDTO `json:"children,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

func ModuleFromModel(in model.Module) ModuleDTO {
	out := ModuleDTO{ID: in.PublicID, Name: in.Name, Level: in.Level, Sort: in.Sort, Path: in.Path, CreatedAt: in.CreatedAt, UpdatedAt: in.UpdatedAt}
	if in.Parent != nil {
		out.ParentID = in.Parent.PublicID
	}
	if len(in.Children) > 0 {
		out.Children = make([]ModuleDTO, len(in.Children))
		for i := range in.Children {
			out.Children[i] = ModuleFromModel(in.Children[i])
		}
	}
	return out
}

type PermissionDTO struct {
	Resource string `json:"resource"`
	Action   string `json:"action"`
	Effect   string `json:"effect"`
}

func PermissionFromModel(in model.Permission) PermissionDTO {
	return PermissionDTO{Resource: in.Resource, Action: in.Action, Effect: in.Effect}
}
