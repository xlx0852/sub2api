package admin

import (
	"github.com/gin-gonic/gin/binding"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGroupPlatformValidationCursor(t *testing.T) {
	require.NoError(t, binding.Validator.ValidateStruct(&CreateGroupRequest{Name: "Cursor", Platform: "cursor"}))
	require.NoError(t, binding.Validator.ValidateStruct(&UpdateGroupRequest{Platform: "cursor"}))
	require.Error(t, binding.Validator.ValidateStruct(&CreateGroupRequest{Name: "Invalid", Platform: "unsupported"}))
	require.Error(t, binding.Validator.ValidateStruct(&UpdateGroupRequest{Platform: "unsupported"}))
}
