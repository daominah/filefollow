package filefollow

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"time"
)

// Follower works like "tail --follow=name" linux command,
// caller receives newly appended data as bytes from OutputChan
type Follower struct {
	FilePath   string      // file to follow
	FileName   string      // just for logging
	OutputChan chan []byte // returns newly appended data

	PollInterval time.Duration // duration between checks the file is modified
	Log          Logger        // for debugging

	// read from beginning of the file on modified or read from current offset
	IsReadFromBeginningOnModified bool
	// read the file again without check whether it is modified with os_Stat,
	// sometimes check modified is very slow on a network drive
	IsSkipCheckModified bool

	fd           *os.File
	lastFileInfo os.FileInfo
	Stop         context.CancelFunc // call this func to stop following the file
	StopDoneChan <-chan struct{}    // corresponding channel of the Stop func
}

// NewFollower start following a file with default config
func NewFollower(filePath0 string) *Follower {
	ctxStop, cclStop := context.WithCancel(context.Background())
	ret := &Follower{
		FilePath:   filePath0,
		FileName:   filepath.Base(filePath0),
		OutputChan: make(chan []byte),

		PollInterval: 100 * time.Millisecond,
		Log:          LoggerNil{},

		IsReadFromBeginningOnModified: false,
		IsSkipCheckModified:           false,

		fd:           nil, // will be defined in Follow loop
		lastFileInfo: nil,
		Stop:         cclStop,
		StopDoneChan: ctxStop.Done(),
	}
	go ret.Follow()
	return ret
}

func (f *Follower) checkShouldStop() bool {
	select {
	case <-f.StopDoneChan:
		return true
	default:
		return false
	}
}

// Follow loops following the file until Stop is called,
// each iteration reads to EOF, send data to OutputChan and wait for a file modification
func (f *Follower) Follow() {
	if f.FileName == "" {
		f.FileName = filepath.Base(f.FilePath)
	}
	for i := 0; true; i++ {
		if i > 0 {
			time.Sleep(f.PollInterval)
		}
		if f.checkShouldStop() {
			break
		}

		if f.fd == nil { // open the file or reopen if needed
			beginT := time.Now()
			fd, err := os.Open(f.FilePath)
			f.Log.Printf("i %v os_Open %v dur: %v", i, f.FileName, time.Since(beginT))
			if err != nil {
				f.Log.Printf("error os_Open: %v", err)
			}
			f.fd = fd
			f.lastFileInfo, _ = fd.Stat()
		}

		// reads to EOF

		buf := bytes.NewBuffer(nil)
		beginT := time.Now()
		n, err := buf.ReadFrom(f.fd)
		f.Log.Printf("file %v i %v buf_ReadFrom: nBytes: %v, dur: %v", f.FileName, i, n, time.Since(beginT))
		if i == 0 || n > 0 {
			// the first time open the file always send data to OutputChan,
			// even if the file is empty; from the second time only send data to
			// OutputChan if n > 0
			select {
			case f.OutputChan <- buf.Bytes():
			case <-f.StopDoneChan:
			}
		}
		if err != nil {
			f.Log.Printf("error buf_ReadFrom: %v", err)
			f.fd.Close()
			f.fd = nil
			continue
		}

		// special case: periodically read the file regardless of modification

		if f.IsSkipCheckModified {
			if !f.IsReadFromBeginningOnModified {
				continue
			}
			beginT := time.Now()
			f.Log.Printf("i %v fd_Seek %v, dur: %v", i, f.FileName, time.Since(beginT))
			_, err := f.fd.Seek(0, io.SeekStart)
			if err == nil {
				continue
			}
			f.Log.Printf("error fd_Seek %v", err)
			f.fd.Close()
			f.fd = nil
			continue
		}

		// common case: wait for a file modification then read again

		modifiedType := Unchanged
		var errCFM error
		for k := 0; true; k++ {
			if f.checkShouldStop() {
				break
			}
			modifiedType, errCFM = f.checkFileModified()
			f.Log.Printf("file %v i %v k %v checkFileModified: %v, err: %v", f.FileName, i, k, modifiedType, errCFM)
			if errCFM != nil || modifiedType != Unchanged {
				break
			}
			time.Sleep(f.PollInterval)
		}
		if f.checkShouldStop() {
			break
		}
		if errCFM != nil {
			f.fd.Close()
			f.fd = nil
			continue
		}
		switch modifiedType {
		case Appended, Edited:
			if !f.IsReadFromBeginningOnModified {
				continue
			}
			beginT := time.Now()
			_, err := f.fd.Seek(0, io.SeekStart)
			f.Log.Printf("fd_Seek %v, dur: %v", f.FileName, time.Since(beginT))
			if err != nil {
				f.Log.Printf("error fd_Seek %v: %v", f.FileName, err)
				f.fd.Close()
				f.fd = nil
				continue
			} else {
				continue // next loop read current from begin
			}
		case Created:
			f.fd.Close()
			f.fd = nil
			continue
		}
	}
}

type ModifiedType string

// ModifiedType enum
const (
	Unchanged ModifiedType = "UNCHANGED"
	Created   ModifiedType = "CREATED"
	Appended  ModifiedType = "APPENDED"
	Edited    ModifiedType = "EDITED"
)

// checkFileModified modifies flr_lastFileInfo
func (f *Follower) checkFileModified() (ModifiedType, error) {
	beginT := time.Now()
	defer func() {
		f.Log.Printf("checkFileModified %v dur: %v", f.FileName, time.Since(beginT))
	}()
	fi, err := os.Stat(f.FilePath)
	if err != nil {
		if os.IsNotExist(err) || (runtime.GOOS == "windows" && os.IsPermission(err)) {
			return Created, nil
		}
		return Created, err
	}
	lastFI := f.lastFileInfo
	f.lastFileInfo = fi

	if checkNilInterface(lastFI) {
		return Created, nil
	}

	// file got moved or renamed
	if !os.SameFile(lastFI, fi) {
		return Created, nil
	}

	// file got modified
	if lastFI.ModTime() != fi.ModTime() {
		if lastFI.Size() >= fi.Size() {
			return Edited, nil
		}
		return Appended, nil
	}

	// file has no changes
	return Unchanged, nil
}

type Logger interface {
	Printf(format string, args ...interface{})
}

func LoggerStd() Logger {
	return log.New(os.Stdout, "", log.Lshortfile|log.Lmicroseconds)
}

type LoggerNil struct{}

func (l LoggerNil) Printf(format string, args ...interface{}) {}

// checkNilInterface returns true even if arg x is a typed nil,
// we need this func because an interface variable is nil only if both the type
// and value are nil, so `x == nil` returns false if x is a typed nil
func checkNilInterface(x interface{}) (result bool) {
	defer func() {
		r := recover()
		if r != nil {
			result = false
		}
	}()
	if x == nil {
		// only untyped nil return here
		return true
	}
	if reflect.ValueOf(x).IsNil() {
		// panic if x is not chan, func, interface, map, pointer, or slice
		return true
	}
	return false
}
