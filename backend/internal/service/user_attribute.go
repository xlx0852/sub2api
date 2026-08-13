package service

import (
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Error definitions for user attribute operations
var (
	ErrInvalidAttributeType      = infraerrors.BadRequest("INVALID_ATTRIBUTE_TYPE", "invalid attribute type")
	ErrAttributeValidationFailed = infraerrors.BadRequest("ATTRIBUTE_VALIDATION_FAILED", "attribute value validation failed")
)

const (
	AttributeTypeText        UserAttributeType = "text"
	AttributeTypeTextarea    UserAttributeType = "textarea"
	AttributeTypeNumber      UserAttributeType = "number"
	AttributeTypeEmail       UserAttributeType = "email"
	AttributeTypeURL         UserAttributeType = "url"
	AttributeTypeDate        UserAttributeType = "date"
	AttributeTypeSelect      UserAttributeType = "select"
	AttributeTypeMultiSelect UserAttributeType = "multi_select"
)

// CreateAttributeDefinitionInput for creating new definition
type CreateAttributeDefinitionInput struct {
	Key         string
	Name        string
	Description string
	Type        UserAttributeType
	Options     []UserAttributeOption
	Required    bool
	Validation  UserAttributeValidation
	Placeholder string
	Enabled     bool
}

// UpdateAttributeDefinitionInput for updating definition
type UpdateAttributeDefinitionInput struct {
	Name        *string
	Description *string
	Type        *UserAttributeType
	Options     *[]UserAttributeOption
	Required    *bool
	Validation  *UserAttributeValidation
	Placeholder *string
	Enabled     *bool
}
