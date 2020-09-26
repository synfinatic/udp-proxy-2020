package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"strings"
	"time"
)

// Check to see if the string is in the slice
func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

// Check to see if the string prefix is in the slice
func stringPrefixInSlice(a string, list []string) bool {
	for _, b := range list {
		if strings.HasPrefix(b, a) {
			return true
		}
	}
	return false
}

// takes a list of ports and builds our BPF filter
func buildBPFFilter(ports []int32) string {
	if len(ports) < 1 {
		log.Fatal("--port must be specified one or more times")
	}
	var bpf_filters = []string{}
	for _, p := range ports {
		bpf_filters = append(bpf_filters, fmt.Sprintf("udp port %d", p))
	}
	var bpf_filter string
	if len(ports) > 1 {
		bpf_filter = strings.Join(bpf_filters, " or ")
	} else {
		bpf_filter = bpf_filters[0]
	}
	return bpf_filter
}

func parseTimeout(timeout int64) time.Duration {
	d := fmt.Sprintf("%dms", timeout)
	to, err := time.ParseDuration(d)
	if err != nil {
		log.Fatal(err)
	}
	return to
}
