package main

import (
	"context"
	"log"
	"time"

	"github.com/daominah/filefollow"
)

func main() {
	filePath := `/home/tungdt/Desktop/a.txt`

	// manually config Follower, can use NewFollower instead

	ctxStop, cclStop := context.WithCancel(context.Background())
	f := &filefollow.Follower{
		FilePath:   filePath,
		OutputChan: make(chan []byte),

		PollInterval:                  100 * time.Millisecond,
		Log:                           filefollow.LoggerStd(),
		IsReadFromBeginningOnModified: false,
		IsSkipCheckModified:           false,

		Stop:         cclStop,
		StopDoneChan: ctxStop.Done(),
	}
	go f.Follow()

	go func() {
		time.Sleep(5 * time.Second)
		f.Stop()
	}()

	log.SetFlags(log.Lshortfile | log.Lmicroseconds)
TheForLoop:
	for {
		select {
		case bs := <-f.OutputChan:
			log.Printf("newly appended data: %s", bs)
		case <-f.StopDoneChan:
			break TheForLoop
		}
	}
	log.Printf("done")
}
