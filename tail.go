package filefollow

import (
	"context"
	"os"
	"runtime"
	"time"

	"github.com/daominah/gomicrokit/gofast"
	"github.com/daominah/gomicrokit/log"
	"io/ioutil"
	"bufio"
	"bytes"
)

// Follower _
type Follower struct {
	// file need to follow
	filePath string
	// sleeping duration in loop check file stat
	pollInterval time.Duration
	// if isWriterTruncate is true, func Follow reopen file on modified,
	// default assume writer append the file
	isWriterTruncate bool

	// file descriptor
	fd           *os.File
	lastFileInfo os.FileInfo
	// save where to continue to read if the file got appended
	lastOffset   int
	outputChan   chan []byte
	stopDoneChan <-chan struct{}
	stopCxl      context.CancelFunc
}

// NewFollower init a Follower,
// read Follower fields comment for input meaning
func NewFollower(filePath string, isWriterTruncate bool) *Follower {
	ctx, cxl := context.WithCancel(context.Background())
	ret := &Follower{
		filePath:         filePath,
		pollInterval:     100 * time.Millisecond,
		isWriterTruncate: isWriterTruncate,

		outputChan:   make(chan []byte),
		stopDoneChan: ctx.Done(),
		stopCxl:      cxl,
	}
	go ret.Follow()
	return ret
}

// Follow reads until EOF, send data to the outputChan and
// wait for file modification
func (flr *Follower) Follow() {
	for i := 0; i > -1; i++ {
		select {
		case <-flr.stopDoneChan:
			break
		default:
			// pass
		}
		if flr.fd == nil {
			fd, err := os.Open(flr.filePath)
			if err != nil {
				if i == 0 {
					log.Debugf("error when first time os_Open: %v", err)
				}
				time.Sleep(flr.pollInterval)
				continue
			}
			flr.fd = fd
		}
		bufio.NewReader(flr.fd)
	}
}

// checkFileChanged modifies flr_lastFileInfo
func (flr *Follower) checkFileChanged() (
	isNewFile bool, isModified bool, err error) {
	fi, err := os.Stat(flr.filePath)
	if err != nil {
		if os.IsNotExist(err) || (runtime.GOOS == "windows" && os.IsPermission(err)) {
			return true, true, nil
		}
		return false, false, err
	}

	// first time successfully stat the file
	if gofast.CheckNilInterface(flr.lastFileInfo) {
		flr.lastFileInfo = fi
		return true, true, nil
	}

	// file got moved or renamed
	if !os.SameFile(flr.lastFileInfo, fi) {
		return true, true, nil
	}

	// file got truncated or appended
	if flr.lastFileInfo.ModTime() != fi.ModTime() {
		return false, true, nil
	}

	// file has no changes
	return false, false, nil
}
