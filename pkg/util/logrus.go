package util

import (
	"encoding/json"

	"github.com/sirupsen/logrus"
)

// FieldsToJSON is a helper function to convert specified fields in a logrus.Fields map to JSON strings.
func FieldsToJSON(fields logrus.Fields, keys []string) logrus.Fields {
	for _, k := range keys {
		v, ok := fields[k]
		if ok {
			vBytes, err := json.Marshal(v)
			if err != nil {
				logrus.WithFields(logrus.Fields{
					"key":   k,
					"value": v,
				}).Errorf("Failed to marshall field: %v", err)
			} else {
				fields[k] = string(vBytes)
			}
		}
	}
	return fields
}
