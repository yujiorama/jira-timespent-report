package jira

import (
	"testing"
	"time"
)

func TestConfig_dateCondition(t *testing.T) {
	type fields struct {
		Worklog         bool
		TargetYearMonth string
		clock           func() time.Time
	}
	tests := []struct {
		name   string
		fields fields
		want   string
		want1  bool
	}{
		{
			name: "2020-12 to 2020-11",
			fields: fields{
				Worklog:         false,
				TargetYearMonth: "2020-12",
				clock: func() time.Time {
					t, _ := time.Parse("2006-01-02", "2020-11-01")
					return t
				},
			},
			want:  "",
			want1: false,
		},
		{
			name: "2021-01 to 2020-12",
			fields: fields{
				Worklog:         false,
				TargetYearMonth: "2021-01",
				clock: func() time.Time {
					t, _ := time.Parse("2006-01-02", "2020-12-01")
					return t
				},
			},
			want:  "",
			want1: false,
		},
		{
			name: "2020-11 to 2020-12",
			fields: fields{
				Worklog:         false,
				TargetYearMonth: "2020-11",
				clock: func() time.Time {
					t, _ := time.Parse("2006-01-02", "2020-12-01")
					return t
				},
			},
			want:  "updated >= startOfMonth(-1) AND updated <= endOfMonth(-1)",
			want1: true,
		},
		{
			name: "2020-11 to 2021-01",
			fields: fields{
				Worklog:         false,
				TargetYearMonth: "2020-11",
				clock: func() time.Time {
					t, _ := time.Parse("2006-01-02", "2021-01-01")
					return t
				},
			},
			want:  "updated >= startOfMonth(-2) AND updated <= endOfMonth(-2)",
			want1: true,
		},
		{
			name: "2020-12 to 2021-01",
			fields: fields{
				Worklog:         false,
				TargetYearMonth: "2020-12",
				clock: func() time.Time {
					t, _ := time.Parse("2006-01-02", "2021-01-01")
					return t
				},
			},
			want:  "updated >= startOfMonth(-1) AND updated <= endOfMonth(-1)",
			want1: true,
		},
		{
			name: "2020-12 to 2022-01",
			fields: fields{
				Worklog:         false,
				TargetYearMonth: "2020-12",
				clock: func() time.Time {
					t, _ := time.Parse("2006-01-02", "2022-01-01")
					return t
				},
			},
			want:  "updated >= startOfMonth(-13) AND updated <= endOfMonth(-13)",
			want1: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{
				Worklog:         tt.fields.Worklog,
				TargetYearMonth: tt.fields.TargetYearMonth,
				clock:           tt.fields.clock,
			}
			got, got1 := c.dateCondition()
			if got1 != tt.want1 {
				t.Errorf("dateCondition() got1 = %v, want %v", got1, tt.want1)
			}
			if got != tt.want {
				t.Errorf("dateCondition() got = %v, want %v", got, tt.want)
			}
		})
	}
}
