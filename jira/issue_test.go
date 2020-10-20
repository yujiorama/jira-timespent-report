package jira

import (
	"reflect"
	"testing"
)

func TestToRecord(t *testing.T) {
	field := IssueField{
		Summary:                       "サマリ",
		Timespent:                     3600,
		Timeoriginalestimate:          3600,
		Aggregatetimespent:            3600,
		Aggregatetimeoriginalestimate: 3600,
		Status: Status{
			Name:        "close",
			Description: "description",
		},
	}

	expected := []string{"サマリ"}
	actual := field.ToRecord([]string{"summary"})

	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("expected=[%v] <> actual[%v]\n", expected, actual)
	}

	TimeUnit = "hh"
	expected = []string{"サマリ", "1.00"}
	actual = field.ToRecord([]string{"summary", "timespent"})

	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("expected=[%v] <> actual[%v]\n", expected, actual)
	}
}
