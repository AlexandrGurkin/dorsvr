package groupsock

import (
	"fmt"
	"testing"
)

func TestOurRandom32(t *testing.T) {
	sessionID := fmt.Sprintf("%010d", OurRandom32())
	if len(sessionID) == 10 {
		t.Log("success")
	} else {
		t.Error("failed")
	}
}
