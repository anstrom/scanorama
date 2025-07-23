package internal

import (
	"encoding/xml"
	"fmt"
	"os"
	"time"
)

// ScanXML is the root element for XML serialization of scan results.
type ScanXML struct {
	XMLName   xml.Name  `xml:"scanresult"`
	Hosts     []HostXML `xml:"host"`
	StartTime string    `xml:"start_time,attr"`
	EndTime   string    `xml:"end_time,attr"`
	Duration  string    `xml:"duration,attr"`
}

// HostXML represents a scanned host for XML serialization.
// It contains the host's address, status, and discovered ports.
type HostXML struct {
	Address string    `xml:"Address"`
	Status  string    `xml:"Status"`
	Ports   []PortXML `xml:"Ports,omitempty"`
}

// PortXML represents a scanned port for XML serialization.
// It includes the port number, protocol, state, and service information.
type PortXML struct {
	Number      uint16 `xml:"Number"`
	Protocol    string `xml:"Protocol"`
	State       string `xml:"State"`
	Service     string `xml:"Service"`
	Version     string `xml:"Version,omitempty"`
	ServiceInfo string `xml:"ServiceInfo,omitempty"`
}

// SaveResults writes scan results to an XML file at the specified path.
// The output is formatted with proper indentation for readability.
func SaveResults(result *ScanResult, filepath string) error {
	if result == nil {
		return fmt.Errorf("cannot save nil result")
	}

	if result == nil {
		return &ScanError{Op: "save results", Err: fmt.Errorf("nil result")}
	}

	// Convert scan result to XML structure
	xmlData := &ScanXML{
		Hosts: make([]HostXML, len(result.Hosts)),
	}

	// Add timing information
	xmlData.StartTime = result.StartTime.Format(time.RFC3339)
	xmlData.EndTime = result.EndTime.Format(time.RFC3339)
	xmlData.Duration = result.Duration.String()

	for i, host := range result.Hosts {
		xmlHost := HostXML{
			Address: host.Address,
			Status:  host.Status,
			Ports:   make([]PortXML, len(host.Ports)),
		}

		for j, port := range host.Ports {
			xmlHost.Ports[j] = PortXML(port)
		}

		xmlData.Hosts[i] = xmlHost
	}

	// Create file
	file, err := os.Create(filepath)
	if err != nil {
		return &ScanError{Op: "create file", Err: err}
	}
	defer file.Close()

	// Create encoder with indentation for readable output
	encoder := xml.NewEncoder(file)
	encoder.Indent("", "  ")

	// Write XML header
	if _, err := file.Write([]byte(xml.Header)); err != nil {
		return &ScanError{Op: "write XML header", Err: err}
	}

	// Encode and write the data
	if err := encoder.Encode(xmlData); err != nil {
		return &ScanError{Op: "encode XML", Err: err}
	}

	return nil
}

// LoadResults reads and parses scan results from an XML file.
// It returns the parsed results or an error if the file cannot be read or parsed.
func LoadResults(filepath string) (*ScanResult, error) {
	// Open and read file
	file, err := os.Open(filepath)
	if err != nil {
		return nil, &ScanError{Op: "open file", Err: err}
	}
	defer file.Close()

	// Create decoder
	decoder := xml.NewDecoder(file)

	// Decode XML
	var xmlData ScanXML
	if err := decoder.Decode(&xmlData); err != nil {
		return nil, &ScanError{Op: "decode XML", Err: err}
	}

	// Convert to scan result
	result := &ScanResult{
		Hosts: make([]Host, len(xmlData.Hosts)),
	}

	for i, xmlHost := range xmlData.Hosts {
		host := Host{
			Address: xmlHost.Address,
			Status:  xmlHost.Status,
			Ports:   make([]Port, len(xmlHost.Ports)),
		}

		for j, xmlPort := range xmlHost.Ports {
			host.Ports[j] = Port(xmlPort)
		}

		result.Hosts[i] = host
	}

	return result, nil
}
