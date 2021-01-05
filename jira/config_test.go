package jira

import (
	"testing"
	"time"
)

func TestConfig_dateCondition(t *testing.T) {
	type fields struct {
		Worklog         bool
		TargetYearMonth string
	}
	tests := []struct {
		name       string
		fields     fields
		actualTime time.Time
		want       string
		want1      bool
	}{
		// TODO: Add test cases.
		{
			name: "success",
			fields: fields{
				Worklog:         false,
				TargetYearMonth: "2020-12",
			},
			actualTime: time.Now(),
			want:       "updated >= startOfMonth(-1) AND updated <= endOfMonth(-1)",
			want1:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{
				Worklog:         tt.fields.Worklog,
				TargetYearMonth: tt.fields.TargetYearMonth,
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
