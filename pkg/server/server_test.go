package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_NewServer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	assert := require.New(t)
	var err error
	err = createTmpDir()
	assert.NoError(err, "expected no error during creation of tmp dir")
	go func() {
		err = newServer(ctx, tmpDir)
		assert.Contains(err.Error(), "http: Server closed", "error occurred during shutdown") //only expected error is context canceled
	}()
	time.Sleep(1 * time.Second)
	f, err := os.MkdirTemp(TempDir(), "sample")
	assert.NoError(err, "expect no error during creation of tmp file")
	_, relative := filepath.Split(f)
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/%s", defaultPort, relative))
	assert.NoError(err, "expect no error during http call")
	assert.Equal(resp.StatusCode, 200, "expected http response code to be 200")
	cancel()
	time.Sleep(5 * time.Second)
	_, err = os.Stat(f)
	assert.True(os.IsNotExist(err), "expected no file to be found")
}
