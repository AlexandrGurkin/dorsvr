package liveMedia

import (
	"fmt"
	. "groupsock"
	"utils"
)

const (
	// RTCP packet types:
	RTCP_PT_SR   = 200
	RTCP_PT_RR   = 201
	RTCP_PT_SDES = 202
	RTCP_PT_BYE  = 203
	RTCP_PT_APP  = 204

	// SDES tags:
	RTCP_SDES_END   = 0
	RTCP_SDES_CNAME = 1
	RTCP_SDES_NAME  = 2
	RTCP_SDES_EMAIL = 3
	RTCP_SDES_PHONE = 4
	RTCP_SDES_LOC   = 5
	RTCP_SDES_TOOL  = 6
	RTCP_SDES_NOTE  = 7
	RTCP_SDES_PRIV  = 8

	// overhead (bytes) of IP and UDP hdrs
	IP_UDP_HDR_SIZE = 28
)

type SDESItem struct {
	data []byte
}

// bytes, (1500, minus some allowance for IP, UDP, UMTP headers)
var maxRTCPPacketSize uint = 1450
var preferredPacketSize uint = 1000 // bytes

type RTCPInstance struct {
	typeOfEvent        uint
	lastSentSize       uint
	totSessionBW       uint
	lastPacketSentSize uint
	haveJustSentPacket bool
	prevReportTime     int64
	nextReportTime     int64
	inBuf              []byte
	CNAME              *SDESItem
	Sink               *RTPSink
	Source             *RTPSource
	outBuf             *OutPacketBuffer
	rtcpInterface      *RTPInterface
	ByeHandlerTask     interface{}
	SRHandlerTask      interface{}
	RRHandlerTask      interface{}
}

func NewSDESItem(tag int, value string) *SDESItem {
	item := new(SDESItem)

	length := len(value)
	if length > 0xFF {
		length = 0xFF // maximum data length for a SDES item
	}

	item.data = []byte{byte(tag), byte(length)}
	return item
}

func dTimeNow() int64 {
	var timeNow utils.Timeval
	utils.GetTimeOfDay(&timeNow)
	return timeNow.Tv_sec + timeNow.Tv_usec/1000000.0
}

func (this *SDESItem) totalSize() uint {
	return 2 + uint(this.data[1])
}

func NewRTCPInstance(rtcpGS *GroupSock, totSessionBW uint, cname string) *RTCPInstance {
	rtcp := new(RTCPInstance)
	rtcp.typeOfEvent = EVENT_REPORT
	rtcp.totSessionBW = totSessionBW
	rtcp.CNAME = NewSDESItem(RTCP_SDES_CNAME, cname)

	rtcp.prevReportTime = dTimeNow()
	rtcp.nextReportTime = rtcp.prevReportTime

	rtcp.inBuf = make([]byte, maxRTCPPacketSize)
	rtcp.outBuf = NewOutPacketBuffer(preferredPacketSize, maxRTCPPacketSize)

	rtcp.rtcpInterface = NewRTPInterface(rtcp, rtcpGS)
	rtcp.rtcpInterface.startNetworkReading()

	go rtcp.incomingReportHandler()
	go rtcp.onExpire()
	return rtcp
}

func (this *RTCPInstance) setSpecificRRHandler() {
}

func (this *RTCPInstance) SetByeHandler(handlerTask interface{}, clientData interface{}) {
	this.ByeHandlerTask = handlerTask
	//this.byeHandlerClientData = clientData
}

func (this *RTCPInstance) setSRHandler() {
}

func (this *RTCPInstance) setRRHandler() {
}

func (this *RTCPInstance) incomingReportHandler() {
	readResult := this.rtcpInterface.handleRead()
	fmt.Println(readResult)
}

func (this *RTCPInstance) onReceive() {
}

func (this *RTCPInstance) sendReport() {
	// Begin by including a SR and/or RR report:
	this.addReport()

	// Then, include a SDES:
	this.addSDES()

	// Send the report:
	this.sendBuiltPacket()
}

func (this *RTCPInstance) sendBuiltPacket() {
	reportSize := this.outBuf.curPacketSize()
	this.rtcpInterface.sendPacket(this.outBuf.packet(), reportSize)
	this.outBuf.resetOffset()

	this.lastSentSize = uint(IP_UDP_HDR_SIZE) + reportSize
	this.haveJustSentPacket = true
	this.lastPacketSentSize = reportSize
}

func (this *RTCPInstance) addReport() {
	if this.Sink != nil {
		if this.Sink.EnableRTCPReports() {
			return
		}

		if this.Sink.NextTimestampHasBeenPreset() {
			return
		}

		this.addSR()
	} else if this.Source != nil {
		this.addRR()
	}
}

func (this *RTCPInstance) addSDES() {
	numBytes := 4
	//numBytes += this.CNAME.totalSize()
	numBytes += 1

	num4ByteWords := (numBytes + 3) / 4

	var rtcpHdr int64 = 0x81000000 // version 2, no padding, 1 SSRC chunk
	rtcpHdr |= (RTCP_PT_SDES << 16)
	rtcpHdr |= int64(num4ByteWords)
	this.outBuf.enqueueWord(uint(rtcpHdr))
}

func (this *RTCPInstance) addSR() {
	//this.enqueueCommonReportPrefix(RTCP_PT_SR, this.Source.SSRC(), 0)
	this.enqueueCommonReportSuffix()
}

func (this *RTCPInstance) addRR() {
	//this.enqueueCommonReportPrefix(RTCP_PT_RR, this.Source.SSRC(), 0)
	this.enqueueCommonReportSuffix()
}

func (this *RTCPInstance) onExpire() {
}

func (this *RTCPInstance) unsetSpecificRRHandler() {
}

func (this *RTCPInstance) enqueueCommonReportPrefix(packetType, SSRC, numExtraWords uint) {
}

func (this *RTCPInstance) enqueueCommonReportSuffix() {
}
