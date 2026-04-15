// Package enrichment — unit tests for SNMP enrichment helper functions.
package enrichment

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

// ── buildIfOIDs ───────────────────────────────────────────────────────────────

func TestBuildIfOIDs(t *testing.T) {
	oids := buildIfOIDs(2)
	require.Len(t, oids, 14, "count=2 should produce 14 OIDs (7 per interface)")

	// First interface OIDs — order: descr, adminStatus, operStatus, speed, physAddr, inOctets, outOctets.
	assert.Equal(t, fmt.Sprintf("%s.1", oidIfDescr), oids[0])
	assert.Equal(t, fmt.Sprintf("%s.1", oidIfAdminStatus), oids[1])
	assert.Equal(t, fmt.Sprintf("%s.1", oidIfOperStatus), oids[2])
	assert.Equal(t, fmt.Sprintf("%s.1", oidIfSpeed), oids[3])
	assert.Equal(t, fmt.Sprintf("%s.1", oidIfPhysAddr), oids[4])
	assert.Equal(t, fmt.Sprintf("%s.1", oidIfInOctets), oids[5])
	assert.Equal(t, fmt.Sprintf("%s.1", oidIfOutOctets), oids[6])

	// Second interface OIDs.
	assert.Equal(t, fmt.Sprintf("%s.2", oidIfDescr), oids[7])
	assert.Equal(t, fmt.Sprintf("%s.2", oidIfAdminStatus), oids[8])
	assert.Equal(t, fmt.Sprintf("%s.2", oidIfOperStatus), oids[9])
	assert.Equal(t, fmt.Sprintf("%s.2", oidIfSpeed), oids[10])
	assert.Equal(t, fmt.Sprintf("%s.2", oidIfPhysAddr), oids[11])
	assert.Equal(t, fmt.Sprintf("%s.2", oidIfInOctets), oids[12])
	assert.Equal(t, fmt.Sprintf("%s.2", oidIfOutOctets), oids[13])
}

func TestBuildIfOIDs_Zero(t *testing.T) {
	oids := buildIfOIDs(0)
	assert.Empty(t, oids)
}

// ── indexFromOID ──────────────────────────────────────────────────────────────

func TestIndexFromOID(t *testing.T) {
	tests := []struct {
		name     string
		oid      string
		expected int
	}{
		{"simple index 1", ".1.3.6.1.2.1.2.2.1.2.1", 1},
		{"simple index 5", ".1.3.6.1.2.1.2.2.1.2.5", 5},
		{"index 64", ".1.3.6.1.2.1.2.2.1.8.64", 64},
		{"empty OID", "", 0},
		{"trailing non-numeric", ".1.3.6.1.2.1.2.2.1.2.abc", 0},
		{"root dot only", ".", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, indexFromOID(tc.oid))
		})
	}
}

// ── formatMAC ─────────────────────────────────────────────────────────────────

func TestFormatMAC(t *testing.T) {
	t.Run("valid 6-byte raw string", func(t *testing.T) {
		raw := "\x00\x1a\x2b\x3c\x4d\x5e"
		assert.Equal(t, "00:1a:2b:3c:4d:5e", formatMAC(raw))
	})

	t.Run("wrong length returns empty", func(t *testing.T) {
		assert.Equal(t, "", formatMAC("\x00\x1a"))
	})

	t.Run("empty string returns empty", func(t *testing.T) {
		assert.Equal(t, "", formatMAC(""))
	})
}

// ── ptrStr ────────────────────────────────────────────────────────────────────

func TestPtrStr(t *testing.T) {
	t.Run("nil pointer returns empty string", func(t *testing.T) {
		assert.Equal(t, "", ptrStr(nil))
	})

	t.Run("non-nil pointer returns value", func(t *testing.T) {
		s := "hello"
		assert.Equal(t, "hello", ptrStr(&s))
	})
}

// ── parseSystemOIDs ───────────────────────────────────────────────────────────

func TestParseSystemOIDs_AllFields(t *testing.T) {
	vars := []gosnmp.SnmpPDU{
		{Name: oidSysDescr, Value: "Linux router"},
		{Name: oidSysName, Value: "my-router"},
		{Name: oidSysLocation, Value: "Server Room"},
		{Name: oidSysContact, Value: "admin@example.com"},
		{Name: oidSysUptime, Value: uint32(123456)},
		{Name: oidIfNumber, Value: uint32(4)},
	}

	d := parseSystemOIDs(vars)
	require.NotNil(t, d)

	require.NotNil(t, d.SysDescr)
	assert.Equal(t, "Linux router", *d.SysDescr)

	require.NotNil(t, d.SysName)
	assert.Equal(t, "my-router", *d.SysName)

	require.NotNil(t, d.SysLocation)
	assert.Equal(t, "Server Room", *d.SysLocation)

	require.NotNil(t, d.SysContact)
	assert.Equal(t, "admin@example.com", *d.SysContact)

	require.NotNil(t, d.SysUptime)
	assert.Equal(t, int64(123456), *d.SysUptime)

	require.NotNil(t, d.IfCount)
	assert.Equal(t, 4, *d.IfCount)
}

