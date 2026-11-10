package emailcase_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/kernel/emailcase"
)

func TestOfTrimsAndLowerCasesAnAddress(t *testing.T) {
	assert.Equal(t, "grace@example.com", emailcase.Of("  Grace@Example.COM\t"))
}

func TestOfPtrKeepsAnAbsentAddressAbsent(t *testing.T) {
	assert.Nil(t, emailcase.OfPtr(nil))
	assert.Equal(t, "a@b.co", *emailcase.OfPtr(new(" A@B.co")))
}
