package filefollow

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"
)

func TestFollower(t *testing.T) {
	filePath := "/tmp/testff" + fmt.Sprintf("%v", time.Now().UnixNano())
	t.Logf("file path: %v", filePath)

	// periodically append to test file once per second
	go func() {
		cmd := exec.Command("/bin/bash", "-c",
			fmt.Sprintf(`for i in $(seq 60); do echo "line $i $(date --iso=ns)" >> %v; sleep 1; done`, filePath))
		_, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
	}()

	// common use case: follow the file with default config

	f := NewFollower(filePath)
	go func() {
		time.Sleep(5 * time.Second)
		f.Stop()
	}()
TheForLoop:
	for {
		select {
		case bs := <-f.OutputChan:
			t.Logf("at %v newly appended data: %s",
				time.Now().Format(time.RFC3339Nano), bs)
		case <-f.StopDoneChan:
			break TheForLoop
		}
	}
	t.Logf("done")

	// customized config use case 1

	ctxStop1, cclStop1 := context.WithCancel(context.Background())
	f1 := &Follower{
		FilePath:   filePath,
		OutputChan: make(chan []byte),

		PollInterval:                  2100 * time.Millisecond,
		Log:                           LoggerNil{},
		IsReadFromBeginningOnModified: true,
		IsSkipCheckModified:           false,

		Stop:         cclStop1,
		StopDoneChan: ctxStop1.Done(),
	}
	go f1.Follow()

	go func() {
		time.Sleep(5 * time.Second)
		f1.Stop()
	}()

TheForLoop1:
	for {
		select {
		case bs := <-f1.OutputChan:
			t.Logf("at %v newly appended data: %s",
				time.Now().Format(time.RFC3339Nano), bs)
		case <-f1.StopDoneChan:
			break TheForLoop1
		}
	}
	t.Logf("done1")

	// customized config use case 2

	ctxStop2, cclStop2 := context.WithCancel(context.Background())
	f2 := &Follower{
		FilePath:   filePath,
		OutputChan: make(chan []byte),

		PollInterval:                  1100 * time.Millisecond,
		Log:                           LoggerNil{},
		IsReadFromBeginningOnModified: false,
		IsSkipCheckModified:           true,

		Stop:         cclStop2,
		StopDoneChan: ctxStop2.Done(),
	}
	go f2.Follow()

	go func() {
		time.Sleep(5 * time.Second)
		f2.Stop()
	}()

TheForLoop2:
	for {
		select {
		case bs := <-f2.OutputChan:
			t.Logf("at %v newly appended data: %s",
				time.Now().Format(time.RFC3339Nano), bs)
		case <-f2.StopDoneChan:
			break TheForLoop2
		}
	}
	t.Logf("done2")
}
