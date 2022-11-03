package rbac

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_cache_isValid(t *testing.T) {
	mock := cacheMetadata{
		updatedAt: time.Now(),
	}

	assert.True(t, mock.isValid())
}

func Test_cache_isValid_customTTL(t *testing.T) {
	mock := cacheMetadata{
		ttl:       1 * time.Millisecond,
		updatedAt: time.Now(),
	}
	time.Sleep(1 * time.Millisecond)

	assert.False(t, mock.isValid())
}
