package abuse

import (
	"time"

	"github.com/getsphericon/sphericon/internal/reputation"
)

// DomainRates are the Complaint and Bounce rates of one Sending domain, as the detector reads
// them: a rate is undefined while the domain sent fewer than the volume floor in the window.
type DomainRates struct {
	SendingDomainID int64
	Domain          string
	Complaint       reputation.Rate
	Bounce          reputation.Rate
}

// Deliverability is a Workspace's rates per Sending domain and its send volume over Window.
type Deliverability struct {
	Window      time.Duration
	VolumeFloor int
	// SendVolume is the messages sent in the window across all Sending domains
	// (email.sent Events, by their own timestamp).
	SendVolume int
	Domains    []DomainRates
}
