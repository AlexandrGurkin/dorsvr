package livemedia

import (
	"fmt"
	gs "github.com/djwackey/dorsvr/groupsock"
	"net"
	"os"
	"strconv"
	"strings"
)

//////// MediaSession ////////
type MediaSession struct {
	cname                  string
	sessionName            string
	controlPath            string
	absStartTime           string
	absEndTime             string
	mediaSessionType       string
	sessionDescription     string
	connectionEndpointName string
	subSessionNum          int
	subSessionIndex        int
	maxPlayStartTime       float64
	maxPlayEndTime         float64
	scale                  float32
	mediaSubSessions       []*MediaSubSession
}

func NewMediaSession(sdpDesc string) *MediaSession {
	mediaSession := new(MediaSession)
	mediaSession.mediaSubSessions = make([]*MediaSubSession, 1024)
	mediaSession.cname, _ = os.Hostname()
	mediaSession.scale = 1.0
	if !mediaSession.InitWithSDP(sdpDesc) {
		return nil
	}
	return mediaSession
}

func (session *MediaSession) InitWithSDP(sdpLine string) bool {
	if sdpLine == "" {
		return false
	}

	// Begin by processing All SDP lines until we see the first "m="
	var result bool
	var nextSDPLine, thisSDPLine string
	for {
		nextSDPLine, thisSDPLine, result = session.parseSDPLine(sdpLine)
		if !result {
			return false
		}

		sdpLine = nextSDPLine

		if thisSDPLine[0] == 'm' {
			break
		}

		// there is no m= lines at all
		if sdpLine == "" {
			break
		}

		// Check for various special SDP lines that we understand:
		if session.parseSDPLine_s(thisSDPLine) {
			continue
		}
		if session.parseSDPLine_i(thisSDPLine) {
			continue
		}
		if session.parseSDPLine_c(thisSDPLine) {
			continue
		}
		if session.parseSDPAttributeControl(thisSDPLine) {
			continue
		}
		if session.parseSDPAttributeRange(thisSDPLine) {
			continue
		}
		if session.parseSDPAttributeType(thisSDPLine) {
			continue
		}
		if session.parseSDPAttributeSourceFilter(thisSDPLine) {
			continue
		}
	}

	var payloadFormat uint
	var mediumName, protocolName string
	for {
		subsession := NewMediaSubSession(session)
		if subsession == nil {
			fmt.Println("Unable to create new MediaSubsession")
			return false
		}

		num1, _ := fmt.Sscanf(thisSDPLine, "m=%s %d RTP/AVP %d", &mediumName,
			&subsession.clientPortNum, &payloadFormat)
		num2, _ := fmt.Sscanf(thisSDPLine, "m=%s %d/%*d RTP/AVP %d", &mediumName,
			&subsession.clientPortNum, &payloadFormat)
		num3, _ := fmt.Sscanf(thisSDPLine, "m=%s %d UDP %d", &mediumName,
			&subsession.clientPortNum, &payloadFormat)
		num4, _ := fmt.Sscanf(thisSDPLine, "m=%s %d udp %d", &mediumName,
			&subsession.clientPortNum, &payloadFormat)
		num5, _ := fmt.Sscanf(thisSDPLine, "m=%s %d RAW/RAW/UDP %d", &mediumName,
			&subsession.clientPortNum, &payloadFormat)

		if (num1 == 3 || num2 == 3) && int(payloadFormat) <= 127 {
			protocolName = "RTP"
		} else if (num3 == 3 || num4 == 3 || num5 == 3) && int(payloadFormat) <= 127 {
			// This is a RAW UDP source
			protocolName = "UDP"
		} else {
		}

		// Insert this subsession at the end of the list:
		//this.mediaSubSessions = append(this.mediaSubSessions, subsession)
		session.mediaSubSessions[session.subSessionNum] = subsession
		session.subSessionNum++

		subsession.serverPortNum = subsession.clientPortNum
		subsession.savedSDPLines = sdpLine
		subsession.mediumName = mediumName
		subsession.protocolName = protocolName
		subsession.rtpPayloadFormat = payloadFormat

		// Process the following SDP lines, up until the next "m=":
		for {
			sdpLine = nextSDPLine
			if sdpLine == "" {
				//fmt.Println("we've reached the end")
				break // we've reached the end
			}

			nextSDPLine, thisSDPLine, result = session.parseSDPLine(sdpLine)
			if !result {
				return false
			}

			if thisSDPLine[0] == 'm' {
				break // we've reached the next subsession
			}

			// Check for various special SDP lines that we understand:
			if subsession.parseSDPLine_c(thisSDPLine) {
				continue
			}
			if subsession.parseSDPLine_b(thisSDPLine) {
				continue
			}
			if subsession.parseSDPAttributeRtpmap(thisSDPLine) {
				continue
			}
			if subsession.parseSDPAttributeControl(thisSDPLine) {
				continue
			}
			if subsession.parseSDPAttributeRange(thisSDPLine) {
				continue
			}
			if subsession.parseSDPAttributeFmtp(thisSDPLine) {
				continue
			}
			if subsession.parseSDPAttributeSourceFilter(thisSDPLine) {
				continue
			}
			if subsession.parseSDPAttributeXDimensions(thisSDPLine) {
				continue
			}
			if subsession.parseSDPAttributeFrameRate(thisSDPLine) {
				continue
			}
			// (Later, check for malformed lines, and other valid SDP lines#####)
		}

		if subsession.codecName == "" {
			subsession.codecName,
				subsession.rtpTimestampFrequency,
				subsession.numChannels = session.lookupPayloadFormat(subsession.rtpPayloadFormat)
		}

		// If we don't yet know this subsession's RTP timestamp frequency
		// (because it uses a dynamic payload type and the corresponding
		// SDP "rtpmap" attribute erroneously didn't specify it),
		// then guess it now:
		if subsession.rtpTimestampFrequency == 0 {
			subsession.rtpTimestampFrequency = session.guessRTPTimestampFrequency(subsession.mediumName,
				subsession.codecName)
		}
		break
	}
	return true
}

