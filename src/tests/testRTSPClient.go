package main

import (
	env "UsageEnvironment"
	"flag"
	"fmt"
	. "liveMedia"
	"os"
)

type ourRTSPClient struct {
	RTSPClient
}

type DummySink struct {
	MediaSink
}

var rtspClientCount int

func NewOurRTSPClient(appName, rtspURL string) *ourRTSPClient {
	rtspClient := new(ourRTSPClient)
	rtspClient.InitRTSPClient(rtspURL, appName)
	return rtspClient
}

func main() {
	flag.Parse()
	if flag.NArg() < 1 {
		usage(os.Args[0])
		return
	}

	os.Args[0] = "dorsvr"

	if openURL(os.Args[0], os.Args[1]) {
		env.TaskScheduler().DoEventLoop()
	}
}

func usage(progName string) {
	fmt.Println("Usage: " + progName + " <rtsp-url-1> ... <rtsp-url-N>")
	fmt.Println("\t(where each <rtsp-url-i> is a \"rtsp://\" URL)")
}

func openURL(appName, rtspURL string) bool {
	rtspClient := NewOurRTSPClient(appName, rtspURL)
	if rtspClient == nil {
		fmt.Println("Failed to create a RTSP client URL", rtspURL)
		return false
	}

	rtspClientCount++

	sendBytes := rtspClient.SendDescribeCommand(continueAfterDESCRIBE)
	if sendBytes == 0 {
		fmt.Println("Failed to send describe command.")
		return false
	}

	return true
}

func continueAfterDESCRIBE(rtspClient *RTSPClient, resultCode int, resultStr string) {
	for {
		if resultCode != 0 {
			fmt.Println(fmt.Sprintf("Failed to get a SDP description: %s", resultStr))
			break
		}

		sdpDesc := resultStr

		scs := rtspClient.SCS()
		// Create a media session object from this SDP description
		scs.Session = NewMediaSession(sdpDesc)
		if scs.Session == nil {
			fmt.Println("Failed to create a MediaSession object from the sdp Description.")
			break
		} else if !scs.Session.HasSubSessions() {
			fmt.Println("This session has no media subsessions (i.e., no \"-m\" lines)")
			break
		}

		// Then, create and set up our data source objects for the session.
		setupNextSubSession(rtspClient)
		return
	}

	// An error occurred with this stream.
	shutdownStream(rtspClient)
}

func continueAfterSETUP(rtspClient *RTSPClient, resultCode int, resultStr string) {
	fmt.Println("continueAfterSETUP")
	for {
		if resultCode != 0 {
			fmt.Println(fmt.Sprintf("Failed to set up the subsession"))
			break
		}

		scs := rtspClient.SCS()
		scs.Subsession.Sink = NewDummySink()
		if scs.Subsession.Sink == nil {
			fmt.Println("Failed to create a data sink for the subsession.")
			break
		}

		scs.Subsession.Sink.StartPlaying()
		if scs.Subsession.RtcpInstance() != nil {
			scs.Subsession.RtcpInstance().SetByeHandler(subsessionByeHandler, scs.Subsession)
		}
		break
	}

	// Set up the next subsession, if any:
	setupNextSubSession(rtspClient)
}

func continueAfterPLAY(rtspClient *RTSPClient, resultCode int, resultStr string) {
	for {
		if resultCode != 0 {
			fmt.Println(fmt.Sprintf("Failed to start playing session: %s", resultStr))
			break
		}

		break
	}

	// An unrecoverable error occurred with this stream.
	shutdownStream(rtspClient)
}

func subsessionByeHandler(subsession *MediaSubSession) {
	fmt.Println("Received RTCP BYE on subsession.")

	// Now act as if the subsession had closed:
	subsessionAfterPlaying(subsession)
}

func subsessionAfterPlaying(subsession *MediaSubSession) {
	shutdownStream(nil)
}

func shutdownStream(rtspClient *RTSPClient) {
	//subsession.tcpInstance().setByeHandler(nil, nil)

	scs := rtspClient.SCS()

	if rtspClient != nil {
		rtspClient.SendTeardownCommand(scs.Session, nil)
	}

	fmt.Println("Closing the Stream.")
}

func setupNextSubSession(rtspClient *RTSPClient) {
	scs := rtspClient.SCS()
	scs.Subsession = scs.Next()
	if scs.Subsession != nil {
		if !scs.Subsession.Initiate() {
			fmt.Println("Failed to initiate the subsession.")
			setupNextSubSession(rtspClient)
		} else {
			rtspClient.SendSetupCommand(scs.Subsession, continueAfterSETUP)
		}
		return
	}

	fmt.Println("setupNextSubSession", scs.Subsession)

	if scs.Session.AbsStartTime() != "" {
		rtspClient.SendPlayCommand(scs.Session, continueAfterPLAY)
	} else {
		rtspClient.SendPlayCommand(scs.Session, continueAfterPLAY)
	}
}

func NewDummySink() *DummySink {
	return new(DummySink)
}

func (sink *DummySink) StartPlaying() {
}
