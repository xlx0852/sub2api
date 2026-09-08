package cursor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCreateCursorChecksum(t *testing.T) {
	machineID := "fd065226-77db-45bf-abda-3e11d4f26c67"
	now := time.UnixMilli(1788887269000)

	checksum := CreateCursorChecksum(machineID, now)
	expected := "7jo8KmW3fd065226-77db-45bf-abda-3e11d4f26c67"

	assert.Equal(t, expected, checksum)
}

func TestEnhancedObfuscate_Empty(t *testing.T) {
	res := EnhancedObfuscate([]byte{})
	assert.Empty(t, res)
}

func TestEnhancedObfuscate_PreservesLength(t *testing.T) {
	input := []byte{1, 2, 3, 4, 5, 6}
	res := EnhancedObfuscate(input)
	assert.Equal(t, len(input), len(res))
	assert.NotEqual(t, input, res)
}
