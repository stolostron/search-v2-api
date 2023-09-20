// handlers_test.go
package rbac

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckDBAvailabilityError(t *testing.T) {
	// Create a request to pass to our handler.
	req, err := http.NewRequest("GET", "/search", nil)
	if err != nil {
		t.Fatal(err)
	}
	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	// execute function
	result := CheckDBAvailability(handler)

	// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
	// directly and pass in our Request and ResponseRecorder.
	result.ServeHTTP(rr, req)
	// Check the status code is what we expect.
	if status := rr.Code; status != http.StatusServiceUnavailable {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusServiceUnavailable)
	}
	// Check the response body is what we expect.
	expected := "Unable to establish connection with database.\n"
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got *%v* want *%v*",
			rr.Body.String(), expected)
	}
}
