package manifest

import (
	"strings"
	"testing"
)

func TestPopupSizeUnmarshalTOML(t *testing.T) {
	tests := []struct {
		name        string
		value       any
		want        PopupSize
		wantErrPart string
	}{
		{name: "cells", value: int64(20), want: PopupSize{Cells: 20}},
		{name: "zero cells", value: int64(0), want: PopupSize{}},
		{name: "largest cell count", value: int64(65535), want: PopupSize{Cells: 65535}},
		{name: "percentage", value: "80%", want: PopupSize{Percent: 80}},
		{name: "smallest percentage", value: "1%", want: PopupSize{Percent: 1}},
		{name: "full percentage", value: "100%", want: PopupSize{Percent: 100}},
		{name: "negative cells", value: int64(-1), wantErrPart: "out of range"},
		{name: "cells above the limit", value: int64(65536), wantErrPart: "out of range"},
		{name: "string without a percent sign", value: "20", wantErrPart: "must be a percentage"},
		{name: "percentage of zero", value: "0%", wantErrPart: "between 1% and 100%"},
		{name: "percentage above 100", value: "101%", wantErrPart: "between 1% and 100%"},
		{name: "percentage not a number", value: "eighty%", wantErrPart: "between 1% and 100%"},
		{name: "boolean", value: true, wantErrPart: "must be an integer"},
		{name: "float", value: 20.5, wantErrPart: "must be an integer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got PopupSize
			err := got.UnmarshalTOML(tt.value)
			if tt.wantErrPart != "" {
				if err == nil {
					t.Fatalf("UnmarshalTOML(%v) succeeded, want an error", tt.value)
				}
				if !strings.Contains(err.Error(), tt.wantErrPart) {
					t.Errorf("error = %q, want it to contain %q", err, tt.wantErrPart)
				}
				return
			}
			if err != nil {
				t.Fatalf("UnmarshalTOML(%v) error = %v", tt.value, err)
			}
			if got != tt.want {
				t.Errorf("size = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPopupSize(t *testing.T) {
	tests := []struct {
		name        string
		size        PopupSize
		wantPercent bool
		wantString  string
	}{
		{name: "cells", size: PopupSize{Cells: 20}, wantString: "20"},
		{name: "zero cells", size: PopupSize{}, wantString: "0"},
		{name: "percentage", size: PopupSize{Percent: 80}, wantPercent: true, wantString: "80%"},
		{name: "full percentage", size: PopupSize{Percent: 100}, wantPercent: true, wantString: "100%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.size.IsPercent(); got != tt.wantPercent {
				t.Errorf("IsPercent() = %v, want %v", got, tt.wantPercent)
			}
			if got := tt.size.String(); got != tt.wantString {
				t.Errorf("String() = %q, want %q", got, tt.wantString)
			}
		})
	}
}