func TestParseSystemOIDs_EmptyString(t *testing.T) {
	// An empty string value should leave the field nil.
	vars := []gosnmp.SnmpPDU{
		{Name: oidSysName, Value: ""},
	}
	d := parseSystemOIDs(vars)
	assert.Nil(t, d.SysName, "empty string should not set the field")
}

func TestParseSystemOIDs_UnknownOID(t *testing.T) {
	vars := []gosnmp.SnmpPDU{
		{Name: ".9.9.9.9.9", Value: "ignored"},
	}
	// Should not panic and should return an empty struct.
	d := parseSystemOIDs(vars)
	require.NotNil(t, d)
	assert.Nil(t, d.SysName)
	assert.Nil(t, d.SysDescr)
}

// ── applyIfPDU ────────────────────────────────────────────────────────────────

func TestApplyIfPDU_Descr(t *testing.T) {
	entry := &ifData{}
	pdu := gosnmp.SnmpPDU{Name: oidIfDescr + ".1", Value: "eth0"}
	applyIfPDU(entry, pdu)
	assert.Equal(t, "eth0", entry.name)
}

func TestApplyIfPDU_OperStatus_Up(t *testing.T) {
	entry := &ifData{}
	pdu := gosnmp.SnmpPDU{Name: oidIfOperStatus + ".1", Value: uint32(1)}
	applyIfPDU(entry, pdu)
	assert.Equal(t, "up", entry.status)
}

func TestApplyIfPDU_OperStatus_Down(t *testing.T) {
	entry := &ifData{}
	pdu := gosnmp.SnmpPDU{Name: oidIfOperStatus + ".1", Value: uint32(2)}
	applyIfPDU(entry, pdu)
	assert.Equal(t, "down", entry.status)
}

func TestApplyIfPDU_Speed(t *testing.T) {
	entry := &ifData{}
	// 1 Gbps = 1_000_000_000 bps → 1000 Mbps
	pdu := gosnmp.SnmpPDU{Name: oidIfSpeed + ".1", Value: uint64(1_000_000_000)}
	applyIfPDU(entry, pdu)
	assert.Equal(t, uint(1000), entry.speed)
}

func TestApplyIfPDU_PhysAddr(t *testing.T) {
	entry := &ifData{}
	raw := "\x00\x1a\x2b\x3c\x4d\x5e"
	pdu := gosnmp.SnmpPDU{Name: oidIfPhysAddr + ".1", Value: raw}
	applyIfPDU(entry, pdu)
	assert.Equal(t, "00:1a:2b:3c:4d:5e", entry.mac)
}

func TestApplyIfPDU_AdminStatus_Up(t *testing.T) {
	entry := &ifData{}
	pdu := gosnmp.SnmpPDU{Name: oidIfAdminStatus + ".1", Value: uint32(1)}
	applyIfPDU(entry, pdu)
	assert.Equal(t, "up", entry.adminStatus)
}

func TestApplyIfPDU_AdminStatus_Down(t *testing.T) {
	entry := &ifData{}
	pdu := gosnmp.SnmpPDU{Name: oidIfAdminStatus + ".1", Value: uint32(2)}
	applyIfPDU(entry, pdu)
	assert.Equal(t, "down", entry.adminStatus)
}

func TestApplyIfPDU_RxBytes(t *testing.T) {
	entry := &ifData{}
	pdu := gosnmp.SnmpPDU{Name: oidIfInOctets + ".1", Value: uint32(1_000_000)}
	applyIfPDU(entry, pdu)
	assert.Equal(t, uint64(1_000_000), entry.rxBytes)
}

func TestApplyIfPDU_TxBytes(t *testing.T) {
	entry := &ifData{}
	pdu := gosnmp.SnmpPDU{Name: oidIfOutOctets + ".1", Value: uint32(2_000_000)}
	applyIfPDU(entry, pdu)
	assert.Equal(t, uint64(2_000_000), entry.txBytes)
}

func TestApplyIfPDU_Unknown(t *testing.T) {
	entry := &ifData{}
	pdu := gosnmp.SnmpPDU{Name: ".9.9.9.9.1", Value: "whatever"}
	applyIfPDU(entry, pdu)
	// No field should be set.
	assert.Empty(t, entry.name)
	assert.Empty(t, entry.adminStatus)
	assert.Empty(t, entry.status)
	assert.Zero(t, entry.speed)
	assert.Empty(t, entry.mac)
	assert.Zero(t, entry.rxBytes)
	assert.Zero(t, entry.txBytes)
}

// ── parseIfPDUs ───────────────────────────────────────────────────────────────

