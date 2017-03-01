package livemedia

type MPEGVideoStreamParser struct {
	StreamParser
	limit                  []byte
	buffTo                 []byte
	saveTo                 []byte
	startOfFrame           []byte
	numTruncatedBytes      uint
	savedNumTruncatedBytes uint
	usingSource            IFramedSource
}

func (p *MPEGVideoStreamParser) initMPEGVideoStreamParser(inputSource IFramedSource) {
	p.usingSource = inputSource

	p.initStreamParser(inputSource,
		inputSource.(*MPEGVideoStreamFramer).handleClosure,
		inputSource.(*MPEGVideoStreamFramer).continueReadProcessing)
}

func (p *MPEGVideoStreamParser) registerReadInterest(buffTo []byte, maxSize uint) {
	p.buffTo = buffTo
	p.saveTo = buffTo
	p.limit = buffTo[maxSize:]
	p.numTruncatedBytes = 0
	p.savedNumTruncatedBytes = 0
	p.startOfFrame = p.buffTo
}

func (p *MPEGVideoStreamParser) saveByte(ubyte uint) {
}

func (p *MPEGVideoStreamParser) save4Bytes(word uint) {
}

func (p *MPEGVideoStreamParser) curFrameSize() uint {
	return 0
}

func (p *MPEGVideoStreamParser) setParseState() {
	p.saveParserState()
}
