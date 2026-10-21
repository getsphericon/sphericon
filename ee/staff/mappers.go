package staff

import (
	"github.com/getsphericon/sphericon/ee/staff/resources"
)

// staffMapper and serviceMapper are the generated converters of ee/staff/resources.
var (
	staffMapper   = &resources.StaffConverterImpl{}
	serviceMapper = &resources.ServiceConverterImpl{}
)

// periodLayout is how a Usage period is written on the wire: the calendar month, YYYY-MM.
const periodLayout = resources.PeriodLayout
