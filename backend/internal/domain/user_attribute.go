// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

var ErrAttributeDefinitionNotFound = infraerrors.NotFound("ATTRIBUTE_DEFINITION_NOT_FOUND", "attribute definition not found")

var ErrAttributeKeyExists = infraerrors.Conflict("ATTRIBUTE_KEY_EXISTS", "attribute key already exists")

// UserAttributeType represents supported attribute types
type UserAttributeType string

// UserAttributeOption represents a select option for select/multi_select types
type UserAttributeOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// UserAttributeValidation represents validation rules for an attribute
type UserAttributeValidation struct {
	MinLength *int    `json:"min_length,omitempty"`
	MaxLength *int    `json:"max_length,omitempty"`
	Min       *int    `json:"min,omitempty"`
	Max       *int    `json:"max,omitempty"`
	Pattern   *string `json:"pattern,omitempty"`
	Message   *string `json:"message,omitempty"`
}

// UserAttributeDefinition represents a custom attribute definition
type UserAttributeDefinition struct {
	ID           int64
	Key          string
	Name         string
	Description  string
	Type         UserAttributeType
	Options      []UserAttributeOption
	Required     bool
	Validation   UserAttributeValidation
	Placeholder  string
	DisplayOrder int
	Enabled      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// UserAttributeValue represents a user's attribute value
type UserAttributeValue struct {
	ID          int64
	UserID      int64
	AttributeID int64
	Value       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// UpdateUserAttributeInput for updating a single attribute value
type UpdateUserAttributeInput struct {
	AttributeID int64
	Value       string
}
