// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package coordinator

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/labstack/echo/v4"
	"gopkg.in/yaml.v3"
)

//go:embed docs/openapi.yaml
var openapiYAMLBytes []byte

var (
	openapiJSONOnce  sync.Once
	openapiJSONBytes []byte
	openapiJSONErr   error
)

func getOpenapiJSON() ([]byte, error) {
	openapiJSONOnce.Do(func() {
		var obj interface{}
		if err := yaml.Unmarshal(openapiYAMLBytes, &obj); err != nil {
			openapiJSONErr = err
			return
		}
		openapiJSONBytes, openapiJSONErr = json.Marshal(obj)
	})
	return openapiJSONBytes, openapiJSONErr
}

func (c *Coordinator) openapiHandler(ctx echo.Context) error {
	data, err := getOpenapiJSON()
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return ctx.JSONBlob(http.StatusOK, data)
}
