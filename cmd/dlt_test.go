package main

import (
	"github.com/google/gopacket/layers"
	"runtime"
	"testing"
)

func TestIsValidLayerType(t *testing.T) {
	// These are valid
	if !isValidLayerType(layers.LinkTypeEthernet) {
		t.Errorf("Expected that Ethernet was valid")
	}
	// Test all the Raw types because they are special
	if !isValidLayerType(layers.LinkTypeRaw) {
		t.Errorf("Expected that Raw was valid")
	}

	if runtime.GOOS == "openbsd" {
		if !isValidLayerType(LinkTypeRawOpenBSD) {
			t.Errorf("Expected that LinkTypeRawOpenBSD was valid")
		}
		if isValidLayerType(LinkTypeRawOthers) {
			t.Errorf("Expected that LinkTypeRawOthers was invalid")
		}
	} else {
		if !isValidLayerType(LinkTypeRawOthers) {
			t.Errorf("Expected that LinkTypeRawOthers was valid")
		}
		if isValidLayerType(LinkTypeRawOpenBSD) {
			t.Errorf("Expected that LinkTypeRawOpenBSD was invalid")
		}
	}

	// not valid
	if isValidLayerType(layers.LinkTypeLinuxSLL) {
		t.Errorf("Did not expect that Linux SLL was valid")
	}
}

func TestLayerTypeRaw(t *testing.T) {
	if runtime.GOOS == "openbsd" {
		if LinkTypeRawOpenBSD.String() != "Raw" {
			t.Errorf("RawOpenBSD should be known as 'Raw': %s", LinkTypeRawOpenBSD.String())
		}
		if LinkTypeRawOthers.String() == "Raw" {
			t.Errorf("RawOthers should not be known as 'Raw': %s", LinkTypeRawOthers.String())
		}
	} else {
		if LinkTypeRawOthers.String() != "Raw" {
			t.Errorf("RawOthers should be known as 'Raw': %s", LinkTypeRawOthers.String())
		}
		if LinkTypeRawOpenBSD.String() == "Raw" {
			t.Errorf("RawOpenBSD should not be known as 'Raw': %s", LinkTypeRawOpenBSD.String())
		}
	}
}