func (session *MediaSession) Scale() float32 {
	return session.scale
}

func (session *MediaSession) ControlPath() string {
	return session.controlPath
}

func (session *MediaSession) AbsStartTime() string {
	return session.absStartTime
}

func (session *MediaSession) AbsEndTime() string {
	return session.absEndTime
}

func (session *MediaSession) HasSubSessions() bool {
	return len(session.mediaSubSessions) > 0
}

func (this *MediaSession) SubSession() *MediaSubSession {
	this.subSessionIndex++
	return this.mediaSubSessions[this.subSessionIndex-1]
}

func (this *MediaSession) parseSDPLine(inputLine string) (nextLine, thisLine string, result bool) {
	inputLen := len(inputLine)

	// Begin by finding the start of the next line (if any):
	for i := 0; i < inputLen; i++ {
		if inputLine[i] == '\r' || inputLine[i] == '\n' {
			for i += 1; i < inputLen && (inputLine[i] == '\r' || inputLine[i] == '\n'); i++ {
			}
			nextLine = inputLine[i:]
			thisLine = inputLine[:i-2]
			break
		}
	}

	if len(thisLine) < 2 || thisLine[1] != '=' || thisLine[0] < 'a' || thisLine[0] > 'z' {
		fmt.Println("Invalid SDP line:", thisLine, nextLine)
	} else {
		result = true
	}
	return
}

func parseCLine(sdpLine string) string {
	var result string
	fmt.Sscanf(sdpLine, "c=IN IP4 %s", &result)
	return result
}

func (session *MediaSession) parseSDPLine_s(sdpLine string) bool {
	// Check for "s=<session name>" line
	var parseSuccess bool

	if sdpLine[0] == 's' && sdpLine[1] == '=' {
		session.sessionName = sdpLine[2:]
		parseSuccess = true
	}

	return parseSuccess
}

func (this *MediaSession) parseSDPLine_i(sdpLine string) bool {
	// Check for "i=<session description>" line
	var parseSuccess bool

	if sdpLine[0] == 'i' && sdpLine[1] == '=' {
		this.sessionDescription = sdpLine[2:]
		parseSuccess = true
	}

	return parseSuccess
}

func (this *MediaSession) parseSDPLine_c(sdpLine string) bool {
	// Check for "c=IN IP4 <connection-endpoint>"
	// or "c=IN IP4 <connection-endpoint>/<ttl+numAddresses>"
	// (Later, do something with <ttl+numAddresses> also #####)
	connectionEndpointName := parseCLine(sdpLine)
	if connectionEndpointName != "" {
		this.connectionEndpointName = connectionEndpointName
		return true
	}

	return false
}

