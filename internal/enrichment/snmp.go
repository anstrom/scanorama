// Package enrichment provides post-scan host enrichment using SNMP.
package enrichment

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gosnmp/gosnmp"

	"github.com/anstrom/scanorama/internal/db"
)

const (
	snmpPort          = 161
	snmpTimeout       = 5 * time.Second
	snmpCommunityStr  = "public"
	snmpMaxInterfaces = 64
	ifOIDsPerEntry    = 4
	bitsPerMbps       = 1_000_000
	macAddrBytes      = 6
)

// Standard MIB-2 system OIDs.
const (
	oidSysDescr    = ".1.3.6.1.2.1.1.1.0"
	oidSysUptime   = ".1.3.6.1.2.1.1.3.0"
	oidSysContact  = ".1.3.6.1.2.1.1.4.0"
	oidSysName     = ".1.3.6.1.2.1.1.5.0"
	oidSysLocation = ".1.3.6.1.2.1.1.6.0"
)

// Interface table OIDs (IF-MIB).
const (
	oidIfNumber     = ".1.3.6.1.2.1.2.1.0"
	oidIfDescr      = ".1.3.6.1.2.1.2.2.1.2"
	oidIfOperStatus = ".1.3.6.1.2.1.2.2.1.8"
	oidIfSpeed      = ".1.3.6.1.2.1.2.2.1.5"
	oidIfPhysAddr   = ".1.3.6.1.2.1.2.2.1.6"
)

// IF-MIB ifOperStatus values (RFC 2863).
const (
	ifOperStatusUp             = 1
	ifOperStatusDown           = 2
	ifOperStatusTesting        = 3
	ifOperStatusUnknown        = 4
	ifOperStatusDormant        = 5
	ifOperStatusNotPresent     = 6
	ifOperStatusLowerLayerDown = 7
)

var systemOIDs = []string{
	oidSysDescr,
	oidSysUptime,
	oidSysContact,
	oidSysName,
	oidSysLocation,
	oidIfNumber,
}

// sysOIDSetters maps system OIDs to functions that apply PDU values to HostSNMPData.
var sysOIDSetters = map[string]func(*db.HostSNMPData, gosnmp.SnmpPDU){
	oidSysDescr: func(d *db.HostSNMPData, v gosnmp.SnmpPDU) {
		if s, ok := v.Value.(string); ok && s != "" {
			d.SysDescr = &s
		}
	},
	oidSysName: func(d *db.HostSNMPData, v gosnmp.SnmpPDU) {
		if s, ok := v.Value.(string); ok && s != "" {
			d.SysName = &s
		}
	},
	oidSysLocation: func(d *db.HostSNMPData, v gosnmp.SnmpPDU) {
		if s, ok := v.Value.(string); ok && s != "" {
			d.SysLocation = &s
		}
	},
	oidSysContact: func(d *db.HostSNMPData, v gosnmp.SnmpPDU) {
		if s, ok := v.Value.(string); ok && s != "" {
			d.SysContact = &s
		}
	},
	oidSysUptime: func(d *db.HostSNMPData, v gosnmp.SnmpPDU) {
		n := gosnmp.ToBigInt(v.Value).Int64()
		d.SysUptime = &n
	},
	oidIfNumber: func(d *db.HostSNMPData, v gosnmp.SnmpPDU) {
		n := int(gosnmp.ToBigInt(v.Value).Int64())
		d.IfCount = &n
	},
}

// SNMPTarget groups the host details needed for SNMP enrichment.
type SNMPTarget struct {
	HostID    uuid.UUID
	IP        string
	Community string // overrides default "public" when non-empty
}

// SNMPEnricher probes network devices via SNMP and persists results.
type SNMPEnricher struct {
	repo   *db.SNMPRepository
	logger *slog.Logger
}

// NewSNMPEnricher creates a new SNMPEnricher.
func NewSNMPEnricher(repo *db.SNMPRepository, logger *slog.Logger) *SNMPEnricher {
	return &SNMPEnricher{repo: repo, logger: logger}
}

// EnrichHost probes a single host via SNMPv2c and stores the results.
// Returns nil if the host does not respond (non-fatal).
func (e *SNMPEnricher) EnrichHost(ctx context.Context, target SNMPTarget) error {
	community := snmpCommunityStr
	if target.Community != "" {
		community = target.Community
	}

	g := &gosnmp.GoSNMP{
		Target:             target.IP,
		Port:               snmpPort,
		Community:          community,
		Version:            gosnmp.Version2c,
		Timeout:            snmpTimeout,
		Retries:            1,
		ExponentialTimeout: false,
		MaxOids:            gosnmp.MaxOids,
	}

	if err := g.ConnectIPv4(); err != nil {
		// Unreachable host is not an error we want to propagate.
		e.logger.Debug("snmp: connect failed", "ip", target.IP, "err", err)
		return nil
	}
	defer func() {
		if err := g.Conn.Close(); err != nil {
			e.logger.Debug("snmp: close error", "ip", target.IP, "err", err)
		}
	}()

	result, err := g.Get(systemOIDs)
	if err != nil {
		e.logger.Debug("snmp: get failed", "ip", target.IP, "err", err)
		return nil
	}

	data := parseSystemOIDs(result.Variables)

	// Best-effort interface walk — ignore errors.
	ifaces := e.walkInterfaces(g, data)
	data.Interfaces = marshalInterfaces(ifaces)

	data.HostID = target.HostID

	if err := e.repo.UpsertSNMPData(ctx, data); err != nil {
		return fmt.Errorf("snmp enrichment: upsert failed for %s: %w", target.IP, err)
	}

	e.logger.Info("snmp: enriched host", "ip", target.IP, "sys_name", ptrStr(data.SysName))
	return nil
}

