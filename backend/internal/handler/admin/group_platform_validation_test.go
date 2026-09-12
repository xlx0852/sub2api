package admin

import (
	"github.com/gin-gonic/gin/binding"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGroupPlatformValidation(t *testing.T) {
	require.NoError(t, binding.Validator.ValidateStruct(&CreateGroupRequest{Name: "OpenAI", Platform: "openai"}))
	require.NoError(t, binding.Validator.ValidateStruct(&UpdateGroupRequest{Platform: "openai"}))
	require.Error(t, binding.Validator.ValidateStruct(&CreateGroupRequest{Name: "Invalid", Platform: "unsupported"}))
	require.Error(t, binding.Validator.ValidateStruct(&UpdateGroupRequest{Platform: "unsupported"}))
}
