package filefollow

import (
	"bytes"
	"context"
	"os"
	"runtime"
	"time"

	"github.com/daominah/gomicrokit/gofast"
	"github.com/daominah/gomicrokit/log"
)

// Follower _
type Follower struct {
	// file need to follow
	filePath string
	// sleeping duration in loop check file stat
	pollInterval time.Duration
	// if isWriterTruncate is true, func follow reopen file on modified,
	// default assume writer append the file
	isWriterTruncate bool

	// file descriptor
	fd           *os.File
	lastFileInfo os.FileInfo
	// save where to continue to read if the file got appended
	lastOffset int
	OutputChan chan []byte
	// call this func tell Follower to stop loops, release resources
	stopCxl context.CancelFunc
	// receive StopDoneChan to know when the Follower stop
	StopDoneChan <-chan struct{}
}

// NewFollower init a Follower,
// read Follower fields comment for input meaning
func NewFollower(filePath string, isWriterTruncate bool) *Follower {
	ctx, cxl := context.WithCancel(context.Background())
	ret := &Follower{
		filePath:         filePath,
		pollInterval:     100 * time.Millisecond,
		isWriterTruncate: isWriterTruncate,

		OutputChan:   make(chan []byte),
		StopDoneChan: ctx.Done(),
		stopCxl:      cxl,
	}
	go ret.follow()
	return ret
}

// follow reads until EOF, send data to the OutputChan and
// wait for file modification
func (flr *Follower) follow() {
	for i := 0; i > -1; i++ { // this loop break if Follower stop
		if true || i%1000 == 0 {
			log.Debugf("loop %v of Follower_follow", i)
		}
		select {
		case <-flr.StopDoneChan:
			break
		default:
		}

		// reopen the file if needed
		if flr.fd == nil {
			fd, err := os.Open(flr.filePath)
			log.Debugf("loop %v os_Open err: %v", i, err)
			if err != nil {
				if i == 0 {
					log.Infof("error when first time os_Open: %v", err)
				}
				time.Sleep(flr.pollInterval)
				continue
			}
			flr.fd = fd
			flr.lastFileInfo, _ = fd.Stat()
		}

		// read until EOF
		buf := bytes.NewBuffer(nil)
		n, err := buf.ReadFrom(flr.fd)
		if n > 0 {
			select {
			case flr.OutputChan <- buf.Bytes():
			case <-flr.StopDoneChan:
			}
		}
		if err != nil {
			log.Infof("error when bytes_Buffer_ReadFrom: %v", err)
			flr.fd.Close()
			flr.Stop()
		}

		// wait for the file modification
		changedType := Unchanged
		shouldStop := false
		for err == nil && changedType == Unchanged && !shouldStop {
			select {
			case <-flr.StopDoneChan:
				shouldStop = true
			default:
			}
			changedType, err = flr.checkFileChanged()
			time.Sleep(flr.pollInterval)
		}
		if err != nil {
			log.Infof("error when checkFileChanged: %v", err)
			flr.fd.Close()
			flr.Stop()
		}
		switch changedType {
		case Appended:
			continue
		case Truncated:
			if !flr.isWriterTruncate { // handling same as append
				continue
			}
			fallthrough // handling same as new file
		case Created:
			flr.fd.Close()
			flr.fd = nil
			continue
		}
	}
}

// Stop stops loops, release resources
func (flr Follower) Stop() {
	log.Infof("the Follower about to stop")
	flr.stopCxl()
}

// ChangedType is type of file modification
type ChangedType string

// ChangedType enum
const (
	Unchanged ChangedType = ""
	Created   ChangedType = "created"
	Truncated ChangedType = "truncated"
	Appended  ChangedType = "appended"
)

// checkFileChanged modifies flr_lastFileInfo
func (flr *Follower) checkFileChanged() (ChangedType, error) {
	fi, err := os.Stat(flr.filePath)
	if err != nil {
		if os.IsNotExist(err) || (runtime.GOOS == "windows" && os.IsPermission(err)) {
			return Created, nil
		}
		return Created, err
	}
	lastFI := flr.lastFileInfo
	flr.lastFileInfo = fi

	if gofast.CheckNilInterface(lastFI) {
		return Created, nil
	}

	// file got moved or renamed
	if !os.SameFile(lastFI, fi) {
		return Created, nil
	}

	// file got modified
	if lastFI.ModTime() != fi.ModTime() {
		if lastFI.Size() >= fi.Size() {
			return Truncated, nil
		}
		return Appended, nil
	}

	// file has no changes
	return Unchanged, nil
}
