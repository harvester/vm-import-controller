package util

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestLogJSON_success(t *testing.T) {
	type myStruct struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	assert := require.New(t)
	fields := logrus.Fields{
		"key1": "value1",
		"key2": myStruct{ID: 1, Name: "foo"},
		"key3": 4815162342,
	}

	FieldsToJSON(fields, []string{"key2"})
	assert.Equal(fields["key1"], "value1", "expected value not to modified")
	assert.JSONEq(fields["key2"].(string), `{"id":1,"name":"foo"}`, "expected value to be marshalled to JSON")
	assert.Equal(fields["key3"], 4815162342, "expected value not to modified")
}

func TestLogJSON_fail(t *testing.T) {
	type myStruct struct {
		Name   string
		Secret chan int
	}
	assert := require.New(t)
	fields := logrus.Fields{
		"key1": myStruct{Name: "John", Secret: make(chan int)}, // this will fail to marshal
		"key2": "value2",
	}

	FieldsToJSON(fields, []string{"key1"})
	assert.IsType(fields["key1"], myStruct{}, "expected value not to marshalled to JSON")
}
