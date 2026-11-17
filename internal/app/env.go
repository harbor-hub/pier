package app

import (
	"os"
	"time"

	"github.com/spf13/cast"
)

type EnvVar struct {
	value interface{}
}

func (e *EnvVar) GetInt() int {
	return cast.ToInt(e.value)
}

func (e *EnvVar) GetBool() bool {
	return cast.ToBool(e.value)
}

func (e *EnvVar) GetString() string {
	return cast.ToString(e.value)
}

func (e *EnvVar) GetFloat64() float64 {
	return cast.ToFloat64(e.value)
}

func (e *EnvVar) GetDuration() time.Duration {
	return cast.ToDuration(e.value)
}

func LookupEnv(key string, defaultVal interface{}) *EnvVar {
	e := &EnvVar{
		value: defaultVal,
	}

	if val, ok := os.LookupEnv(key); ok {
		e.value = val
	}

	return e
}
