package internal

import (
	"encoding/xml"
	"fmt"
	"os"
)

// ScanXML represents the root XML element for scan results
type ScanXML struct {
	XMLName xml.Name  `xml:"scanresult"`
	Hosts   []HostXML `xml:"host"`
}

// HostXML represents a host in XML format
type HostXML struct {
	Address string    `xml:"Address"`
	Status  string    `xml:"Status"`
	Ports   []PortXML `xml:"Ports,omitempty"`
}

// PortXML represents a port in XML format
type PortXML struct {
	Number      uint16 `xml:"Number"`
	Protocol    string `xml:"Protocol"`
	State       string `xml:"State"`
	Service     string `xml:"Service"`
	Version     string `xml:"Version,omitempty"`
	ServiceInfo string `xml:"ServiceInfo,omitempty"`
}

// SaveResults saves scan results to an XML file
func SaveResults(result *ScanResult, filepath string) error {
	if result == nil {
		return fmt.Errorf("cannot save nil result")
	}

	// Convert scan result to XML structure
	xmlData := &ScanXML{
		Hosts: make([]HostXML, len(result.Hosts)),
	}

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
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	// Create encoder with indentation for readable output
	encoder := xml.NewEncoder(file)
	encoder.Indent("", "  ")

	// Write XML header
	if _, err := file.Write([]byte(xml.Header)); err != nil {
		return fmt.Errorf("failed to write XML header: %v", err)
	}

	// Encode and write the data
	if err := encoder.Encode(xmlData); err != nil {
		return fmt.Errorf("failed to encode XML: %v", err)
	}

	return nil
}

// LoadResults loads scan results from an XML file
func LoadResults(filepath string) (*ScanResult, error) {
	// Open and read file
	file, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Create decoder
	decoder := xml.NewDecoder(file)

	// Decode XML
	var xmlData ScanXML
	if err := decoder.Decode(&xmlData); err != nil {
		return nil, fmt.Errorf("failed to decode XML: %v", err)
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
