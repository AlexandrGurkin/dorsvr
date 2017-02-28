package livemedia

import gs "github.com/djwackey/dorsvr/groupsock"

type VideoRTPSink struct {
	MultiFramedRTPSink
}

func (s *VideoRTPSink) InitVideoRTPSink(rtpSink IRTPSink, rtpGroupSock *gs.GroupSock,
	rtpPayloadType, rtpTimestampFrequency uint, rtpPayloadFormatName string) {
	s.InitMultiFramedRTPSink(rtpSink, rtpGroupSock, rtpPayloadType,
		rtpTimestampFrequency, rtpPayloadFormatName)
}

func (s *VideoRTPSink) SdpMediaType() string {
	return "video"
}