func (this *MediaSession) parseSDPAttributeType(sdpLine string) bool {
	// Check for a "a=type:broadcast|meeting|moderated|test|H.332|recvonly" line:
	var parseSuccess bool

	var buffer string
	if num, _ := fmt.Sscanf(sdpLine, "a=type: %[^ ]", &buffer); num == 1 {
		this.mediaSessionType = buffer
		parseSuccess = true
	}

	return parseSuccess
}

func (this *MediaSession) parseSDPAttributeControl(sdpLine string) bool {
	// Check for a "a=control:<control-path>" line:
	var parseSuccess bool

	ok := strings.HasPrefix(sdpLine, "a=control:")
	if ok {
		this.controlPath = sdpLine[10:]
		parseSuccess = true
	}

	return parseSuccess
}

func parseRangeAttribute(sdpLine, method string) (string, string, bool) {
	if method == "npt" {
		var startTime, endTime string
		num, _ := fmt.Sscanf(sdpLine, "a=range: npt = %f - %f", &startTime, &endTime)
		return startTime, endTime, (num == 2)
	} else if method == "clock" {
		var as, ae, absStartTime, absEndTime string
		num, _ := fmt.Sscanf(sdpLine, "a=range: clock = %[^-\r\n]-%[^\r\n]", &as, &ae)
		if num == 2 {
			absStartTime = as
			absEndTime = ae
		} else if num == 1 {
			absStartTime = as
		}

		return absStartTime, absEndTime, (num == 2) || (num == 1)
	}

	return "", "", false
}

func (this *MediaSession) parseSDPAttributeRange(sdpLine string) bool {
	// Check for a "a=range:npt=<startTime>-<endTime>" line:
	// (Later handle other kinds of "a=range" attributes also???#####)
	var parseSuccess bool

	startTime, endTime, ok := parseRangeAttribute(sdpLine, "npt")
	if ok {
		parseSuccess = true

		playStartTime, _ := strconv.ParseFloat(startTime, 32)
		playEndTime, _ := strconv.ParseFloat(endTime, 32)

		if playStartTime > this.maxPlayStartTime {
			this.maxPlayStartTime = playStartTime
		}
		if playEndTime > this.maxPlayEndTime {
			this.maxPlayEndTime = playEndTime
		}
	} else if this.absStartTime, this.absEndTime, ok = parseRangeAttribute(sdpLine, "clock"); ok {
		parseSuccess = true
	}

	return parseSuccess
}

func parseSourceFilterAttribute(sdpLine string) bool {
	// Check for a "a=source-filter:incl IN IP4 <something> <source>" line.
	// Note: At present, we don't check that <something> really matches
	// one of our multicast addresses.  We also don't support more than
	// one <source> #####
	var sourceName string
	num, _ := fmt.Sscanf(sdpLine, "a=source-filter: incl IN IP4 %*s %s", &sourceName)
	return (num == 1)
}

func (this *MediaSession) parseSDPAttributeSourceFilter(sdpLine string) bool {
	return parseSourceFilterAttribute(sdpLine)
}

