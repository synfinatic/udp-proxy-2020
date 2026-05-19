package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy/stages"
)

type testSource struct {
	name string
}

func (s *testSource) Read(ctx context.Context) (*proxy.Packet, error) {
	return nil, context.Canceled
}

func (s *testSource) Close() error {
	return nil
}

func (s *testSource) Name() string {
	return s.name
}

type taggedSink struct {
	target string
}

func (s *taggedSink) Name() string {
	return "taggedSink(" + s.target + ")"
}

func (s *taggedSink) Write(pkt *proxy.Packet) error {
	return nil
}

func (s *taggedSink) Close() error {
	return nil
}

func TestAttachCrossInterfaceSinks_AssignsAllOtherInterfaces(t *testing.T) {
	inames := []string{"eth0", "eth1", "eth2"}
	states := make([]ifaceState, 0, len(inames))
	pipelinesByName := make(map[string]*proxy.Pipeline, len(inames))

	for _, iname := range inames {
		pipe := proxy.NewPipeline(&testSource{name: "src:" + iname})
		states = append(states, ifaceState{name: iname, pipeline: pipe})
		pipelinesByName[iname] = pipe
	}

	err := attachCrossInterfaceSinks(states, func(src, dst ifaceState) error {
		src.pipeline.AddSink(&taggedSink{target: dst.name})
		return nil
	})
	if err != nil {
		t.Fatalf("attachCrossInterfaceSinks failed: %v", err)
	}

	for _, src := range inames {
		pipe := pipelinesByName[src]
		if got, want := len(pipe.Sinks), len(inames)-1; got != want {
			t.Fatalf("pipeline %s expected %d sinks, got %d", src, want, got)
		}

		seen := make(map[string]bool, len(pipe.Sinks))
		for _, sink := range pipe.Sinks {
			tagged, ok := sink.(*taggedSink)
			if !ok {
				t.Fatalf("pipeline %s has unexpected sink type %T", src, sink)
			}
			if tagged.target == src {
				t.Fatalf("pipeline %s should not include sink to itself", src)
			}
			seen[tagged.target] = true
		}

		for _, dst := range inames {
			if dst == src {
				continue
			}
			if !seen[dst] {
				t.Fatalf("pipeline %s missing sink to %s", src, dst)
			}
		}
	}
}

func TestAttachCrossInterfaceSinks_PropagatesErrors(t *testing.T) {
	states := []ifaceState{
		{name: "eth0", pipeline: proxy.NewPipeline(&testSource{name: "src:eth0"})},
		{name: "eth1", pipeline: proxy.NewPipeline(&testSource{name: "src:eth1"})},
	}

	errExpected := errors.New("boom")
	err := attachCrossInterfaceSinks(states, func(src, dst ifaceState) error {
		if src.name == "eth0" && dst.name == "eth1" {
			return errExpected
		}
		return nil
	})
	if !errors.Is(err, errExpected) {
		t.Fatalf("expected propagated error %v, got %v", errExpected, err)
	}
}

func TestBuildSharedRegistries_UsesSingleSharedRegistry(t *testing.T) {
	original := newRegistryProcessorByInterface
	defer func() {
		newRegistryProcessorByInterface = original
	}()

	called := 0
	returned := &stages.RegistryProcessor{}
	newRegistryProcessorByInterface = func(ttl time.Duration, fixedIPs map[string][]string) (*stages.RegistryProcessor, error) {
		called++
		if ttl != 5*time.Minute {
			t.Fatalf("expected ttl 5m, got %v", ttl)
		}
		if len(fixedIPs) != 2 {
			t.Fatalf("expected two interfaces in fixedIPs, got %d", len(fixedIPs))
		}
		return returned, nil
	}

	registries, err := buildSharedRegistries(5*time.Minute, map[string][]string{
		"eth0": {"10.0.0.10"},
		"eth1": {"10.0.1.10"},
	})
	if err != nil {
		t.Fatalf("buildSharedRegistries failed: %v", err)
	}
	if called != 1 {
		t.Fatalf("expected registry constructor to be called once, got %d", called)
	}
	if len(registries) != 1 {
		t.Fatalf("expected exactly one shared registry, got %d", len(registries))
	}
	if registries[0] != returned {
		t.Fatal("expected returned registry to be the shared instance from constructor")
	}
}
