package metrics

import (
	"testing"
)

// Ensure we do not include raw api_key label in the default RequestsTotal metric
func TestRequestsTotalLabels(t *testing.T) {
	// Calling WithLabelValues with four labels should panic because the
	// metric expects only three labels (method,path,status). Ensure that
	// passing an api_key as an extra label results in a panic.
	didPanic := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				didPanic = true
			}
		}()
		// This should panic due to wrong number of labels
		RequestsTotal.WithLabelValues("GET", "/", "200", "some-key")
	}()
	if !didPanic {
		t.Error("expected wrong label count to panic — api_key label should not be accepted")
	}
}
