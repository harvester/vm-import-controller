package server

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"golang.org/x/sync/errgroup"
)

const defaultPort = 8080

var tmpDir string

func NewServer(ctx context.Context) error {
	var err error
	tmpDir, err = createTmpDir()
	if err != nil {
		return err
	}
	return newServer(ctx, tmpDir)
}

func newServer(ctx context.Context, path string) error {
	defer os.RemoveAll(tmpDir)
	srv := http.Server{
		Addr:    fmt.Sprintf(":%d", defaultPort),
		Handler: http.FileServer(http.Dir(path)),
	}

	eg, _ := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return srv.ListenAndServe()
	})

	eg.Go(func() error {
		<-ctx.Done()
		return srv.Shutdown(ctx)
	})

	return eg.Wait()
}

func createTmpDir() (string, error) {
	return ioutil.TempDir("/tmp", "vm-import-controller-")
}

func DefaultPort() int {
	return defaultPort
}

func TempDir() string {
	return tmpDir
}

// Address returns the address for vm-import url. For local testing set env variable
// SVC_ADDRESS to point to a local endpoint
func Address() string {
	address := "harvester-vm-import-controller.harvester-system.svc"
	if val := os.Getenv("SVC_ADDRESS"); val != "" {
		address = val
	}
	return address
}