func (this *MediaSession) lookupPayloadFormat(rtpPayloadType uint) (string, uint, uint) {
	// Look up the codec name and timestamp frequency for known (static)
	// RTP payload formats.
	var codecName string
	var freq, nCh uint
	switch rtpPayloadType {
	case 0:
		codecName = "PCMU"
		freq = 8000
		nCh = 1
	case 2:
		codecName = "G726-32"
		freq = 8000
		nCh = 1
	case 3:
		codecName = "GSM"
		freq = 8000
		nCh = 1
	case 4:
		codecName = "G723"
		freq = 8000
		nCh = 1
	case 5:
		codecName = "DVI4"
		freq = 8000
		nCh = 1
	case 6:
		codecName = "DVI4"
		freq = 16000
		nCh = 1
	case 7:
		codecName = "LPC"
		freq = 8000
		nCh = 1
	case 8:
		codecName = "PCMA"
		freq = 8000
		nCh = 1
	case 9:
		codecName = "G722"
		freq = 8000
		nCh = 1
	case 10:
		codecName = "L16"
		freq = 44100
		nCh = 2
	case 11:
		codecName = "L16"
		freq = 44100
		nCh = 1
	case 12:
		codecName = "QCELP"
		freq = 8000
		nCh = 1
	case 14:
		codecName = "MPA"
		freq = 90000
		nCh = 1
	// 'number of channels' is actually encoded in the media stream
	case 15:
		codecName = "G728"
		freq = 8000
		nCh = 1
	case 16:
		codecName = "DVI4"
		freq = 11025
		nCh = 1
	case 17:
		codecName = "DVI4"
		freq = 22050
		nCh = 1
	case 18:
		codecName = "G729"
		freq = 8000
		nCh = 1
	case 25:
		codecName = "CELB"
		freq = 90000
		nCh = 1
	case 26:
		codecName = "JPEG"
		freq = 90000
		nCh = 1
	case 28:
		codecName = "NV"
		freq = 90000
		nCh = 1
	case 31:
		codecName = "H261"
		freq = 90000
		nCh = 1
	case 32:
		codecName = "MPV"
		freq = 90000
		nCh = 1
	case 33:
		codecName = "MP2T"
		freq = 90000
		nCh = 1
	case 34:
		codecName = "H263"
		freq = 90000
		nCh = 1
	}

	return codecName, freq, nCh
}

func (this *MediaSession) guessRTPTimestampFrequency(mediumName, codecName string) uint {
	// By default, we assume that audio sessions use a frequency of 8000,
	// video sessions use a frequency of 90000,
	// and text sessions use a frequency of 1000.
	// Begin by checking for known exceptions to this rule
	// (where the frequency is known unambiguously (e.g., not like "DVI4"))
	if strings.EqualFold(codecName, "L16") {
		return 44100
	}
	if strings.EqualFold(codecName, "MPA") ||
		strings.EqualFold(codecName, "MPA-ROBUST") ||
		strings.EqualFold(codecName, "X-MP3-DRAFT-00") {
		return 90000
	}

	// Now, guess default values:
	if strings.EqualFold(mediumName, "video") {
		return 90000
	} else if strings.EqualFold(mediumName, "text") {
		return 1000
	}
	return 8000 // for "audio", and any other medium
}

func (this *MediaSession) initiateByMediaType(mimeType string, useSpecialRTPoffset int) bool {
	return true
}

//////// MediaSubSession ////////
type MediaSubSession struct {
	RTPSource              *RTPSource
	rtpSocket              *gs.GroupSock
	rtcpSocket             *gs.GroupSock
	Sink                   IMediaSink
	readSource             IFramedSource
	rtcpInstance           *RTCPInstance
	parent                 *MediaSession
	MiscPtr                interface{}
	numChannels            uint
	rtpChannelID           uint
	rtcpChannelID          uint
	rtpPayloadFormat       uint
	rtpTimestampFrequency  uint
	clientPortNum          uint
	serverPortNum          uint
	bandWidth              uint
	videoWidth             uint
	videoHeight            uint
	sessionID              string
	protocolName           string
	controlPath            string
	savedSDPLines          string
	mediumName             string
	codecName              string
	absStartTime           string
	absEndTime             string
	connectionEndpointName string
	playStartTime          float64
	playEndTime            float64
	videoFPS               float32
	scale                  float32
}

func NewMediaSubSession(parent *MediaSession) *MediaSubSession {
	if parent == nil {
		return nil
	}

	subsession := new(MediaSubSession)
	subsession.parent = parent
	subsession.scale = 1.0
	return subsession
}

func (subsession *MediaSubSession) ParentSession() *MediaSession {
	return subsession.parent
}

