package app

import (
	"testing"
)

// ---------------------------------------------------------------------------
// LookupEnv – default value used when env var is absent
// ---------------------------------------------------------------------------

func TestLookupEnv_DefaultString(t *testing.T) {
	t.Parallel()
	e := LookupEnv("PIER_TEST_ABSENT_STRING", "hello")
	if e.GetString() != "hello" {
		t.Errorf("expected hello, got %q", e.GetString())
	}
}

func TestLookupEnv_DefaultInt(t *testing.T) {
	t.Parallel()
	e := LookupEnv("PIER_TEST_ABSENT_INT", 42)
	if e.GetInt() != 42 {
		t.Errorf("expected 42, got %d", e.GetInt())
	}
}

func TestLookupEnv_DefaultBool(t *testing.T) {
	t.Parallel()
	e := LookupEnv("PIER_TEST_ABSENT_BOOL", true)
	if !e.GetBool() {
		t.Error("expected true")
	}
}

func TestLookupEnv_DefaultFloat64(t *testing.T) {
	t.Parallel()
	e := LookupEnv("PIER_TEST_ABSENT_FLOAT", 3.14)
	if e.GetFloat64() != 3.14 {
		t.Errorf("expected 3.14, got %f", e.GetFloat64())
	}
}

// ---------------------------------------------------------------------------
// LookupEnv – env var present overrides default
// ---------------------------------------------------------------------------

func TestLookupEnv_EnvOverridesDefault(t *testing.T) {
	t.Setenv("PIER_TEST_STR", "fromenv")
	e := LookupEnv("PIER_TEST_STR", "default")
	if e.GetString() != "fromenv" {
		t.Errorf("expected fromenv, got %q", e.GetString())
	}
}

func TestLookupEnv_EnvIntAsString(t *testing.T) {
	t.Setenv("PIER_TEST_INT", "99")
	e := LookupEnv("PIER_TEST_INT", 0)
	if e.GetInt() != 99 {
		t.Errorf("expected 99, got %d", e.GetInt())
	}
}

func TestLookupEnv_EnvBoolTrue(t *testing.T) {
	t.Setenv("PIER_TEST_BOOL", "true")
	e := LookupEnv("PIER_TEST_BOOL", false)
	if !e.GetBool() {
		t.Error("expected true from env")
	}
}

func TestLookupEnv_EnvBoolFalse(t *testing.T) {
	t.Setenv("PIER_TEST_BOOL_F", "false")
	e := LookupEnv("PIER_TEST_BOOL_F", true)
	if e.GetBool() {
		t.Error("expected false from env")
	}
}

func TestLookupEnv_EnvFloat64(t *testing.T) {
	t.Setenv("PIER_TEST_FLOAT", "2.71")
	e := LookupEnv("PIER_TEST_FLOAT", 0.0)
	if e.GetFloat64() != 2.71 {
		t.Errorf("expected 2.71, got %f", e.GetFloat64())
	}
}

// ---------------------------------------------------------------------------
// LookupEnv – empty string env var overrides default
// ---------------------------------------------------------------------------

func TestLookupEnv_EmptyEnvOverridesDefault(t *testing.T) {
	t.Setenv("PIER_TEST_EMPTY", "")
	e := LookupEnv("PIER_TEST_EMPTY", "default")
	if e.GetString() != "" {
		t.Errorf("expected empty string, got %q", e.GetString())
	}
}

// ---------------------------------------------------------------------------
// GetString – type coercions from default value
// ---------------------------------------------------------------------------

func TestGetString_FromInt(t *testing.T) {
	t.Parallel()
	e := LookupEnv("PIER_TEST_ABSENT_X1", 123)
	if e.GetString() != "123" {
		t.Errorf("expected '123', got %q", e.GetString())
	}
}

func TestGetString_FromBool(t *testing.T) {
	t.Parallel()
	e := LookupEnv("PIER_TEST_ABSENT_X2", false)
	if e.GetString() != "false" {
		t.Errorf("expected 'false', got %q", e.GetString())
	}
}

// ---------------------------------------------------------------------------
// GetInt – type coercions
// ---------------------------------------------------------------------------

func TestGetInt_FromStringDefault(t *testing.T) {
	t.Parallel()
	e := LookupEnv("PIER_TEST_ABSENT_X3", "7")
	if e.GetInt() != 7 {
		t.Errorf("expected 7, got %d", e.GetInt())
	}
}

func TestGetInt_FromInvalidString(t *testing.T) {
	t.Parallel()
	e := LookupEnv("PIER_TEST_ABSENT_X4", "notanumber")
	// cast returns 0 for unparseable strings
	if e.GetInt() != 0 {
		t.Errorf("expected 0 for invalid int, got %d", e.GetInt())
	}
}

// ---------------------------------------------------------------------------
// GetBool – type coercions
// ---------------------------------------------------------------------------

func TestGetBool_From1(t *testing.T) {
	t.Parallel()
	e := LookupEnv("PIER_TEST_ABSENT_X5", "1")
	if !e.GetBool() {
		t.Error("expected true for '1'")
	}
}

func TestGetBool_From0(t *testing.T) {
	t.Parallel()
	e := LookupEnv("PIER_TEST_ABSENT_X6", "0")
	if e.GetBool() {
		t.Error("expected false for '0'")
	}
}
