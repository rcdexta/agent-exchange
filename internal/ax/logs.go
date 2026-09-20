package ax

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const maxLogBytes = 8 << 20

// Keep a real descriptor for native children and detached brokers. An AX
// launcher exiting must not turn a backend's diagnostic output into SIGPIPE.
func openDiagnosticLog(dir, name string) (*os.File, error) {
	f, err := privateFile(filepath.Join(dir, name), syscall.O_CREAT|syscall.O_WRONLY|syscall.O_APPEND)
	if err != nil {
		return nil, err
	}
	trimDiagnosticLog(f, filepath.Join(dir, name+".lock"))
	return f, nil
}

func trimDiagnosticLog(f *os.File, lockPath string) {
	info, err := f.Stat()
	if err != nil || info.Size() <= maxLogBytes {
		return
	}
	lock, err := lockFile(lockPath)
	if err != nil {
		return
	}
	defer lock.Close()
	// Truncate the same inode; children retain open append descriptors. A
	// concurrent write can exceed the threshold until the next check.
	if info, err = f.Stat(); err == nil && info.Size() > maxLogBytes {
		_ = f.Truncate(0)
	}
}

func watchDiagnosticLog(ctx context.Context, dir, name string) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			f, err := openDiagnosticLog(dir, name)
			if err == nil {
				f.Close()
			}
		}
	}
}
