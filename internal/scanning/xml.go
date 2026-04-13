package scanning

import (
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
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
func SaveResults(result *ScanResult, filePath string) error {
	if result == nil {
		return fmt.Errorf("cannot save nil result")
	}

	// Convert scan result to XML structure
	xmlData := &ScanXML{
		Hosts: make([]HostXML, len(result.Hosts)),
	}

	// Add timing information
	xmlData.StartTime = result.StartTime.Format(time.RFC3339)
	xmlData.EndTime = result.EndTime.Format(time.RFC3339)
	xmlData.Duration = result.Duration.String()

	for i := range result.Hosts {
		host := &result.Hosts[i]
		xmlHost := HostXML{
			Address: host.Address,
			Status:  host.Status,
			Ports:   make([]PortXML, len(host.Ports)),
		}

		for j, port := range host.Ports {
			xmlHost.Ports[j] = PortXML{
				Number:      port.Number,
				Protocol:    port.Protocol,
				State:       port.State,
				Service:     port.Service,
				Version:     port.Version,
				ServiceInfo: port.ServiceInfo,
			}
		}

		xmlData.Hosts[i] = xmlHost
	}

	// Validate and create file
	if err := validateFilePath(filePath); err != nil {
		return &ExecError{Op: "validate path", Err: err}
	}

	file, err := os.Create(filePath) //nolint:gosec // path is validated by validateFilePath
	if err != nil {
		return &ExecError{Op: "create file", Err: err}
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Failed to close file: %v", err)
		}
	}()

	// Create encoder with indentation for readable output
	encoder := xml.NewEncoder(file)
	encoder.Indent("", "  ")

	// Write XML header
	if _, err := file.WriteString(xml.Header); err != nil {
		return &ExecError{Op: "write XML header", Err: err}
	}

	// Encode and write the data
	if err := encoder.Encode(xmlData); err != nil {
		return &ExecError{Op: "encode XML", Err: err}
	}

	return nil
}

// LoadResults reads and parses scan results from an XML file.
// It returns the parsed results or an error if the file cannot be read or parsed.
func LoadResults(filePath string) (*ScanResult, error) {
	// Open and read file
	// Validate and open file
	if err := validateFilePath(filePath); err != nil {
		return nil, &ExecError{Op: "validate path", Err: err}
	}

	file, err := os.Open(filePath) //nolint:gosec // path is validated by validateFilePath
	if err != nil {
		return nil, &ExecError{Op: "open file", Err: err}
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("Failed to close file: %v", err)
		}
	}()

	// Create decoder
	decoder := xml.NewDecoder(file)

	// Decode XML
	var xmlData ScanXML
	if err := decoder.Decode(&xmlData); err != nil {
		return nil, &ExecError{Op: "decode XML", Err: err}
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
			host.Ports[j] = Port{
				Number:      xmlPort.Number,
				Protocol:    xmlPort.Protocol,
				State:       xmlPort.State,
				Service:     xmlPort.Service,
				Version:     xmlPort.Version,
				ServiceInfo: xmlPort.ServiceInfo,
			}
		}

		result.Hosts[i] = host
	}

	return result, nil
}

// validateFilePath validates that the file path is safe to use.
func validateFilePath(path string) error {
	// Check each component of the raw path before cleaning.
	// filepath.Clean removes ".." segments from absolute paths
	// (e.g. "/foo/../etc/passwd" → "/etc/passwd"), so the traversal check must
	// happen on the original string. We test for ".." as a complete path
	// component (not a substring) to avoid rejecting valid filenames that
	// happen to contain ".." (e.g. "scan..results.xml").
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return fmt.Errorf("path contains directory traversal")
		}
	}

	return nil
}
