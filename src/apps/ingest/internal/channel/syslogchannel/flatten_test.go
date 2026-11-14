package syslogchannel

import (
	"testing"
)

// RFC5424 信封：PRI、时间、主机、应用与结构化数据都要摊平。
func TestFlattenRFC5424(t *testing.T) {
	line := `<34>1 2026-11-14T10:00:00Z fw01 sshd 1234 ID47 [exampleSDID@32473 iut="3"] login failed`

	fields, err := flatten([]byte(line))
	if err != nil {
		t.Fatal(err)
	}

	if fields["syslog.version"] != "rfc5424" {
		t.Fatalf("version = %#v", fields["syslog.version"])
	}
	if fields["syslog.hostname"] != "fw01" {
		t.Fatalf("hostname = %#v", fields["syslog.hostname"])
	}
	if fields["syslog.appname"] != "sshd" {
		t.Fatalf("appname = %#v", fields["syslog.appname"])
	}
	if fields["message"] != "login failed" {
		t.Fatalf("message = %#v", fields["message"])
	}
	if fields["syslog.sd.exampleSDID@32473.iut"] != "3" {
		t.Fatalf("structured data = %#v", fields)
	}
	if fields["syslog.facility"] != 4 || fields["syslog.severity"] != 2 {
		t.Fatalf("pri split = %#v %#v", fields["syslog.facility"], fields["syslog.severity"])
	}
}

// RFC3164 信封：传统时间戳格式，没有版本号。
func TestFlattenRFC3164(t *testing.T) {
	line := "<34>Nov 14 10:00:00 fw01 sshd[1234]: login failed"

	fields, err := flatten([]byte(line))
	if err != nil {
		t.Fatal(err)
	}

	if fields["syslog.version"] != "rfc3164" {
		t.Fatalf("version = %#v", fields["syslog.version"])
	}
	if fields["syslog.hostname"] != "fw01" {
		t.Fatalf("hostname = %#v", fields["syslog.hostname"])
	}
}

// CEF 塞在 syslog 载荷里：信封与 CEF 字段都要在，规则可直接映射 src/dst。
func TestFlattenDetectsCEFPayload(t *testing.T) {
	line := `<34>1 2026-11-14T10:00:00Z fw01 - - - - CEF:0|Fortinet|FortiGate|7.4|100|Web Attack|5|src=1.2.3.4 act=deny`

	fields, err := flatten([]byte(line))
	if err != nil {
		t.Fatal(err)
	}

	if fields["cef.src"] != "1.2.3.4" {
		t.Fatalf("CEF fields missing: %#v", fields)
	}
	if fields["cef.DeviceVendor"] != "Fortinet" {
		t.Fatalf("CEF header missing: %#v", fields)
	}
	if fields["syslog.hostname"] != "fw01" {
		t.Fatalf("envelope missing: %#v", fields)
	}
}

// 不是 syslog 的报文返回错误，由调用方走原文透传。
func TestFlattenRejectsNonSyslog(t *testing.T) {
	if _, err := flatten([]byte("plain text, not syslog")); err == nil {
		t.Fatal("non-syslog accepted")
	}
}
