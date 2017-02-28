package livemedia

import (
	"fmt"

	gs "github.com/djwackey/dorsvr/groupsock"
)

type BasicUDPSource struct {
	FramedSource
	inputSocket        *gs.GroupSock
	haveStartedReading bool
}

func NewBasicUDPSource(inputSocket *gs.GroupSock) *BasicUDPSource {
	source := new(BasicUDPSource)
	source.inputSocket = inputSocket
	source.InitFramedSource(source)
	return source
}

func (s *BasicUDPSource) doGetNextFrame() {
	go s.incomingPacketHandler()
}

func (s *BasicUDPSource) doStopGettingFrames() {
	s.haveStartedReading = false
}

func (s *BasicUDPSource) incomingPacketHandler() {
	for {
		numBytes, err := s.inputSocket.HandleRead(s.buffTo)
		if err != nil {
			fmt.Println("Failed to read from input socket.", err.Error())
			break
		}

		s.frameSize = uint(numBytes)

		s.afterGetting()
	}
}
