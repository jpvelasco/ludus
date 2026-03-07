package buildgraph

import "encoding/xml"

// BuildGraph represents a UE5 BuildGraph XML document.
type BuildGraph struct {
	XMLName    xml.Name    `xml:"BuildGraph"`
	Options    []Option    `xml:"Option"`
	Properties []Property  `xml:"Property"`
	Agents     []Agent     `xml:"Agent"`
	Aggregates []Aggregate `xml:"Aggregate"`
}

// Option defines a BuildGraph option with a name, default value, and description.
type Option struct {
	Name         string `xml:"Name,attr"`
	DefaultValue string `xml:"DefaultValue,attr"`
	Description  string `xml:"Description,attr"`
}

// Property defines a BuildGraph property with a name and value.
type Property struct {
	Name  string `xml:"Name,attr"`
	Value string `xml:"Value,attr"`
}

// Agent groups related build nodes with a name and optional type constraint.
type Agent struct {
	Name  string `xml:"Name,attr"`
	Type  string `xml:"Type,attr,omitempty"`
	Nodes []Node `xml:"Node"`
}

// Node is a single build step within an agent.
type Node struct {
	Name     string  `xml:"Name,attr"`
	Requires string  `xml:"Requires,attr,omitempty"`
	Steps    []Spawn `xml:"Spawn"`
}

// Spawn executes a command within a node.
type Spawn struct {
	Exe        string `xml:"Exe,attr"`
	Arguments  string `xml:"Arguments,attr,omitempty"`
	WorkingDir string `xml:"WorkingDir,attr,omitempty"`
}

// Aggregate defines a named collection of node requirements.
type Aggregate struct {
	Name     string `xml:"Name,attr"`
	Requires string `xml:"Requires,attr"`
}

// Marshal serializes the BuildGraph to indented XML with an XML header.
func (bg *BuildGraph) Marshal() ([]byte, error) {
	output, err := xml.MarshalIndent(bg, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), output...), nil
}
