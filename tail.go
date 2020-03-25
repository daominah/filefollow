package filefollow

import (
	"context"
	"os"
	"sync"
	"time"
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

	fileDescriptor *os.File
	outputChan     chan []byte
	// call this func to stop the Follower (clean resource))
	stopCxl context.CancelFunc
	// many goroutines updates the fileDescriptor
	mutex *sync.Mutex
}

// NewFollower init a Follower,
// read Follower fields comment for input meaning
func NewFollower(filePath string, pollInterval time.Duration,
	isWriterTruncate bool) *Follower {
	ret := &Follower{
		filePath:         filePath,
		pollInterval:     pollInterval,
		isWriterTruncate: isWriterTruncate,
	}
	return ret
}

// Follow reads until EOF, send data to the outputChan and
// wait for file modification
func (flr Follower) Follow() {
}

func (flr Follower) checkFileChanged() bool {
	f, _ := os.Stat("")
	f.ModTime()
	return false
}