// walkInterfaces performs a lightweight walk of the IF-MIB interface table.
func (e *SNMPEnricher) walkInterfaces(g *gosnmp.GoSNMP, data *db.HostSNMPData) []db.SNMPInterface {
	if data.IfCount == nil || *data.IfCount == 0 {
		return nil
	}

	count := *data.IfCount
	if count > snmpMaxInterfaces {
		count = snmpMaxInterfaces
	}

	oids := buildIfOIDs(count)
	result, err := g.Get(oids)
	if err != nil {
		e.logger.Debug("snmp: interface walk failed", "err", err)
		return nil
	}

	byIndex := parseIfPDUs(result.Variables, count)
	return buildIfSlice(byIndex, count)
}

// buildIfOIDs returns per-index OIDs for descr, operStatus, speed, physAddr.
func buildIfOIDs(count int) []string {
	oids := make([]string, 0, count*ifOIDsPerEntry)
	for i := 1; i <= count; i++ {
		oids = append(oids,
			fmt.Sprintf("%s.%d", oidIfDescr, i),
			fmt.Sprintf("%s.%d", oidIfOperStatus, i),
			fmt.Sprintf("%s.%d", oidIfSpeed, i),
			fmt.Sprintf("%s.%d", oidIfPhysAddr, i),
		)
	}
	return oids
}

type ifData struct {
	name   string
	status string
	speed  uint
	mac    string
}

// parseIfPDUs groups SNMP variables by interface index.
func parseIfPDUs(vars []gosnmp.SnmpPDU, count int) map[int]*ifData {
	byIndex := make(map[int]*ifData, count)
	for _, v := range vars {
		idx := indexFromOID(v.Name)
		if idx == 0 {
			continue
		}
		if byIndex[idx] == nil {
			byIndex[idx] = &ifData{}
		}
		applyIfPDU(byIndex[idx], v)
	}
	return byIndex
}

// applyIfPDU applies a single PDU value to the matching ifData field.
func applyIfPDU(entry *ifData, v gosnmp.SnmpPDU) {
	switch {
	case strings.HasPrefix(v.Name, oidIfDescr+"."):
		if s, ok := v.Value.(string); ok {
			entry.name = s
		}
	case strings.HasPrefix(v.Name, oidIfOperStatus+"."):
		switch gosnmp.ToBigInt(v.Value).Int64() {
		case ifOperStatusUp:
			entry.status = "up"
		case ifOperStatusDown:
			entry.status = "down"
		case ifOperStatusTesting:
			entry.status = "testing"
		case ifOperStatusUnknown:
			entry.status = "unknown"
		case ifOperStatusDormant:
			entry.status = "dormant"
		case ifOperStatusNotPresent:
			entry.status = "notPresent"
		case ifOperStatusLowerLayerDown:
			entry.status = "lowerLayerDown"
		default:
			entry.status = "unknown"
		}
	case strings.HasPrefix(v.Name, oidIfSpeed+"."):
		bps := gosnmp.ToBigInt(v.Value).Uint64()
		entry.speed = uint(bps / bitsPerMbps)
	case strings.HasPrefix(v.Name, oidIfPhysAddr+"."):
		if raw, ok := v.Value.(string); ok {
			entry.mac = formatMAC(raw)
		}
	}
}

// buildIfSlice converts the byIndex map to an ordered slice.
func buildIfSlice(byIndex map[int]*ifData, count int) []db.SNMPInterface {
	ifaces := make([]db.SNMPInterface, 0, count)
	for i := 1; i <= count; i++ {
		if d, ok := byIndex[i]; ok {
			ifaces = append(ifaces, db.SNMPInterface{
				Name:   d.name,
				Status: d.status,
				Speed:  d.speed,
				MAC:    d.mac,
			})
		}
	}
	return ifaces
}

// parseSystemOIDs converts SNMP GET variables into HostSNMPData fields.
func parseSystemOIDs(vars []gosnmp.SnmpPDU) *db.HostSNMPData {
	d := &db.HostSNMPData{}
	for _, v := range vars {
		if setter, ok := sysOIDSetters[v.Name]; ok {
			setter(d, v)
		}
	}
	return d
}

// marshalInterfaces serializes interfaces to db.JSONB.
func marshalInterfaces(ifaces []db.SNMPInterface) db.JSONB {
	if len(ifaces) == 0 {
		return db.JSONB("[]")
	}
	b, err := json.Marshal(ifaces)
	if err != nil {
		return db.JSONB("[]")
	}
	return db.JSONB(b)
}

// indexFromOID extracts the trailing integer index from an OID string.
func indexFromOID(oid string) int {
	parts := strings.Split(oid, ".")
	if len(parts) == 0 {
		return 0
	}
	last := parts[len(parts)-1]
	var idx int
	if _, err := fmt.Sscanf(last, "%d", &idx); err != nil {
		return 0
	}
	return idx
}

// formatMAC converts a raw SNMP octet string to xx:xx:xx:xx:xx:xx format.
func formatMAC(raw string) string {
	if len(raw) != macAddrBytes {
		return ""
	}
	return net.HardwareAddr([]byte(raw)).String()
}

// ptrStr safely dereferences a *string.
func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
