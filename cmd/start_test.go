package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestStartNoURLs ensures the start commadn cannot be called without url params
func TestStartNoURLs(t *testing.T) {
	// ctx, _ := context.WithCancel(context.Background())
	err := startCmd.PreRunE(startCmd, []string{})
	assert.Error(t, err, "should fail with no args")
	t.Log(err)
}
