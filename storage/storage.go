package storage

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ernestrc/blue/document"
	"github.com/ernestrc/blue/document/firstmover"
	"github.com/ernestrc/blue/encoding"
	"unstable.build/go-tui/config"
	workdoc "unstable.build/go-tui/storage/workspace"
	"unstable.build/go-tui/workspace"
)

// New returns a document.Service storage service that
// uses the local directory dir to setup a local filesystem-based
// multi-process safe, goroutine-safe document.Service.
func New(dir string, marshaler encoding.Marshaler) (document.Service, error) {
	storageDir := filepath.Join(dir, ".db")
	err := os.MkdirAll(storageDir, 0777)
	if err != nil {
		return nil, fmt.Errorf("mkdir: %v", err)
	}
	storageDirURI, err := workspace.CurrentUserHostURI(storageDir)
	if err != nil {
		return nil, fmt.Errorf("URI: %v", err)
	}
	scheme, err := workspace.NewFileScheme(config.NopConfig(), storageDirURI)
	if err != nil {
		return nil, err
	}
	storage, err := workdoc.NewWorkspaceService(scheme, marshaler)
	if err != nil {
		return nil, err
	}

	// place lock path at parent dir of .db
	lockPath := filepath.Join(dir, ".dblock")

	cfg := firstmover.DefaultConfig()
	cfg.Marshaler = marshaler
	cfg.CloseError = workdoc.ErrClosing
	storage = firstmover.New(storage, lockPath, cfg)
	return storage, nil
}
