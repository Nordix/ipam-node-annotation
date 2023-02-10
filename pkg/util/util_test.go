package util

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

type CniErrorData struct {
	CniVersion string `json:"cniVersion"`
	Code       int    `json:"code"`
	Message    string `json:"msg"`
	Details    string `json:"details"`
}

func TestCniError(t *testing.T) {
	CniVersion = "0.4.0"
	err := fmt.Errorf("ERROR: Testing")
	cniErr := CniError(context.TODO(), err, 7, "Something bad")
	if !json.Valid([]byte(cniErr)) {
		t.Fatal("Not valid json:", cniErr)
	}
	var cniErrorData CniErrorData
	err = json.Unmarshal([]byte(cniErr), &cniErrorData)
	if err != nil {
		t.Fatal("Unmarshal", err)
	}
	//fmt.Println(cniErr)
}
