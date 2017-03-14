package livemedia

import gs "github.com/djwackey/dorsvr/groupsock"

type RTPSource struct {
	FramedSource
	ssrc                   uint
	lastReceivedSSRC       uint
	rtpPayloadFormat       uint
	timestampFrequency     uint
	curPacketRTPSeqNum     uint32
	curPacketRTPTimestamp  uint32
	curPacketSyncUsingRTCP bool
	curPacketMarkerBit     bool
	receptionStatsDB       *RTPReceptionStatsDB
	rtpInterface           *RTPInterface
}

func NewRTPSource() *RTPSource {
	source := new(RTPSource)
	return source
}

func (s *RTPSource) InitRTPSouce(isource IFramedSource, RTPgs *gs.GroupSock,
	rtpPayloadFormat, rtpTimestampFrequency uint) {
	s.rtpInterface = newRTPInterface(s, RTPgs)
	s.lastReceivedSSRC = 0
	s.rtpPayloadFormat = rtpPayloadFormat
	s.timestampFrequency = rtpTimestampFrequency
	s.ssrc = uint(gs.OurRandom32())
	s.curPacketSyncUsingRTCP = false
	s.receptionStatsDB = NewRTPReceptionStatsDB()
	s.InitFramedSource(isource)
}

func (s *RTPSource) SetStreamSocket() {
}
