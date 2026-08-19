package udscmds

import (
	"fmt"
	"io"

	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/throttle"
)

type ThrottlePrintBuckets struct {
	AppProvider framework.AppProviderFunc
}

func (h *ThrottlePrintBuckets) GroupName() string {
	return "throttle"
}

func (h *ThrottlePrintBuckets) Command() string {
	return "throttle-print-buckets"
}

func (h *ThrottlePrintBuckets) Desc() string {
	return "Print all the buckets in the throttle bucket store"
}

func (h *ThrottlePrintBuckets) Usage() string {
	return h.Command()
}

func (h *ThrottlePrintBuckets) HandleCommand(_ []string, w io.Writer) error {
	appCore := h.AppProvider().AppCore()
	// Node-plane typed reach: inspection must work on a stopped service too —
	// the buckets are passive state and survive Stop.
	throttleBucketStore, ok := appCore.ThrottleHandle().Node().Service().(*throttle.Service)
	if !ok {
		return fmt.Errorf("throttle service not configured in this app")
	}
	keyMap := throttleBucketStore.Inspect()
	for groupID, localIDs := range keyMap {
		_, _ = fmt.Fprintf(w, "\n[%s]\n", groupID)
		for _, localID := range localIDs {
			_, _ = fmt.Fprintln(w, localID)
		}
	}
	return nil
}
