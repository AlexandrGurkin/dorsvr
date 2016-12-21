package rtspclient

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
)

var OutPacketBufferMaxSize uint = 60000 // default

//////// OutPacketBuffer ////////
type OutPacketBuffer struct {
	buff                           []byte
	limit                          uint
	preferred                      uint
	curOffset                      uint
	packetStart                    uint
	maxPacketSize                  uint
	overflowDataSize               uint
	overflowDataOffset             uint
	overflowDurationInMicroseconds uint
	overflowPresentationTime       Timeval
}

func NewOutPacketBuffer(preferredPacketSize, maxPacketSize uint) *OutPacketBuffer {
	outPacketBuffer := new(OutPacketBuffer)
	outPacketBuffer.preferred = preferredPacketSize
	outPacketBuffer.maxPacketSize = maxPacketSize

	maxNumPackets := (OutPacketBufferMaxSize - (maxPacketSize - 1)) / maxPacketSize
	outPacketBuffer.limit = maxNumPackets * maxPacketSize
	outPacketBuffer.buff = make([]byte, outPacketBuffer.limit)
	outPacketBuffer.resetOffset()
	outPacketBuffer.resetPacketStart()
	outPacketBuffer.resetOverflowData()
	return outPacketBuffer
}

func (this *OutPacketBuffer) packet() []byte {
	return this.buff[this.packetStart:]
}

func (this *OutPacketBuffer) curPtr() []byte {
	return this.buff[(this.packetStart + this.curOffset):]
}

func (this *OutPacketBuffer) curPacketSize() uint {
	return this.curOffset
}

func (this *OutPacketBuffer) totalBufferSize() uint {
	return this.limit
}

func (this *OutPacketBuffer) increment(numBytes uint) {
	this.curOffset += numBytes
}

func (this *OutPacketBuffer) haveOverflowData() bool {
	return this.overflowDataSize > 0
}

func (this *OutPacketBuffer) isPreferredSize() bool {
	return this.curOffset >= this.preferred
}

func (this *OutPacketBuffer) useOverflowData() {
	this.enqueue(this.buff[(this.packetStart+this.overflowDataOffset):], this.overflowDataSize)
}

func (this *OutPacketBuffer) OverflowDataSize() uint {
	return this.overflowDataSize
}

func (this *OutPacketBuffer) OverflowPresentationTime() Timeval {
	return this.overflowPresentationTime
}

func (this *OutPacketBuffer) OverflowDurationInMicroseconds() uint {
	return this.overflowDurationInMicroseconds
}

func (this *OutPacketBuffer) adjustPacketStart(numBytes uint) {
	this.packetStart += numBytes
	if this.overflowDataOffset >= numBytes {
		this.overflowDataOffset -= numBytes
	} else {
		this.overflowDataOffset = 0
		this.overflowDataSize = 0
	}
}

func (this *OutPacketBuffer) totalBytesAvailable() uint {
	return this.limit - (this.packetStart + this.curOffset)
}

func (this *OutPacketBuffer) enqueue(from []byte, numBytes uint) {
	if numBytes > this.totalBytesAvailable() {
		fmt.Println("OutPacketBuffer::enqueue() warning: %d > %d", numBytes, this.totalBytesAvailable())
		numBytes = this.totalBytesAvailable()
	}

	if string(this.curPtr()) != string(from) {
		//this.curPtr() = from
	}
	this.increment(numBytes)
}

func (this *OutPacketBuffer) enqueueWord(word uint) {
	buf := bytes.NewBuffer([]byte{})
	binary.Write(buf, binary.BigEndian, word)
	this.enqueue(buf.Bytes(), 4)
}

func (this *OutPacketBuffer) insert(from []byte, numBytes, toPosition uint) {
	realToPosition := this.packetStart + toPosition
	if realToPosition+numBytes > this.limit {
		if realToPosition > this.limit {
			return // we can't do this
		}
		numBytes = this.limit - realToPosition
	}

	//memmove(&fBuf[realToPosition], from, numBytes)
	if toPosition+numBytes > this.curOffset {
		this.curOffset = toPosition + numBytes
	}
}

func (this *OutPacketBuffer) insertWord(word byte, toPosition uint) {
}

func (this *OutPacketBuffer) wouldOverflow(numBytes uint) bool {
	return (this.curOffset + numBytes) > this.maxPacketSize
}

func (this *OutPacketBuffer) skipBytes(numBytes uint) {
}

func (this *OutPacketBuffer) resetPacketStart() {
	if this.overflowDataSize > 0 {
		this.overflowDataOffset += this.packetStart
	}
	this.packetStart = 0
}

func (this *OutPacketBuffer) resetOffset() {
	this.curOffset = 0
}

func (this *OutPacketBuffer) resetOverflowData() {
	this.overflowDataSize = 0
	this.overflowDataOffset = 0
}

//////// MediaSink ////////
type IMediaSink interface {
	StartPlaying(source IFramedSource) bool
}

type MediaSink struct {
	Source  IFramedSource
	rtpSink IRTPSink
}

func (this *MediaSink) InitMediaSink(rtpSink IRTPSink) {
	this.rtpSink = rtpSink
}

func (this *MediaSink) StartPlaying(source IFramedSource) bool {
	if this.Source != nil {
		fmt.Println("This sink is already being played")
		return false
	}

	if this.rtpSink == nil {
		fmt.Println("This RTP Sink is nil")
		return false
	}

	this.Source = source
	this.rtpSink.ContinuePlaying()
	return true
}

func (this *MediaSink) StopPlaying() {
	// First, tell the source that we're no longer interested:
	if this.Source != nil {
		this.Source.stopGettingFrames()
	}
}

func (sink *MediaSink) AuxSDPLine() string {
	return ""
}

func (sink *MediaSink) RtpPayloadType() uint {
	return 0
}

func (sink *MediaSink) RtpmapLine() string {
	return ""
}

func (sink *MediaSink) SdpMediaType() string {
	return ""
}

func (this *MediaSink) OnSourceClosure() {
}

func (sink *MediaSink) addStreamSocket(sockNum net.Conn, streamChannelID uint) {
	return
}

func (sink *MediaSink) delStreamSocket() {
}

func (sink *MediaSink) currentSeqNo() uint {
	return 0
}

func (sink *MediaSink) presetNextTimestamp() uint {
	return 0
}
