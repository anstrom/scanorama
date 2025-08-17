// Package scanning provides core network scanning functionality for Scanorama.
//
// This package contains the essential scanning engine that powers Scanorama's
// network discovery and port scanning capabilities. It handles scan execution,
// result processing, XML parsing, and provides the core data structures used
// throughout the application.
//
// # Overview
//
// The scanning package is built around the ScanConfig structure which defines
// scan parameters, and the ScanResult structure which contains scan outcomes.
// The main entry points are RunScanWithContext and RunScanWithDB functions
// that execute scans with different database integration levels.
//
// # Main Components
//
// ## Scan Execution
//
// The core scanning functionality is provided through:
//   - ScanConfig: Configuration structure defining scan parameters
//   - RunScanWithContext: Execute scans with context and optional database
//   - RunScanWithDB: Execute scans with database integration
//   - PrintResults: Display scan results in human-readable format
//
// ## Result Processing
//
// Scan results are processed and structured through:
//   - ScanResult: Main result structure containing discovered hosts and services
//   - Host: Individual host information including open ports and services
//   - Port: Port-specific information including state and service details
//   - Service: Service identification and version information
//
// ## XML Processing
//
// Nmap XML output parsing is handled by:
//   - NmapRun: Root structure for parsing nmap XML output
//   - NmapHost: Host-specific data from nmap scans
//   - NmapPort: Port-specific data from nmap scans
//   - ParseNmapXML: Parse nmap XML output into structured data
//
// # Usage Examples
//
// ## Basic Scan
//
//	config := &scanning.ScanConfig{
//		Targets:    []string{"192.168.1.1/24"},
//		Ports:      "80,443,22",
//		ScanType:   "syn",
//		Timeout:    300,
//		Concurrency: 10,
//	}
//
//	ctx := context.Background()
//	result, err := scanning.RunScanWithContext(ctx, config, nil)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	scanning.PrintResults(result)
//
// ## Database-Integrated Scan
//
//	db, err := db.NewDB(dbConfig)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	config := &scanning.ScanConfig{
//		Targets:    []string{"10.0.0.0/8"},
//		Ports:      "1-1000",
//		ScanType:   "connect",
//		Timeout:    600,
//		Concurrency: 50,
//	}
//
//	result, err := scanning.RunScanWithDB(config, db)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Results are automatically stored in the database
//
// ## XML Parsing
//
//	xmlData, err := os.ReadFile("scan_results.xml")
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	nmapResult, err := scanning.ParseNmapXML(xmlData)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Process structured nmap data
//	for _, host := range nmapResult.Hosts {
//		fmt.Printf("Host: %s\n", host.Address.Addr)
//		for _, port := range host.Ports.Ports {
//			if port.State.State == "open" {
//				fmt.Printf("  Port %d/%s: %s\n",
//					port.PortID, port.Protocol, port.State.State)
//			}
//		}
//	}
//
// # Configuration Options
//
// ScanConfig supports various scanning modes and options:
//
//   - Targets: IP addresses, ranges, or hostnames to scan
//   - Ports: Port specifications (ranges, lists, or named services)
//   - ScanType: Scan technique (syn, connect, udp, etc.)
//   - Timeout: Maximum scan duration in seconds
//   - Concurrency: Number of parallel scanning threads
//   - Rate: Packets per second limit
//   - HostTimeout: Per-host timeout in seconds
//
// # Scan Types
//
// Supported scan types include:
//   - "syn": TCP SYN scan (default, requires privileges)
//   - "connect": TCP connect scan (no privileges required)
//   - "udp": UDP scan
//   - "ping": Host discovery only
//   - "ack": TCP ACK scan
//   - "window": TCP Window scan
//   - "maimon": TCP Maimon scan
//
// # Error Handling
//
// All functions return descriptive errors that can be used for:
//   - User feedback in CLI applications
//   - API error responses in web services
//   - Logging and monitoring in daemon processes
//
// Common error scenarios include:
//   - Invalid target specifications
//   - Network connectivity issues
//   - Permission errors (for privileged scans)
//   - Timeout errors for long-running scans
//   - Database connection failures (for DB-integrated scans)
//
// # Thread Safety
//
// The scanning package is designed to be thread-safe:
//   - Multiple scans can run concurrently
//   - ScanConfig and result structures are immutable after creation
//   - Database operations use connection pooling
//   - Context cancellation is properly handled
//
// # Performance Considerations
//
// For optimal performance:
//   - Use appropriate concurrency levels based on target network capacity
//   - Set reasonable timeouts to avoid hanging scans
//   - Consider rate limiting for large networks
//   - Use database integration for persistent storage of large result sets
//   - Monitor memory usage for very large scans
//
// # Integration
//
// This package integrates with other Scanorama components:
//   - internal/db: Database storage and retrieval of scan results
//   - internal/discovery: Network discovery and host enumeration
//   - internal/daemon: Background scanning services
//   - internal/api: REST API endpoints for scan management
//   - internal/scheduler: Automated scanning workflows
package scanning