func (this *MediaSubSession) Initiate() bool {
	// has already been initiated
	if this.readSource != nil {
		return true
	}

	if len(this.codecName) <= 0 {
		fmt.Println("Codec is unspecified")
		return false
	}

	tempAddr := this.ConnectionEndpointName()

	var success bool
	for {
		// create new socket
		this.rtpSocket = gs.NewGroupSock(tempAddr, 0)
		if this.rtpSocket == nil {
			fmt.Println("Unable to create RTP socket")
			break
		}

		clientPortNum := this.rtpSocket.GetSourcePort()
		if clientPortNum == 0 {
			fmt.Println("Failed to get RTP port number")
			break
		}

		if clientPortNum&1 != 0 {
			this.rtpSocket.Close()
			continue
		}

		this.clientPortNum = clientPortNum

		rtcpPortNum := clientPortNum | 1
		this.rtcpSocket = gs.NewGroupSock(tempAddr, rtcpPortNum)
		if this.rtcpSocket == nil {
			fmt.Println("Unable to create RTCP socket")
			break
		}

		success = true
		break
	}

	if !success {
		if this.rtpSocket != nil {
			this.rtpSocket.Close()
		}
		if this.rtcpSocket != nil {
			this.rtcpSocket.Close()
		}
		return false
	}

	if !this.createSourceObject() {
		return false
	}

	if this.readSource == nil {
		fmt.Println("Failed to create read source.")
		return false
	}

	var totSessionBandwidth uint
	if this.bandWidth != 0 {
		totSessionBandwidth = this.bandWidth + this.bandWidth/20
	} else {
		totSessionBandwidth = 500
	}

	this.rtcpInstance = NewRTCPInstance(this.rtcpSocket, totSessionBandwidth, this.parent.cname)
	return true
}

func (subsession *MediaSubSession) Scale() float32 {
	return subsession.scale
}

func (subsession *MediaSubSession) SetRTPChannelID(rtpChannelID uint) {
	subsession.rtpChannelID = rtpChannelID
}

func (subsession *MediaSubSession) SetRTCPChannelID(rtcpChannelID uint) {
	subsession.rtcpChannelID = rtcpChannelID
}

func (subsession *MediaSubSession) SetServerPortNum(serverPortNum uint) {
	subsession.serverPortNum = serverPortNum
}

func (subsession *MediaSubSession) SetConnectionEndpointName(connectionEndpointName string) {
	subsession.connectionEndpointName = connectionEndpointName
}

func (subsession *MediaSubSession) SetSessionID(sessionID string) {
	subsession.sessionID = sessionID
}

func (subsession *MediaSubSession) SessionID() string {
	return subsession.sessionID
}

func (subsession *MediaSubSession) deInitiate() {
}

func (subsession *MediaSubSession) AbsStartTime() string {
	if subsession.absStartTime != "" {
		return subsession.absStartTime
	}

	return subsession.parent.AbsStartTime()
}

func (this *MediaSubSession) AbsEndTime() string {
	if this.absEndTime != "" {
		return this.absEndTime
	}

	return this.parent.AbsEndTime()
}

func (this *MediaSubSession) CodecName() string {
	return this.codecName
}

func (this *MediaSubSession) MediumName() string {
	return this.mediumName
}

func (this *MediaSubSession) ClientPortNum() uint {
	return this.clientPortNum
}

func (this *MediaSubSession) ProtocolName() string {
	return this.protocolName
}

func (this *MediaSubSession) ControlPath() string {
	return this.controlPath
}

func (this *MediaSubSession) ReadSource() IFramedSource {
	return this.readSource
}

func (this *MediaSubSession) RtcpInstance() *RTCPInstance {
	return this.rtcpInstance
}

func (this *MediaSubSession) SetDestinations(destAddress string) {
}

func (subsession *MediaSubSession) ConnectionEndpointName() string {
	connectionEndpointName := subsession.connectionEndpointName

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		fmt.Println(err)
		return connectionEndpointName
	}

	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				connectionEndpointName = ipnet.IP.String()
				break
			}
		}
	}

	return connectionEndpointName
}

func (this *MediaSubSession) createSourceObject() bool {
	if strings.EqualFold(this.protocolName, "UDP") {
		this.readSource = NewBasicUDPSource(this.rtpSocket)
		this.RTPSource = nil

		// MPEG-2 Transport Stream
		if strings.EqualFold(this.codecName, "MP2T") {
			// this sets "durationInMicroseconds" correctly, based on the PCR values
			//this.readSource = NewMPEG2TransportStreamFramer(this.readSource)
		}
	} else {
		switch this.codecName {
		case "H264":
			this.readSource = NewH264VideoRTPSource(this.rtpSocket,
				this.rtpPayloadFormat, this.rtpTimestampFrequency)
		}
	}
	return true
}

func (this *MediaSubSession) parseSDPLine_b(sdpLine string) bool {
	n, _ := fmt.Sscanf(sdpLine, "b=AS:%d", &this.bandWidth)
	return (n == 1)
}

