package validator

import (
	"net/http"
	"testing"
)

func TestIsExpectedHTTPSProbeResponseForAWS(t *testing.T) {
	t.Parallel()

	target := "https://q.us-east-1.amazonaws.com/"

	if !isExpectedHTTPSProbeResponse(target, &http.Response{
		StatusCode: http.StatusNotFound,
		Header: http.Header{
			"X-Amzn-Requestid": []string{"req-1"},
		},
	}) {
		t.Fatal("expected AWS 404 probe response to be accepted")
	}

	if !isExpectedHTTPSProbeResponse(target, &http.Response{
		StatusCode: http.StatusForbidden,
		Header: http.Header{
			"X-Amzn-Requestid": []string{"req-2"},
		},
	}) {
		t.Fatal("expected AWS 403 probe response to be accepted")
	}
}

func TestIsExpectedHTTPSProbeResponseRejectsUnexpectedStatuses(t *testing.T) {
	t.Parallel()

	target := "https://q.us-east-1.amazonaws.com/"

	if isExpectedHTTPSProbeResponse(target, &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     http.Header{},
	}) {
		t.Fatal("expected AWS 404 without request id to be rejected")
	}

	if isExpectedHTTPSProbeResponse(target, &http.Response{
		StatusCode: http.StatusProxyAuthRequired,
		Header: http.Header{
			"x-amzn-requestid": []string{"req-3"},
		},
	}) {
		t.Fatal("expected 407 to be rejected")
	}

	if isExpectedHTTPSProbeResponse("https://example.com/", &http.Response{
		StatusCode: http.StatusForbidden,
		Header: http.Header{
			"X-Amzn-Requestid": []string{"req-4"},
		},
	}) {
		t.Fatal("expected non-AWS 403 probe response to be rejected")
	}
}
