package openstack

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_NewClient(t *testing.T) {
	ctx := context.TODO()
	assert := require.New(t)
	s, err := SetupOpenstackSecretFromEnv("devstack")
	assert.NoError(err, "expected no error in generation of secret")
	endpoint, region, err := SetupOpenstackSourceFromEnv()
	assert.NoError(err, "expected no error in generation of source")
	c, err := NewClient(ctx, endpoint, region, s)
	assert.NoError(err, "expect no error during client generation")
	assert.NotNil(c, "expected a valid client")
	err = c.Verify()
	assert.NoError(err, "expect no error during verify of client")
}
