package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/synfinatic/udp-proxy-2020/internal/proxy"
	"github.com/synfinatic/udp-proxy-2020/internal/proxy/stages"
)

// GenerateDotFile creates a Graphviz dot file representing the pipelines.
func GenerateDotFile(pipelines []*proxy.Pipeline, filepath string) error {
	var buf strings.Builder
	usedSource := len(pipelines) > 0
	usedRewrite := false
	usedLoop := false
	usedProcessor := false
	usedSink := false
	usedRouteCluster := false
	usedForwardSink := false

	fmt.Fprintf(&buf, "digraph Pipelines {\n")
	fmt.Fprintf(&buf, "  rankdir=LR;\n")
	fmt.Fprintf(&buf, "  compound=true;\n")
	fmt.Fprintf(&buf, "  node [shape=box, style=rounded];\n\n")

	// Process each pipeline
	for i, pipeline := range pipelines {
		pipelineID := fmt.Sprintf("pipeline_%d", i)

		// Get source name
		sourceName := pipeline.Source.Name()

		// Create node for source
		sourceNodeID := fmt.Sprintf("%s_source", pipelineID)
		fmt.Fprintf(&buf, "  %s [label=\"%s\", shape=oval, style=\"filled,bold\", fillcolor=\"lightskyblue\", color=\"steelblue4\"];\n",
			sourceNodeID, sourceName)

		// Track the previous node for edge creation
		prevNodeID := sourceNodeID

		// Process each processor
		for j, processor := range pipeline.Processors {
			usedProcessor = true
			processorNodeID := fmt.Sprintf("%s_proc_%d", pipelineID, j)
			processorName := processor.Name()
			fmt.Fprintf(&buf, "  %s [label=\"%s\", shape=box, style=\"rounded,filled\", fillcolor=\"khaki1\"];\n",
				processorNodeID, processorName)
			fmt.Fprintf(&buf, "  %s -> %s;\n", prevNodeID, processorNodeID)
			prevNodeID = processorNodeID
		}

		// Process each sink
		for j, sink := range pipeline.Sinks {
			// Special handling for RouteSink
			if routeSink, ok := sink.(*stages.RouteSink); ok {
				usedRouteCluster = true
				usedLoop = true
				usedRewrite = true
				usedSink = true
				if len(routeSink.Processors) > 0 {
					usedProcessor = true
				}

				baseID := fmt.Sprintf("%s_sink_%d", pipelineID, j)
				clusterID := fmt.Sprintf("cluster_%s", baseID)
				// Subgraph cluster for RouteSink internals
				fmt.Fprintf(&buf, "  subgraph %s {\n", clusterID)
				fmt.Fprintf(&buf, "    label=\"%s\";\n", routeSink.Name())
				fmt.Fprintf(&buf, "    style=\"filled,bold\";\n")
				fmt.Fprintf(&buf, "    fillcolor=\"honeydew\";\n")
				fmt.Fprintf(&buf, "    color=\"darkgreen\";\n\n")

				// Per-target loop icon node (high contrast so it remains visible in rendered PNGs)
				loopID := baseID + "_loop"
				fmt.Fprintf(&buf, "    %s [label=\"⟳\\nper target\", shape=circle, width=1.15, height=1.15, fixedsize=true, style=\"filled,bold\", fillcolor=gray90, color=gray20, fontcolor=black, fontsize=13];\n", loopID)

				// Rewrite node
				rewriteID := baseID + "_rewrite"
				fmt.Fprintf(&buf, "    %s [label=\"Rewrite\\nPacket\", shape=component, fillcolor=\"thistle1\", style=\"filled\"];\n", rewriteID)

				// Connect loop icon to rewrite node
				fmt.Fprintf(&buf, "    %s -> %s;\n", loopID, rewriteID)

				// Processor nodes
				procNodeID := rewriteID
				for k, proc := range routeSink.Processors {
					procID := fmt.Sprintf("%s_proc_%d", baseID, k)
					fmt.Fprintf(&buf, "    %s [label=\"%s\", shape=box, style=\"rounded,filled\", fillcolor=\"khaki1\"];\n", procID, proc.Name())
					fmt.Fprintf(&buf, "    %s -> %s;\n", procNodeID, procID)
					procNodeID = procID
				}

				// List nested sinks directly, fanning out from the last processing node
				for k, sink := range routeSink.Sinks {
					nestedSinkID := fmt.Sprintf("%s_sink_%d", baseID, k)
					fmt.Fprintf(&buf, "    %s [label=\"%s\", shape=oval, fillcolor=\"mediumseagreen\", fontcolor=\"white\", style=\"filled,bold\"];\n", nestedSinkID, sink.Name())
					fmt.Fprintf(&buf, "    %s -> %s;\n", procNodeID, nestedSinkID)
				}

				fmt.Fprintf(&buf, "  }\n\n")
				// Connect previous node to the RouteSink cluster boundary (compound edge)
				loopIDFull := baseID + "_loop"
				fmt.Fprintf(&buf, "  %s -> %s [lhead=%s];\n", prevNodeID, loopIDFull, clusterID)
				// No update to prevNodeID (RouteSink is a terminal node)
			} else {
				// Generic sink handling (ForwardingSink, PcapFileSink, etc.)
				sinkNodeID := fmt.Sprintf("%s_sink_%d", pipelineID, j)
				sinkName := sink.Name()
				usedSink = true

				fillColor := "mediumseagreen"
				fontColor := "white"
				shape := "oval"
				extraStyle := "filled,bold"
				if isForwardingSink(sink) {
					fillColor = "lightsalmon"
					fontColor = "black"
					shape = "hexagon"
					extraStyle = "filled"
					usedForwardSink = true
				}

				fmt.Fprintf(&buf, "  %s [label=\"%s\", shape=%s, style=\"%s\", fillcolor=\"%s\", fontcolor=\"%s\"];\n",
					sinkNodeID, sinkName, shape, extraStyle, fillColor, fontColor)
				fmt.Fprintf(&buf, "  %s -> %s;\n", prevNodeID, sinkNodeID)
			}
		}

		fmt.Fprintf(&buf, "\n")
	}

	// Add cross-pipeline forwarding edges
	addCrossPipelineEdges(&buf, pipelines)

	includeLegend := usedSource || usedRewrite || usedLoop || usedProcessor || usedSink || usedRouteCluster || usedForwardSink
	if includeLegend {
		// Add extra spacing before legend (2x spacer rows)
		fmt.Fprintf(&buf, "\n// Legend section\n")
		fmt.Fprintf(&buf, "{ rank = sink; dummy_legend_spacer [style=invis, height=0.2, width=0.0, label=\"\"]; }\n")
		fmt.Fprintf(&buf, "{ rank = sink; dummy_legend_spacer2 [style=invis, height=0.2, width=0.0, label=\"\"]; }\n")

		// Legend cluster at the bottom (only include entries used in this diagram)
		fmt.Fprintf(&buf, "  subgraph cluster_legend {\n")
		fmt.Fprintf(&buf, "    label=\"Legend\";\n")
		fmt.Fprintf(&buf, "    style=\"filled,dashed\";\n")
		fmt.Fprintf(&buf, "    color=\"gray\";\n")

		legendNodeIDs := make([]string, 0, 7)
		if usedSource {
			fmt.Fprintf(&buf, "    legend_source [label=\"Source\", shape=oval, style=\"filled,bold\", fillcolor=\"lightskyblue\", color=\"steelblue4\"];\n")
			legendNodeIDs = append(legendNodeIDs, "legend_source")
		}
		if usedProcessor {
			fmt.Fprintf(&buf, "    legend_proc [label=\"Processor\", shape=box, style=\"rounded,filled\", fillcolor=\"khaki1\"];\n")
			legendNodeIDs = append(legendNodeIDs, "legend_proc")
		}
		if usedRouteCluster {
			fmt.Fprintf(&buf, "    legend_route [label=\"RouteSink Cluster\", shape=folder, fillcolor=\"honeydew\", style=\"filled,bold\", color=\"darkgreen\"];\n")
			legendNodeIDs = append(legendNodeIDs, "legend_route")
		}
		if usedLoop {
			fmt.Fprintf(&buf, "    legend_loop [label=\"⟳\\nper target\", shape=circle, width=1.0, height=1.0, fixedsize=true, style=\"filled,bold\", fillcolor=gray90, color=gray30, fontcolor=black, fontsize=12];\n")
			legendNodeIDs = append(legendNodeIDs, "legend_loop")
		}
		if usedRewrite {
			fmt.Fprintf(&buf, "    legend_rewrite [label=\"Rewrite\\nPacket\", shape=component, fillcolor=\"thistle1\", style=\"filled\"];\n")
			legendNodeIDs = append(legendNodeIDs, "legend_rewrite")
		}
		if usedSink {
			fmt.Fprintf(&buf, "    legend_sink [label=\"Sink\", shape=oval, fillcolor=\"mediumseagreen\", fontcolor=\"white\", style=\"filled,bold\"];\n")
			legendNodeIDs = append(legendNodeIDs, "legend_sink")
		}
		if usedForwardSink {
			fmt.Fprintf(&buf, "    legend_forward [label=\"ForwardingSink\", shape=hexagon, fillcolor=\"lightsalmon\", style=\"filled\"];\n")
			legendNodeIDs = append(legendNodeIDs, "legend_forward")
		}

		if len(legendNodeIDs) > 1 {
			fmt.Fprintf(&buf, "    %s [style=invis];\n", strings.Join(legendNodeIDs, " -> "))
		}

		fmt.Fprintf(&buf, "  }\n")
	}

	fmt.Fprintf(&buf, "}\n")

	// Write to file with secure permissions (0600)
	if err := os.WriteFile(filepath, []byte(buf.String()), 0600); err != nil {
		return fmt.Errorf("failed to write dot file: %w", err)
	}

	return nil
}

