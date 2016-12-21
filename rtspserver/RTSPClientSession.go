package rtspserver

import (
	"fmt"
	"net"
	"strings"
	"time"
)

type RTSPClientSession struct {
	isMulticast          bool
	isTimerRunning       bool
	streamAfterSETUP     bool
	numStreamStates      int
	TCPStreamIDCount     uint
	ourSessionID         uint
	streamStates         *streamState
	rtspServer           *RTSPServer
	rtspClientConn       *RTSPClientConnection
	serverMediaSession   *ServerMediaSession
	livenessTimeoutTimer *time.Timer
}

type streamState struct {
	subsession  IServerMediaSubSession
	streamToken *StreamState
}

func NewRTSPClientSession(rtspClientConn *RTSPClientConnection, sessionID uint) *RTSPClientSession {
	rtspClientSession := new(RTSPClientSession)
	rtspClientSession.ourSessionID = sessionID
	rtspClientSession.rtspClientConn = rtspClientConn
	rtspClientSession.rtspServer = rtspClientConn.GetRTSPServer()
	rtspClientSession.noteLiveness()
	return rtspClientSession
}

func (this *RTSPClientSession) HandleCommandSetup(urlPreSuffix, urlSuffix, reqStr string) {
	streamName, trackId := urlPreSuffix, urlSuffix

	sms := this.rtspServer.LookupServerMediaSession(streamName)
	if sms == nil {
		if this.serverMediaSession == nil {
			this.rtspClientConn.handleCommandNotFound()
		} else {
			this.rtspClientConn.handleCommandBad()
		}
		return
	}

	if this.serverMediaSession == nil {
		this.serverMediaSession = sms
	} else if sms != this.serverMediaSession {
		this.rtspClientConn.handleCommandBad()
		return
	}

	if this.streamStates == nil {
		this.numStreamStates = this.serverMediaSession.subsessionCounter

		this.streamStates = new(streamState)
		for i := 0; i < this.numStreamStates; i++ {
			this.streamStates.subsession = this.serverMediaSession.subSessions[i]
		}
	}

	// Look up information for the specified subsession (track):
	//var streamNum int
	var subsession IServerMediaSubSession
	if trackId != "" {
		for streamNum := 0; streamNum < this.numStreamStates; streamNum++ {
			subsession = this.streamStates.subsession
			fmt.Println("Look up", subsession)
			if subsession != nil && strings.EqualFold(trackId, subsession.TrackID()) {
				break
			}
		}
	} else {
		if this.numStreamStates != 1 && this.streamStates == nil {
			this.rtspClientConn.handleCommandBad()
			return
		}
		subsession = this.streamStates.subsession
	}

	// Look for a "Transport:" header in the request string, to extract client parameters:
	transportHeader := parseTransportHeader(reqStr)
	rtpChannelID := transportHeader.rtpChannelID
	rtcpChannelID := transportHeader.rtcpChannelID
	streamingMode := transportHeader.streamingMode
	clientRTPPort := transportHeader.clientRTPPortNum
	clientRTCPPort := transportHeader.clientRTCPPortNum
	streamingModeStr := transportHeader.streamingModeStr

	if streamingMode == RTP_TCP && rtpChannelID == 0xFF {
		rtpChannelID = this.TCPStreamIDCount
		rtcpChannelID = this.TCPStreamIDCount + 1
	}
	if streamingMode == RTP_TCP {
		rtcpChannelID = this.TCPStreamIDCount + 2
	}

	_, sawRangeHeader := parseRangeHeader(reqStr)
	if sawRangeHeader {
		this.streamAfterSETUP = true
	} else if parsePlayNowHeader(reqStr) {
		this.streamAfterSETUP = true
	} else {
		this.streamAfterSETUP = false
	}

	sourceAddrStr := this.rtspClientConn.localAddr
	destAddrStr := this.rtspClientConn.remoteAddr

	var tcpSocketNum net.Conn
	if streamingMode == RTP_TCP {
		tcpSocketNum = this.rtspClientConn.clientOutputSocket
	}

	streamParameter := subsession.getStreamParameters(tcpSocketNum,
		destAddrStr,
		string(this.ourSessionID),
		clientRTPPort,
		clientRTCPPort,
		rtpChannelID,
		rtcpChannelID)
	serverRTPPort := streamParameter.serverRTPPort
	serverRTCPPort := streamParameter.serverRTCPPort

	//fmt.Println("RTSPClientSession::getStreamParameters", streamParameter, transportHeader)

	this.streamStates.streamToken = streamParameter.streamToken

	if this.isMulticast {
		switch streamingMode {
		case RTP_UDP:
			this.rtspClientConn.responseBuffer = fmt.Sprintf("RTSP/1.0 200 OK\r\n"+
				"CSeq: %s\r\n"+
				"%s"+
				"Transport: RTP/AVP;multicast;destination=%s;source=%s;port=%d-%d;ttl=%d\r\n"+
				"Session: %08X\r\n\r\n", this.rtspClientConn.currentCSeq,
				DateHeader(),
				destAddrStr,
				sourceAddrStr,
				serverRTPPort,
				serverRTCPPort,
				transportHeader.destinationTTL,
				this.ourSessionID)
		case RTP_TCP:
			// multicast streams can't be sent via TCP
			this.rtspClientConn.HandleCommandUnsupportedTransport()
		case RAW_UDP:
			this.rtspClientConn.responseBuffer = fmt.Sprintf("RTSP/1.0 200 OK\r\n"+
				"CSeq: %s\r\n"+
				"%s"+
				"Transport: %s;multicast;destination=%s;source=%s;port=%d;ttl=%d\r\n"+
				"Session: %08X\r\n\r\n", this.rtspClientConn.currentCSeq,
				DateHeader(),
				destAddrStr,
				sourceAddrStr,
				serverRTPPort,
				serverRTCPPort,
				transportHeader.destinationTTL,
				this.ourSessionID)
		default:
		}
	} else {
		switch streamingMode {
		case RTP_UDP:
			this.rtspClientConn.responseBuffer = fmt.Sprintf("RTSP/1.0 200 OK\r\n"+
				"CSeq: %s\r\n"+
				"%s"+
				"Transport: RTP/AVP;unicast;destination=%s;source=%s;client_port=%d-%d;server_port=%d-%d\r\n"+
				"Session: %08X\r\n\r\n", this.rtspClientConn.currentCSeq,
				DateHeader(),
				destAddrStr,
				sourceAddrStr,
				clientRTPPort,
				clientRTCPPort,
				serverRTPPort,
				serverRTCPPort,
				this.ourSessionID)
		case RTP_TCP:
			this.rtspClientConn.responseBuffer = fmt.Sprintf("RTSP/1.0 200 OK\r\n"+
				"CSeq: %s\r\n"+
				"%s"+
				"Transport: RTP/AVP/TCP;unicast;destination=%s;source=%s;interleaved=%d-%d\r\n"+
				"Session: %08X\r\n\r\n", this.rtspClientConn.currentCSeq,
				DateHeader(),
				destAddrStr,
				sourceAddrStr,
				rtpChannelID,
				rtcpChannelID,
				this.ourSessionID)
		case RAW_UDP:
			this.rtspClientConn.responseBuffer = fmt.Sprintf("RTSP/1.0 200 OK\r\n"+
				"CSeq: %s\r\n"+
				"%s"+
				"Transport: %s;unicast;destination=%s;source=%s;client_port=%d;server_port=%d\r\n"+
				"Session: %08X\r\n\r\n", this.rtspClientConn.currentCSeq,
				DateHeader(),
				streamingModeStr,
				destAddrStr,
				sourceAddrStr,
				clientRTPPort,
				serverRTPPort,
				this.ourSessionID)
		}
	}
}

