package main

import "testing"

func TestDefaultHeadersPOSTDefaultsJSON(t *testing.T) {
	h := defaultHeaders("POST", nil)
	if h["Content-Type"] != "application/json; charset=utf-8" {
		t.Fatalf("POST should default Content-Type, got %q", h["Content-Type"])
	}
}

func TestDefaultHeadersRespectsExisting(t *testing.T) {
	// existing Content-Type (any case) must not be overwritten
	h := defaultHeaders("PUT", map[string]string{"content-type": "text/plain"})
	if h["content-type"] != "text/plain" {
		t.Fatalf("existing content-type overwritten: %v", h)
	}
	if _, dup := h["Content-Type"]; dup {
		t.Fatalf("added a duplicate Content-Type: %v", h)
	}
}

func TestDefaultHeadersGETUnchanged(t *testing.T) {
	h := defaultHeaders("GET", map[string]string{"X-A": "1"})
	if _, ok := h["Content-Type"]; ok {
		t.Fatalf("GET must not get a default Content-Type: %v", h)
	}
}
