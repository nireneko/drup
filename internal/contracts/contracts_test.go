package contracts

import (
	"strings"
	"testing"
)

func TestDecodeDispatchRejectsUnknownEnumAndField(t *testing.T) {
	valid := []byte(`{"schema_version":"v1","identity":{"root":"/project","candidate":"abc123","run_id":"run-1","phase":"rector"},"agent":"drup-rector","scope":"custom","payload":{}}`)
	if _, err := DecodeDispatch(valid); err != nil {
		t.Fatalf("DecodeDispatch(valid) error = %v", err)
	}

	for name, tc := range map[string]struct{ raw, want string }{
		"unknown phase": {`{"schema_version":"v1","identity":{"root":"/project","candidate":"abc123","run_id":"run-1","phase":"jump"},"agent":"drup-rector","scope":"custom","payload":{}}`, "phase"},
		"unknown field": {`{"schema_version":"v1","identity":{"root":"/project","candidate":"abc123","run_id":"run-1","phase":"rector"},"agent":"drup-rector","scope":"custom","payload":{},"unsafe":true}`, "unknown field"},
		"wrong version": {`{"schema_version":"v2","identity":{"root":"/project","candidate":"abc123","run_id":"run-1","phase":"rector"},"agent":"drup-rector","scope":"custom","payload":{}}`, "schema_version"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := DecodeDispatch([]byte(tc.raw))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("DecodeDispatch() error = %v, want actionable %q error", err, tc.want)
			}
		})
	}
}

func TestDecodeAgentReportBindsDispatchAndRejectsLegacyStatuses(t *testing.T) {
	dispatch, err := DecodeDispatch([]byte(`{"schema_version":"v1","identity":{"root":"/project","candidate":"abc123","run_id":"run-1","phase":"rector"},"agent":"drup-rector","scope":"custom","payload":{}}`))
	if err != nil {
		t.Fatal(err)
	}

	report := []byte(`{"schema_version":"v1","identity":{"root":"/project","candidate":"abc123","run_id":"run-1","phase":"rector"},"agent":"drup-rector","status":"pass","summary":"completed","artifacts":[],"evidence":{},"risks":[]}`)
	if _, err := DecodeAgentReport(dispatch, report); err != nil {
		t.Fatalf("DecodeAgentReport() error = %v", err)
	}
	_, err = DecodeAgentReport(dispatch, []byte(`{"schema_version":"v1","identity":{"root":"/other","candidate":"abc123","run_id":"run-1","phase":"rector"},"agent":"drup-rector","status":"completed","summary":"completed","artifacts":[],"evidence":{},"risks":[]}`))
	if err == nil || (!strings.Contains(err.Error(), "/status") && !strings.Contains(err.Error(), "/identity/root")) {
		t.Fatalf("DecodeAgentReport() error = %v, want identity or enum diagnostic", err)
	}
}

func TestEvidenceContractsRejectUnknownFields(t *testing.T) {
	for name, tc := range map[string]struct {
		decode func([]byte) error
		raw    string
	}{
		"validation": {func(raw []byte) error { _, err := DecodeValidationEvidence(raw); return err }, `{"schema_version":"v1","identity":{"root":"/project","candidate":"abc123","run_id":"run-1","phase":"rector"},"checks":[{"name":"scan","status":"pass"}],"unexpected":true}`},
		"checkpoint": {func(raw []byte) error { _, err := DecodeCheckpointEvidence(raw); return err }, `{"schema_version":"v1","identity":{"root":"/project","candidate":"abc123","run_id":"run-1","phase":"backup"},"checkpoint":"backup","status":"pass","unexpected":true}`},
	} {
		t.Run(name, func(t *testing.T) {
			if err := tc.decode([]byte(tc.raw)); err == nil || !strings.Contains(err.Error(), "unknown field") {
				t.Fatalf("decoder error = %v, want unknown field", err)
			}
		})
	}
}
