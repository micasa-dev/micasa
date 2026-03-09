// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package data

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type defaultsTestStruct struct {
	Name     string  `default:"unnamed"`
	Count    int     `default:"42"`
	BigCount int64   `default:"100"`
	ID       uint    `default:"1"`
	Rate     float64 `default:"3.14"`
	NoTag    string
}

func TestApplyDefaults_setsZeroFields(t *testing.T) {
	v := &defaultsTestStruct{}
	ApplyDefaults(v)

	assert.Equal(t, "unnamed", v.Name)
	assert.Equal(t, 42, v.Count)
	assert.Equal(t, int64(100), v.BigCount)
	assert.Equal(t, uint(1), v.ID)
	assert.InDelta(t, 3.14, v.Rate, 0.001)
	assert.Empty(t, v.NoTag)
}

func TestApplyDefaults_preservesNonZeroFields(t *testing.T) {
	v := &defaultsTestStruct{
		Name:  "custom",
		Count: 7,
	}
	ApplyDefaults(v)

	assert.Equal(t, "custom", v.Name)
	assert.Equal(t, 7, v.Count)
	assert.Equal(t, int64(100), v.BigCount)
}

type defaultsTimeStruct struct {
	When    time.Time `default:"now"`
	DateStr string    `default:"today"`
}

func TestApplyDefaults_timeNow(t *testing.T) {
	before := time.Now().Add(-time.Second)
	v := &defaultsTimeStruct{}
	ApplyDefaults(v)
	after := time.Now().Add(time.Second)

	assert.False(t, v.When.IsZero())
	assert.True(t, v.When.After(before))
	assert.True(t, v.When.Before(after))
}

func TestApplyDefaults_todayString(t *testing.T) {
	v := &defaultsTimeStruct{}
	ApplyDefaults(v)

	expected := time.Now().Format(DateLayout)
	assert.Equal(t, expected, v.DateStr)
}

func TestApplyDefaults_preservesExistingTime(t *testing.T) {
	fixed := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	v := &defaultsTimeStruct{When: fixed}
	ApplyDefaults(v)

	assert.Equal(t, fixed, v.When)
}

type customInt int

type defaultsCustomIntStruct struct {
	Kind customInt `default:"2"`
}

func TestApplyDefaults_customIntType(t *testing.T) {
	v := &defaultsCustomIntStruct{}
	ApplyDefaults(v)

	assert.Equal(t, customInt(2), v.Kind)
}

func TestApplyDefaults_nonPointerIsNoop(t *testing.T) {
	v := defaultsTestStruct{}
	ApplyDefaults(v) // should not panic
	assert.Empty(t, v.Name)
}

func TestApplyDefaults_nilIsNoop(_ *testing.T) {
	ApplyDefaults(nil) // should not panic
}

type nestedInner struct {
	Host string `default:"localhost"`
	Port int    `default:"8080"`
}

type nestedOuter struct {
	Name  string `default:"app"`
	Inner nestedInner
}

func TestApplyDefaults_recursesIntoNestedStructs(t *testing.T) {
	v := &nestedOuter{}
	ApplyDefaults(v)

	assert.Equal(t, "app", v.Name)
	assert.Equal(t, "localhost", v.Inner.Host)
	assert.Equal(t, 8080, v.Inner.Port)
}

func TestApplyDefaults_preservesNestedNonZeroFields(t *testing.T) {
	v := &nestedOuter{
		Inner: nestedInner{Port: 9090},
	}
	ApplyDefaults(v)

	assert.Equal(t, "app", v.Name)
	assert.Equal(t, "localhost", v.Inner.Host)
	assert.Equal(t, 9090, v.Inner.Port)
}

func TestStructDefault(t *testing.T) {
	got := StructDefault[Incident]("Status")
	require.Equal(t, IncidentStatusOpen, got)
}

func TestStructDefault_missingField(t *testing.T) {
	got := StructDefault[Incident]("NonExistent")
	assert.Empty(t, got)
}

func TestStructDefault_noTag(t *testing.T) {
	got := StructDefault[Incident]("Title")
	assert.Empty(t, got)
}