// isForwardingSink checks if a sink is a ForwardingSink by type checking
func isForwardingSink(sink proxy.Sink) bool {
	_, ok := sink.(*stages.ForwardingSink)
	return ok
}

// addCrossPipelineEdges creates edges for cross-pipeline forwarding
func addCrossPipelineEdges(buf *strings.Builder, pipelines []*proxy.Pipeline) {
	// Map interface names to pipeline indices
	ifaceToIndex := make(map[string]int)
	for i, pipeline := range pipelines {
		if iface, ok := sourceInterfaceName(pipeline.Source.Name()); ok {
			ifaceToIndex[iface] = i
		}
	}

	// Look for ForwardingSinks and create edges
	for i, pipeline := range pipelines {
		for j, sink := range pipeline.Sinks {
			if fSink, ok := sink.(*stages.ForwardingSink); ok {
				// Extract destination interface from ForwardingSink
				sinkName := fSink.Name()
				if strings.Contains(sinkName, "(") && strings.Contains(sinkName, ")") {
					start := strings.Index(sinkName, "(") + 1
					end := strings.Index(sinkName, ")")
					if start > 0 && end > start {
						destIface := sinkName[start:end]
						if destIdx, ok := ifaceToIndex[destIface]; ok {
							sinkNodeID := fmt.Sprintf("pipeline_%d_sink_%d", i, j)
							sourceNodeID := fmt.Sprintf("pipeline_%d_source", destIdx)
							fmt.Fprintf(buf, "  %s -> %s [style=dashed, color=red, label=\"forward\"];\n", sinkNodeID, sourceNodeID)
						}
					}
				}
			}
		}
	}
}

func sourceInterfaceName(sourceName string) (string, bool) {
	// Current PcapSource.Name() returns "PcapSource:<iface>".
	if strings.Contains(sourceName, ":") {
		parts := strings.SplitN(sourceName, ":", 2)
		if len(parts) == 2 && parts[1] != "" {
			return parts[1], true
		}
	}

	// Backwards-compatible fallback for "PcapSource(<iface>)".
	if strings.Contains(sourceName, "(") && strings.Contains(sourceName, ")") {
		start := strings.Index(sourceName, "(") + 1
		end := strings.Index(sourceName, ")")
		if start > 0 && end > start {
			return sourceName[start:end], true
		}
	}

	return "", false
}
