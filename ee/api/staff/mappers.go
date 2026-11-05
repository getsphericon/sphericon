package staff

import (
	"github.com/getsphericon/sphericon/ee/api/staff/resources"
)

// staffMapper, serviceMapper and serviceInputs are the generated converters of ee/api/staff/resources.
var (
	staffMapper   = &resources.StaffConverterImpl{}
	serviceMapper = &resources.ServiceConverterImpl{}
	serviceInputs = &resources.ServiceInputsImpl{}
)

// periodLayout is how a Usage period is written on the wire: the calendar month, YYYY-MM.
const periodLayout = resources.PeriodLayout
