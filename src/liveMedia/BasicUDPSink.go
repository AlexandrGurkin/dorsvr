package liveMedia

import (
	. "groupsock"
	"utils"
)

type BasicUDPSink struct {
	MediaSink
	gs             *GroupSock
	maxPayloadSize uint
	outputBuffer   []byte
	nextSendTime   utils.Timeval
}

func NewBasicUDPSink(gs *GroupSock) *BasicUDPSink {
	udpSink := new(BasicUDPSink)
	udpSink.maxPayloadSize = 1450
	udpSink.outputBuffer = make([]byte, udpSink.maxPayloadSize)
	udpSink.gs = gs
	return udpSink
}

func (sink *BasicUDPSink) continuePlaying() {
}
