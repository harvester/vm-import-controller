package qemu

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_ConvertVMDKToRaw(t *testing.T) {
	assert := require.New(t)
	tmpDir, err := os.MkdirTemp("/tmp", "disk-test")
	assert.NoError(err, "expected no error during creation of tmpDir")
	defer os.RemoveAll(tmpDir)
	tmpVMDK := filepath.Join(tmpDir, "vmdktest.vmdk")
	err = createVMDK(tmpVMDK, "512M")
	assert.NoError(err, "expected no error during tmp vmdk creation")
	destRaw := filepath.Join(tmpDir, "vmdktest.img")
	err = ConvertVMDKtoRAW(tmpVMDK, destRaw)
	assert.NoError(err, "expected no error during disk conversion")
	f, err := os.Stat(destRaw)
	assert.NoError(err, "expected no error during check for raw file")
	assert.NotNil(f, "expect file to be not nil")
}
