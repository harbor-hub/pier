package tcp

import "testing"

func TestParseMapping(t *testing.T) {
	t.Parallel()

	cases := map[string]Mapping{
		"3306=db:3306":               {Listen: ":3306", Target: "db:3306"},
		"127.0.0.1:3306=db:3306":     {Listen: "127.0.0.1:3306", Target: "db:3306"},
		"[::1]:3306=db:3306":         {Listen: "[::1]:3306", Target: "db:3306"},
		"5432=192.168.1.50:5432":     {Listen: ":5432", Target: "192.168.1.50:5432"},
		"6379=redis.app.lh:6379":     {Listen: ":6379", Target: "redis.app.lh:6379"},
		"8080=[2001:db8::1]:80":      {Listen: ":8080", Target: "[2001:db8::1]:80"},
		":3306=mysql.my-app.lh:3306": {Listen: ":3306", Target: "mysql.my-app.lh:3306"},
		"0.0.0.0:3306=db.lh:3306":    {Listen: "0.0.0.0:3306", Target: "db.lh:3306"},
		"65535=db:1":                 {Listen: ":65535", Target: "db:1"},
		"localhost:1234=db.lh:3306":  {Listen: "localhost:1234", Target: "db.lh:3306"},
	}

	for in, want := range cases {
		got, err := ParseMapping(in)
		if err != nil {
			t.Errorf("ParseMapping(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseMapping(%q) = %+v, want %+v", in, got, want)
		}
	}
}

func TestParseMapping_Errors(t *testing.T) {
	t.Parallel()

	for _, in := range []string{
		"",
		"3306",
		"=db:3306",
		"3306=",
		"3306=db",
		"3306=:3306",
		"3306=db:0",
		"3306=db:70000",
		"3306=db:abc",
		"0=db:3306",
		"70000=db:3306",
		"abc=db:3306",
		"127.0.0.1=db:3306",
	} {
		if _, err := ParseMapping(in); err == nil {
			t.Errorf("ParseMapping(%q): expected error", in)
		}
	}
}

func TestMappingString(t *testing.T) {
	t.Parallel()

	if got := (Mapping{Listen: ":3306", Target: "db:3306"}).String(); got != ":3306 -> db:3306" {
		t.Errorf("got %q", got)
	}
}
