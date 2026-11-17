package app

import (
	"reflect"
	"testing"
)

func TestParseRecords(t *testing.T) {
	t.Parallel()

	data := `{"records": [
		{"host": " My-App.LH. ", "target": "172.17.0.2", "labels": {"pier.upstream": "3000"}},
		{"host": "dev.lh", "target": "host.docker.internal"}
	]}`

	got, err := parseRecords([]byte(data))
	if err != nil {
		t.Fatal(err)
	}

	want := []Record{
		{Host: "my-app.lh", Target: "172.17.0.2", Labels: map[string]string{"pier.upstream": "3000"}},
		{Host: "dev.lh", Target: "host.docker.internal"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseRecords_Errors(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"invalid JSON":  `{"records": [`,
		"empty host":    `{"records": [{"host": "", "target": "1.2.3.4"}]}`,
		"empty target":  `{"records": [{"host": "a.lh", "target": " "}]}`,
		"ip not target": `{"records": [{"host": "a.lh", "ip": "1.2.3.4"}]}`,
	}

	for name, data := range cases {
		if _, err := parseRecords([]byte(data)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestParseRecords_Empty(t *testing.T) {
	t.Parallel()

	got, err := parseRecords([]byte(`{"records": []}`))
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v", got, err)
	}
}
