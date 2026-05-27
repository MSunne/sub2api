package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAIStreamInitialBufferExpired(t *testing.T) {
	firstPendingAt := time.Unix(100, 0)
	timeout := time.Second

	require.False(t, openAIStreamInitialBufferExpired(firstPendingAt, firstPendingAt.Add(999*time.Millisecond), timeout))
	require.True(t, openAIStreamInitialBufferExpired(firstPendingAt, firstPendingAt.Add(time.Second), timeout))
	require.False(t, openAIStreamInitialBufferExpired(time.Time{}, firstPendingAt.Add(time.Second), timeout))
	require.False(t, openAIStreamInitialBufferExpired(firstPendingAt, firstPendingAt.Add(time.Hour), 0))
}