func TestParseIfPDUs(t *testing.T) {
	vars := []gosnmp.SnmpPDU{
		{Name: oidIfDescr + ".1", Value: "eth0"},
		{Name: oidIfOperStatus + ".1", Value: uint32(1)},
		{Name: oidIfSpeed + ".1", Value: uint64(1_000_000_000)},
		{Name: oidIfPhysAddr + ".1", Value: "\xaa\xbb\xcc\xdd\xee\xff"},
		{Name: oidIfDescr + ".2", Value: "lo"},
		{Name: oidIfOperStatus + ".2", Value: uint32(1)},
	}

	byIndex := parseIfPDUs(vars, 2)
	require.Len(t, byIndex, 2)

	iface1 := byIndex[1]
	require.NotNil(t, iface1)
	assert.Equal(t, "eth0", iface1.name)
	assert.Equal(t, "up", iface1.status)
	assert.Equal(t, uint(1000), iface1.speed)
	assert.Equal(t, "aa:bb:cc:dd:ee:ff", iface1.mac)

	iface2 := byIndex[2]
	require.NotNil(t, iface2)
	assert.Equal(t, "lo", iface2.name)
	assert.Equal(t, "up", iface2.status)
}

func TestParseIfPDUs_ZeroIndexSkipped(t *testing.T) {
	// A PDU whose OID ends in a non-numeric segment should be skipped (index==0).
	vars := []gosnmp.SnmpPDU{
		{Name: ".1.3.6.1.2.1.2.2.1.2.abc", Value: "orphan"}, // non-numeric → index 0
		{Name: oidIfDescr + ".1", Value: "eth0"},
	}
	byIndex := parseIfPDUs(vars, 2)
	assert.Len(t, byIndex, 1)
	assert.NotNil(t, byIndex[1])
}

// ── buildIfSlice ──────────────────────────────────────────────────────────────

func TestBuildIfSlice_Ordered(t *testing.T) {
	// Sparse map: only indices 1 and 3 present out of count=3.
	byIndex := map[int]*ifData{
		1: {
			name: "eth0", adminStatus: "up", status: "up",
			speed: 1000, mac: "aa:bb:cc:dd:ee:ff", rxBytes: 500, txBytes: 250,
		},
		3: {name: "eth2", adminStatus: "up", status: "down", speed: 100},
	}

	ifaces := buildIfSlice(byIndex, 3)
	require.Len(t, ifaces, 2)
	assert.Equal(t, "eth0", ifaces[0].Name)
	assert.Equal(t, "up", ifaces[0].AdminStatus)
	assert.Equal(t, "up", ifaces[0].Status)
	assert.Equal(t, uint64(500), ifaces[0].RxBytes)
	assert.Equal(t, uint64(250), ifaces[0].TxBytes)
	assert.Equal(t, "eth2", ifaces[1].Name)
	assert.Equal(t, "down", ifaces[1].Status)
}

func TestBuildIfSlice_Empty(t *testing.T) {
	ifaces := buildIfSlice(map[int]*ifData{}, 3)
	assert.Empty(t, ifaces)
}

// ── marshalInterfaces ─────────────────────────────────────────────────────────

func TestMarshalInterfaces_Empty(t *testing.T) {
	result := marshalInterfaces(nil)
	assert.Equal(t, db.JSONB("[]"), result)
}

func TestMarshalInterfaces_WithData(t *testing.T) {
	ifaces := []db.SNMPInterface{
		{Name: "eth0", Status: "up", Speed: 1000, MAC: "00:1a:2b:3c:4d:5e"},
	}

	result := marshalInterfaces(ifaces)
	require.NotEmpty(t, result)

	// Parse the returned JSON and verify structure.
	var parsed []map[string]interface{}
	err := json.Unmarshal([]byte(result), &parsed)
	require.NoError(t, err, "result must be valid JSON")
	require.Len(t, parsed, 1)

	entry := parsed[0]
	assert.Equal(t, "eth0", entry["name"])
	assert.Equal(t, "up", entry["status"])
	assert.Equal(t, float64(1000), entry["speed_mbps"])
	assert.Equal(t, "00:1a:2b:3c:4d:5e", entry["mac"])
}

func TestMarshalInterfaces_MultipleEntries(t *testing.T) {
	ifaces := []db.SNMPInterface{
		{Name: "eth0", Status: "up", Speed: 1000},
		{Name: "lo", Status: "up", Speed: 0},
	}

	result := marshalInterfaces(ifaces)

	var parsed []map[string]interface{}
	err := json.Unmarshal([]byte(result), &parsed)
	require.NoError(t, err)
	assert.Len(t, parsed, 2)
	assert.Equal(t, "eth0", parsed[0]["name"])
	assert.Equal(t, "lo", parsed[1]["name"])
}

// ── ifStatusString ────────────────────────────────────────────────────────────

func TestIfStatusString(t *testing.T) {
	cases := []struct {
		code     int64
		expected string
	}{
		{ifOperStatusUp, "up"},
		{ifOperStatusDown, "down"},
		{ifOperStatusTesting, "testing"},
		{ifOperStatusUnknown, "unknown"},
		{ifOperStatusDormant, "dormant"},
		{ifOperStatusNotPresent, "notPresent"},
		{ifOperStatusLowerLayerDown, "lowerLayerDown"},
		{99, "unknown"}, // default case
	}
	for _, tc := range cases {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, ifStatusString(tc.code))
		})
	}
}