func (this *RTSPClientSession) handleCommandWithinSession(cmdName, urlPreSuffix, urlSuffix, fullRequestStr string) {
	fmt.Println("RTSPClientSession::HandleCommandWithinSession", urlPreSuffix, urlSuffix, this.serverMediaSession.StreamName())

	this.noteLiveness()

	var subsession IServerMediaSubSession
	if this.serverMediaSession == nil { // There wasn't a previous SETUP!
		this.rtspClientConn.handleCommandNotSupported()
		return
	} else if urlSuffix != "" && strings.EqualFold(this.serverMediaSession.StreamName(), urlPreSuffix) {
		// Non-aggregated operation.
		// Look up the media subsession whose track id is "urlSuffix":
		for i := 0; i < this.serverMediaSession.subsessionCounter; i++ {
			subsession = this.serverMediaSession.subSessions[i]

			if strings.EqualFold(subsession.TrackID(), urlSuffix) {
				break
			}
		}

		if subsession == nil { // no such track!
			this.rtspClientConn.handleCommandNotFound()
			return
		}
	} else if strings.EqualFold(this.serverMediaSession.StreamName(), urlSuffix) ||
		urlSuffix == "" && strings.EqualFold(this.serverMediaSession.StreamName(), urlPreSuffix) {
		// Aggregated operation
		subsession = nil
	} else if urlPreSuffix != "" && urlSuffix != "" {
		// Aggregated operation, if <urlPreSuffix>/<urlSuffix> is the session (stream) name:
		if strings.EqualFold(this.serverMediaSession.StreamName(), urlPreSuffix) &&
			this.serverMediaSession.StreamName() == "" &&
			strings.EqualFold(this.serverMediaSession.StreamName(), urlSuffix) {
			subsession = nil
		} else {
			this.rtspClientConn.handleCommandNotFound()
			return
		}
	} else { // the request doesn't match a known stream and/or track at all!
		this.rtspClientConn.handleCommandNotFound()
		return
	}

	switch cmdName {
	case "TEARDOWN":
		this.handleCommandTearDown()
	case "PLAY":
		this.handleCommandPlay(subsession, fullRequestStr)
	case "PAUSE":
		this.handleCommandPause()
	case "GET_PARAMETER":
		this.handleCommandGetParameter()
	case "SET_PARAMETER":
		this.handleCommandSetParameter()
	}
}

