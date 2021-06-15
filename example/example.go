package main

import (
	"log"
	"time"

	"github.com/daominah/filefollow"
)

func main() {
	filePath := `/home/tungdt/Desktop/a.txt`

	f := filefollow.NewFollower(filePath)
	go func() {
		time.Sleep(5 * time.Second)
		f.Stop()
	}()

TheForLoop:
	for {
		select {
		case bs := <-f.OutputChan:
			log.Printf("at %v newly appended data: %s",
				time.Now().Format(time.RFC3339Nano), bs)
		case <-f.StopDoneChan:
			break TheForLoop
		}
	}
	log.Printf("done")
}
