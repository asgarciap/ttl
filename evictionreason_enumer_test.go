package ttl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestError(t *testing.T) {
	assert.Equal(t, "key not found", ErrNotFound.Error())

}

func TestEvictionError(t *testing.T) {
	assert.Equal(t, "Removed", Removed.String())
	assert.Equal(t, "Expired", Expired.String())
	assert.Equal(t, "EvictionReason(50)", EvictionReason(50).String())
}

func TestEvictionResonString(t *testing.T) {
	reason, err := EvictionReasonString("Removed")
	assert.Equal(t, reason, Removed)
	assert.Nil(t, err)
	_, err = EvictionReasonString("NotValid")
	assert.NotNil(t, err)
}

func TestIsAEvictionReson(t *testing.T) {
	assert.True(t, Closed.IsAEvictionReason())
	assert.True(t, Removed.IsAEvictionReason())
	assert.True(t, EvictedSize.IsAEvictionReason())
	assert.True(t, Expired.IsAEvictionReason())
	assert.False(t, EvictionReason(50).IsAEvictionReason())
}

func TestGetEvictionResonValues(t *testing.T) {
	assert.NotEmpty(t, EvictionReasonValues())
	assert.Equal(t, len(EvictionReasonValues()), 4)
}