func (this *RTSPClientSession) handleCommandPlay(subsession IServerMediaSubSession, fullRequestStr string) {
	rtspURL := this.rtspServer.RtspURL(this.serverMediaSession.StreamName())

	// Parse the client's "Scale:" header, if any:
	scale, sawScaleHeader := parseScaleHeader(fullRequestStr)

	// Try to set the stream's scale factor to this value:
	if subsession == nil {
		scale = this.serverMediaSession.testScaleFactor()
	} else {
		scale = subsession.testScaleFactor(scale)
	}

	var buf string
	if sawScaleHeader {
		buf = fmt.Sprintf("Scale: %f\r\n", scale)
	}
	scaleHeaderStr := buf

	buf = ""
	var rangeStart, rangeEnd, duration float32
	var absStartTime, absEndTime string

	rangeHeader, sawRangeHeader := parseRangeHeader(fullRequestStr)
	if sawRangeHeader && rangeHeader.absStartTime == "" {
		if subsession == nil {
			duration = this.serverMediaSession.Duration()
		} else {
			//duration = subsession.Duration()
		}
		if duration < 0 {
			duration = -duration
		}

		rangeStart = rangeHeader.rangeStart
		rangeEnd = rangeHeader.rangeEnd
		absStartTime = rangeHeader.absStartTime
		absEndTime = rangeHeader.absEndTime

		if rangeStart < 0 {
			rangeStart = 0
		} else if rangeStart > duration {
			rangeStart = duration
		}
		if rangeEnd < 0 {
			rangeEnd = 0
		} else if rangeEnd > duration {
			rangeEnd = duration
		}

		if (scale > 0.0 && rangeStart > rangeEnd && rangeEnd > 0.0) || (scale < 0.0 && rangeStart < rangeEnd) {
			// "rangeStart" and "rangeEnd" were the wrong way around; swap them:
			rangeStart, rangeEnd = rangeEnd, rangeStart
		}

		// We're seeking by 'absolute' time:
		if absEndTime == "" {
			buf = fmt.Sprintf("Range: clock=%s-\r\n", absStartTime)
		} else {
			buf = fmt.Sprintf("Range: clock=%s-%s\r\n", absStartTime, absEndTime)
		}
	} else {
		// We're seeking by relative (NPT) time:
		if rangeEnd == 0.0 && scale >= 0.0 {
			buf = fmt.Sprintf("Range: npt=%.3f-\r\n", rangeStart)
		} else {
			buf = fmt.Sprintf("Range: npt=%.3f-%.3f\r\n", rangeStart, rangeEnd)
		}
	}

	for i := 0; i < this.numStreamStates; i++ {
		if subsession == nil || this.numStreamStates == 1 {
			if sawScaleHeader {
				if this.streamStates.subsession != nil {
					//this.streamStates.subsession.setStreamScale(this.ourSessionId, this.streamStates.streamToken, scale)
				}
			}
			if sawRangeHeader {
				// Special case handling for seeking by 'absolute' time:
				if absStartTime != "" {
					if this.streamStates.subsession != nil {
						//this.streamStates.subsession.seekStream(this.ourSessionId, this.streamStates.streamToken, absStartTime, absEndTime)
					}
				} else { // Seeking by relative (NPT) time:
					var streamDuration float32 = 0.0                   // by default; means: stream until the end of the media
					if rangeEnd > 0.0 && (rangeEnd+0.001) < duration { // the 0.001 is because we limited the values to 3 decimal places
						// We want the stream to end early.  Set the duration we want:
						streamDuration = rangeEnd - rangeStart
						if streamDuration < 0.0 {
							streamDuration = -streamDuration // should happen only if scale < 0.0
						}
					}
					if this.streamStates.subsession != nil {
						//var numBytes int
						//this.streamStates.subsession.seekStream(this.ourSessionId, this.streamStates.streamToken, rangeStart, streamDuration, numBytes)
					}
				}
			}
		}
	}

	rangeHeaderStr := buf

	rtpSeqNum, rtpTimestamp := this.streamStates.subsession.startStream(this.ourSessionID, this.streamStates.streamToken)
	urlSuffix := this.streamStates.subsession.TrackID()

	// Create a "RTP-INFO" line. It will get filled in from each subsession's state:
	rtpInfoFmt := "RTP-INFO:" +
		"%s" +
		"url=%s/%s" +
		";seq=%d" +
		";rtptime=%d"

	rtpInfo := fmt.Sprintf(rtpInfoFmt, "0", rtspURL, urlSuffix, rtpSeqNum, rtpTimestamp)

	// Fill in the response:
	this.rtspClientConn.responseBuffer = fmt.Sprintf("RTSP/1.0 200 OK\r\n"+
		"CSeq: %s\r\n"+
		"%s"+
		"%s"+
		"%s"+
		"Session: %08X\r\n"+
		"%s\r\n", this.rtspClientConn.currentCSeq,
		DateHeader(),
		scaleHeaderStr,
		rangeHeaderStr,
		this.ourSessionID,
		rtpInfo)
}

