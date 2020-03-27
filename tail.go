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

// Follower works like "tail -f" command,
// Caller receives bytes from OutputChan and StopDoneChan
type Follower struct {
	// file need to follow
	filePath string
	// PollInterval is sleeping duration in the Follower's loops
	PollInterval time.Duration
	// if isWriterTruncate is true, func follow reopen file on modified,
	// default assume writer append the file
	isWriterTruncate bool
	// if skipCheckChange is true, read file periodically,
	// skip check file changes with os_Stat
	skipCheckChange bool

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
func NewFollower(filePath string, isWriterTruncate bool, skipCheckChange bool) *Follower {
	ctx, cxl := context.WithCancel(context.Background())
	ret := &Follower{
		filePath:         filePath,
		PollInterval:     100 * time.Millisecond,
		isWriterTruncate: isWriterTruncate,
		skipCheckChange:  skipCheckChange,

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
		//log.Debugf("loop %v of Follower_follow", i)
		select {
		case <-flr.StopDoneChan:
			break
		default:
		}

		// reopen the file if needed
		if flr.fd == nil {
			bt := time.Now()
			log.Debugf("loop %v before os_Open %v", i, flr.filePath)
			fd, err := os.Open(flr.filePath)
			log.Debugf("loop %v after os_Open %v dur: %v", i, flr.filePath, time.Now().Sub(bt))
			if err != nil {
				if i == 0 {
					log.Infof("error when first time os_Open: %v", err)
				}
				time.Sleep(flr.PollInterval)
				continue
			}
			flr.fd = fd
			flr.lastFileInfo, _ = fd.Stat()
		}

		// read until EOF
		buf := bytes.NewBuffer(nil)
		bt := time.Now()
		log.Debugf("loop %v before buf_ReadFrom %v", i, flr.filePath)
		n, err := buf.ReadFrom(flr.fd)
		log.Debugf("loop %v after buf_ReadFrom %v, dur: %v", i, flr.filePath, time.Now().Sub(bt))
		if n > 0 {
			select {
			case flr.OutputChan <- buf.Bytes():
			case <-flr.StopDoneChan:
			}
		}
		if err != nil {
			log.Infof("error when bytes_Buffer_ReadFrom: %v", err)
			flr.fd.Close()
			flr.fd = nil
			continue
		}

		if flr.skipCheckChange {
			if !flr.isWriterTruncate { // next loop read file from current offset
				time.Sleep(flr.PollInterval)
				continue
			}
			bt := time.Now()
			log.Debugf("before fd_Seek %v", flr.filePath)
			_, err := flr.fd.Seek(0, os.SEEK_SET)
			log.Debugf("after fd_Seek %v, dur: %v", flr.filePath, time.Now().Sub(bt))
			if err != nil {
				log.Infof("error when Seek %v: %v", flr.filePath, err)
				flr.fd.Close()
				flr.fd = nil
				continue
			} else {
				time.Sleep(flr.PollInterval)
				continue
			}
		}

		// wait for the file modification
		isFlrStopped := false
		changedType := Unchanged
	LoopCheckFileChanged:
		for {
			select {
			case <-flr.StopDoneChan:
				isFlrStopped = true
				break LoopCheckFileChanged
			default:
			}
			changedType, err = flr.checkFileChanged()
			if err != nil || changedType != Unchanged {
				break LoopCheckFileChanged
			}
			time.Sleep(flr.PollInterval)
		}
		if isFlrStopped {
			break
		}
		if err != nil {
			log.Infof("error when checkFileChanged: %v", err)
			flr.fd.Close()
			flr.fd = nil
			continue
		}
		switch changedType {
		case Appended:
			continue
		case Truncated:
			if !flr.isWriterTruncate { // handling same as append
				continue
			}
			bt := time.Now()
			log.Debugf("before fd_Seek %v", flr.filePath)
			_, err := flr.fd.Seek(0, os.SEEK_SET)
			log.Debugf("after fd_Seek %v, dur: %v", flr.filePath, time.Now().Sub(bt))
			if err != nil {
				log.Infof("error when Seek %v: %v", flr.filePath, err)
				flr.fd.Close()
				flr.fd = nil
				continue
			} else {
				continue // next loop read current from begin
			}
		case Created:
			flr.fd.Close()
			flr.fd = nil
			continue
		}
	}
}

// Stop stops loops, release resources
func (flr Follower) Stop() {
	log.Infof("the Follower %v about to stop", flr.filePath)
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
	bt := time.Now()
	log.Debugf("before checkFileChanged %v ", flr.filePath)
	defer func() {
		log.Debugf("after checkFileChanged %v, dur: %v", flr.filePath, time.Now().Sub(bt))
	}()
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
