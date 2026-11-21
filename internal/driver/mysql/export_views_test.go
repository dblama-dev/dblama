package mysql

import (
 "testing"
 "time"
)
func TestExportSQLLiteralKeepsFractionalTimestamp(t *testing.T){
 timestamp:="2026-10-28T10:11:12.123456Z"
 if got:=sqlLiteral(timestamp,"datetime");got!="'2026-10-28 10:11:12.123456'"{t.Fatalf("fractional datetime: %s",got)}
 value:=time.Date(2026,10,28,10,11,12,123456000,time.UTC)
 if got:=sqlLiteral(value,"timestamp");got!="'2026-10-28 10:11:12.123456'"{t.Fatalf("fractional timestamp: %s",got)}
}
