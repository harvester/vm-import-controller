package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"golang.org/x/sync/errgroup"
)

const defaultPort = 8080

const tmpDir = "/tmp/vm-import-controller"

func NewServer(ctx context.Context) error {
	err := createTmpDir()
	if err != nil {
		return err
	}
	return newServer(ctx, tmpDir)
}

func newServer(ctx context.Context, path string) error {
	defer os.RemoveAll(tmpDir)
	srv := http.Server{
		Addr: fmt.Sprintf(":%d", defaultPort),
		// fix G114: Use of net/http serve function that has no support for setting timeouts (gosec)
		// refer to https://app.deepsource.com/directory/analyzers/go/issues/GO-S2114
		ReadHeaderTimeout: 10 * time.Second,
		Handler:           http.FileServer(http.Dir(path)),
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

func createTmpDir() error {
	_, err := os.Stat(tmpDir)
	if err != nil {
		if os.IsNotExist(err) {
			return os.Mkdir("/tmp/vm-import-controller", 0755)
		}
		return err
	}
	return nil
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
