package rbac

import (
	"errors"
	"time"
	// "fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_cache_isValid(t *testing.T) {
	mock := cacheMetadata{
		updatedAt: time.Now(),
	}

	assert.True(t, mock.isValid())
}

func Test_cache_isValid_expired(t *testing.T) {
	mock := cacheMetadata{
		updatedAt: time.Now().Add(-3 * time.Minute),
	}

	assert.False(t, mock.isValid())
}

func Test_cache_isValid_withError(t *testing.T) {
	mock := cacheMetadata{
		err:       errors.New("Some error"),
		updatedAt: time.Now().Add(-990 * time.Millisecond),
	}
	assert.True(t, mock.isValid())
	time.Sleep(20 * time.Millisecond)
	assert.False(t, mock.isValid())
}

func Test_cache_isValid_customTTL(t *testing.T) {
	mock := cacheMetadata{
		ttl:       1 * time.Millisecond,
		updatedAt: time.Now(),
	}
	time.Sleep(1 * time.Millisecond)

	assert.False(t, mock.isValid())
}
