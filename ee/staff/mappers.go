package staff

import (
	"time"

	staffapi "github.com/getsphericon/sphericon/gen/staff"

	"github.com/getsphericon/sphericon/ee/staff/resources"
)

// staffMapper and serviceMapper are the generated converters of ee/staff/resources.
var (
	staffMapper   = &resources.StaffConverterImpl{}
	serviceMapper = &resources.ServiceConverterImpl{}
)

// periodLayout is how a Usage period is written on the wire: the calendar month, YYYY-MM.
const periodLayout = resources.PeriodLayout

// deliverabilityResource maps the console's rates read model, whose types live in this
// package (the mapper package cannot import it).
func deliverabilityResource(d Deliverability) staffapi.StaffDeliverability {
	domains := make([]staffapi.StaffDomainRates, 0, len(d.Domains))
	for _, r := range d.Domains {
		domains = append(domains, staffapi.StaffDomainRates{
			SendingDomainId: staffapi.EntityId(r.SendingDomainID),
			Domain:          r.Domain,
			ComplaintRate:   rateResource(r.Complaint),
			BounceRate:      rateResource(r.Bounce),
		})
	}
	return staffapi.StaffDeliverability{
		WindowHours: int32(d.Window / time.Hour),
		VolumeFloor: int32(d.VolumeFloor),
		SendVolume:  int32(d.SendVolume),
		Domains:     domains,
	}
}

func rateResource(r Rate) staffapi.StaffRate {
	out := staffapi.StaffRate{Numerator: int32(r.Numerator), Denominator: int32(r.Denominator)}
	if r.Rate == nil {
		out.Rate = staffapi.NilFloat64{Null: true}
	} else {
		out.Rate = staffapi.NewNilFloat64(*r.Rate)
	}
	return out
}
