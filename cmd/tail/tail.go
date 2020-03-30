package main

import (
	"github.com/daominah/filefollow"
	"github.com/daominah/gomicrokit/log"
)

func main() {
	filePath := `Z:\BACKUP30\SECURITY.DAT`
	isWriterTruncate, isNotCheckModified := false, false
	f := filefollow.NewFollower(filePath, isWriterTruncate, isNotCheckModified)
TheForLoop:
	for {
		select {
		case bs := <-f.OutputChan:
			log.Infof("receive: %s", bs)
		case <-f.StopDoneChan:
			break TheForLoop
		}
	}
}
