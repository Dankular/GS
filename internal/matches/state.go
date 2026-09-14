package matches

import "fmt"

type State string

const (
	Queued     State = "Queued"
	Matched    State = "Matched"
	Allocating State = "Allocating"
	Ready      State = "Ready"
	Running    State = "Running"
	Finalizing State = "Finalizing"
	Completed  State = "Completed"
	Abandoned  State = "Abandoned"
	Disputed   State = "Disputed"
	Failed     State = "Failed"
	Cancelled  State = "Cancelled"
)

func Transition(from State, event string) (State, error) {
	allowed := map[State]map[string]State{
		Queued:     {"matched": Matched, "cancelled": Cancelled},
		Matched:    {"allocate": Allocating},
		Allocating: {"ready": Ready, "failed": Failed},
		Ready:      {"start": Running, "failed": Failed},
		Running:    {"finalize": Finalizing, "abandon": Abandoned},
		Finalizing: {"complete": Completed, "dispute": Disputed},
	}
	if next, ok := allowed[from][event]; ok {
		return next, nil
	}
	return "", fmt.Errorf("invalid match transition %s --%s-->", from, event)
}