func (this *MediaSubSession) parseSDPLine_c(sdpLine string) bool {
	// Check for "c=IN IP4 <connection-endpoint>"
	// or "c=IN IP4 <connection-endpoint>/<ttl+numAddresses>"
	// (Later, do something with <ttl+numAddresses> also #####)
	connectionEndpointName := parseCLine(sdpLine)
	if connectionEndpointName != "" {
		this.connectionEndpointName = connectionEndpointName
		return true
	}

	return false
}

func (this *MediaSubSession) parseSDPAttributeRtpmap(sdpLine string) bool {
	var parseSuccess bool
	var numChannels uint = 1

	for {
		if !strings.HasPrefix(sdpLine, "a=rtpmap:") {
			break
		}

		sdpLine = strings.TrimSpace(sdpLine[9:])

		fields := strings.Split(sdpLine, " ")
		if len(fields) != 2 {
			break
		}

		rtpPayloadFormat, err := strconv.Atoi(fields[0])
		if err != nil {
			break
		}
		this.rtpPayloadFormat = uint(rtpPayloadFormat)

		value := strings.Split(fields[1], "/")
		if len(value) == 2 {
			this.codecName = value[0]

			rtpTimestampFrequency, err := strconv.Atoi(value[1])
			if err != nil {
				break
			}
			this.rtpTimestampFrequency = uint(rtpTimestampFrequency)
		} else {
			break
		}

		parseSuccess = true
		this.numChannels = numChannels
	}

	return parseSuccess
}

func (subsession *MediaSubSession) parseSDPAttributeControl(sdpLine string) bool {
	// Check for a "a=control:<control-path>" line:
	var parseSuccess bool

	ok := strings.HasPrefix(sdpLine, "a=control:")
	if ok {
		subsession.controlPath = sdpLine[10:]
		parseSuccess = true
	}

	return parseSuccess
}

func (subsession *MediaSubSession) parseSDPAttributeRange(sdpLine string) bool {
	var parseSuccess bool

	startTime, endTime, ok := parseRangeAttribute(sdpLine, "npt")
	if ok {
		parseSuccess = true

		playStartTime, _ := strconv.ParseFloat(startTime, 32)
		playEndTime, _ := strconv.ParseFloat(endTime, 32)

		if playStartTime > subsession.playStartTime {
			subsession.playStartTime = playStartTime
			if playStartTime > subsession.parent.maxPlayStartTime {
				subsession.parent.maxPlayStartTime = playStartTime
			}
		}
		if playEndTime > subsession.playEndTime {
			subsession.playEndTime = playEndTime
			if playEndTime > subsession.parent.maxPlayEndTime {
				subsession.parent.maxPlayEndTime = playEndTime
			}
		}
	} else if subsession.absStartTime, subsession.absEndTime, ok = parseRangeAttribute(sdpLine, "clock"); ok {
		parseSuccess = true
	}

	return parseSuccess
}

func (this *MediaSubSession) parseSDPAttributeFmtp(sdpLine string) bool {
	return true
}

func (this *MediaSubSession) parseSDPAttributeSourceFilter(sdpLine string) bool {
	return parseSourceFilterAttribute(sdpLine)
}

func (this *MediaSubSession) parseSDPAttributeXDimensions(sdpLine string) bool {
	var parseSuccess bool
	var width, height uint
	num, _ := fmt.Sscanf(sdpLine, "a=x-dimensions:%d,%d", &width, &height)
	if num == 2 {
		parseSuccess = true
		this.videoWidth = width
		this.videoHeight = height
	}
	return parseSuccess
}

func (this *MediaSubSession) parseSDPAttributeFrameRate(sdpLine string) bool {
	// check for a "a=framerate: <fps>" r "a=x-framerate: <fps>" line:
	parseSuccess := true
	for {
		num, _ := fmt.Sscanf(sdpLine, "a=framerate: %f", &this.videoFPS)
		if num == 1 {
			break
		}

		num, _ = fmt.Sscanf(sdpLine, "a=framerate:%f", &this.videoFPS)
		if num == 1 {
			break
		}

		num, _ = fmt.Sscanf(sdpLine, "a=x-framerate: %f", &this.videoFPS)
		if num == 1 {
			break
		}

		parseSuccess = false
		break
	}

	return parseSuccess
}
