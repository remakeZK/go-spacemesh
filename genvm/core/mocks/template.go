// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/spacemeshos/go-spacemesh/genvm/core (interfaces: Template)

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	scale "github.com/spacemeshos/go-scale"
	core "github.com/spacemeshos/go-spacemesh/genvm/core"
)

// MockTemplate is a mock of Template interface.
type MockTemplate struct {
	ctrl     *gomock.Controller
	recorder *MockTemplateMockRecorder
}

// MockTemplateMockRecorder is the mock recorder for MockTemplate.
type MockTemplateMockRecorder struct {
	mock *MockTemplate
}

// NewMockTemplate creates a new mock instance.
func NewMockTemplate(ctrl *gomock.Controller) *MockTemplate {
	mock := &MockTemplate{ctrl: ctrl}
	mock.recorder = &MockTemplateMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockTemplate) EXPECT() *MockTemplateMockRecorder {
	return m.recorder
}

// EncodeScale mocks base method.
func (m *MockTemplate) EncodeScale(arg0 *scale.Encoder) (int, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "EncodeScale", arg0)
	ret0, _ := ret[0].(int)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// EncodeScale indicates an expected call of EncodeScale.
func (mr *MockTemplateMockRecorder) EncodeScale(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "EncodeScale", reflect.TypeOf((*MockTemplate)(nil).EncodeScale), arg0)
}

// MaxSpend mocks base method.
func (m *MockTemplate) MaxSpend(arg0 uint16, arg1 interface{}) (uint64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "MaxSpend", arg0, arg1)
	ret0, _ := ret[0].(uint64)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// MaxSpend indicates an expected call of MaxSpend.
func (mr *MockTemplateMockRecorder) MaxSpend(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "MaxSpend", reflect.TypeOf((*MockTemplate)(nil).MaxSpend), arg0, arg1)
}

// Verify mocks base method.
func (m *MockTemplate) Verify(arg0 core.Host, arg1 []byte, arg2 *scale.Decoder) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Verify", arg0, arg1, arg2)
	ret0, _ := ret[0].(bool)
	return ret0
}

// Verify indicates an expected call of Verify.
func (mr *MockTemplateMockRecorder) Verify(arg0, arg1, arg2 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Verify", reflect.TypeOf((*MockTemplate)(nil).Verify), arg0, arg1, arg2)
}