func (this *RTSPClientSession) handleCommandPause() {
	this.streamStates.subsession.pauseStream(this.streamStates.streamToken)
	/*
		for i := 0; i < this.numStreamStates; i++ {
			this.streamStates[i].subsession.pauseStream()
		}*/

	this.rtspClientConn.setRTSPResponseWithSessionID("200 OK", this.ourSessionID)
}

func (this *RTSPClientSession) handleCommandGetParameter() {
	this.rtspClientConn.setRTSPResponseWithSessionID("200 OK", this.ourSessionID)
}

func (this *RTSPClientSession) handleCommandSetParameter() {
	this.rtspClientConn.setRTSPResponseWithSessionID("200 OK", this.ourSessionID)
}

func (this *RTSPClientSession) handleCommandTearDown() {
	this.streamStates.subsession.deleteStream(this.streamStates.streamToken)
	/*
		for i := 0; i < this.numStreamStates; i++ {
			this.streamStates[i].subsession.deleteStream()
		}*/
}

func (this *RTSPClientSession) noteLiveness() {
	if !this.isTimerRunning {
		go this.livenessTimeoutTask(time.Second * this.rtspServer.reclamationTestSeconds)
		this.isTimerRunning = true
	} else {
		//fmt.Println("noteLiveness", this.livenessTimeoutTimer)
		this.livenessTimeoutTimer.Reset(time.Second * this.rtspServer.reclamationTestSeconds)
	}
}

func (this *RTSPClientSession) livenessTimeoutTask(d time.Duration) {
	this.livenessTimeoutTimer = time.NewTimer(d)

	for {
		select {
		case <-this.livenessTimeoutTimer.C:
			fmt.Println("livenessTimeoutTask")
		}
	}
}
