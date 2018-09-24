// Code generated by MockGen. DO NOT EDIT.
// Source: network/overlay.go

// Package mock_network is a generated GoMock package.
package mock_network

import (
	context "context"
	gomock "github.com/golang/mock/gomock"
	proto "github.com/golang/protobuf/proto"
	net "net"
	reflect "reflect"
)

// MockOverlay is a mock of Overlay interface
type MockOverlay struct {
	ctrl     *gomock.Controller
	recorder *MockOverlayMockRecorder
}

// MockOverlayMockRecorder is the mock recorder for MockOverlay
type MockOverlayMockRecorder struct {
	mock *MockOverlay
}

// NewMockOverlay creates a new mock instance
func NewMockOverlay(ctrl *gomock.Controller) *MockOverlay {
	mock := &MockOverlay{ctrl: ctrl}
	mock.recorder = &MockOverlayMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockOverlay) EXPECT() *MockOverlayMockRecorder {
	return m.recorder
}

// Start mocks base method
func (m *MockOverlay) Start(arg0 context.Context) error {
	ret := m.ctrl.Call(m, "Start", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// Start indicates an expected call of Start
func (mr *MockOverlayMockRecorder) Start(arg0 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Start", reflect.TypeOf((*MockOverlay)(nil).Start), arg0)
}

// Stop mocks base method
func (m *MockOverlay) Stop(arg0 context.Context) error {
	ret := m.ctrl.Call(m, "Stop", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// Stop indicates an expected call of Stop
func (mr *MockOverlayMockRecorder) Stop(arg0 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Stop", reflect.TypeOf((*MockOverlay)(nil).Stop), arg0)
}

// Broadcast mocks base method
func (m *MockOverlay) Broadcast(arg0 uint32, arg1 proto.Message) error {
	ret := m.ctrl.Call(m, "Broadcast", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// Broadcast indicates an expected call of Broadcast
func (mr *MockOverlayMockRecorder) Broadcast(arg0, arg1 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Broadcast", reflect.TypeOf((*MockOverlay)(nil).Broadcast), arg0, arg1)
}

// Tell mocks base method
func (m *MockOverlay) Tell(arg0 uint32, arg1 net.Addr, arg2 proto.Message) error {
	ret := m.ctrl.Call(m, "Tell", arg0, arg1, arg2)
	ret0, _ := ret[0].(error)
	return ret0
}

// Tell indicates an expected call of Tell
func (mr *MockOverlayMockRecorder) Tell(arg0, arg1, arg2 interface{}) *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Tell", reflect.TypeOf((*MockOverlay)(nil).Tell), arg0, arg1, arg2)
}

// Self mocks base method
func (m *MockOverlay) Self() net.Addr {
	ret := m.ctrl.Call(m, "Self")
	ret0, _ := ret[0].(net.Addr)
	return ret0
}

// Self indicates an expected call of Self
func (mr *MockOverlayMockRecorder) Self() *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Self", reflect.TypeOf((*MockOverlay)(nil).Self))
}

// GetPeers mocks base method
func (m *MockOverlay) GetPeers() []net.Addr {
	ret := m.ctrl.Call(m, "GetPeers")
	ret0, _ := ret[0].([]net.Addr)
	return ret0
}

// GetPeers indicates an expected call of GetPeers
func (mr *MockOverlayMockRecorder) GetPeers() *gomock.Call {
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetPeers", reflect.TypeOf((*MockOverlay)(nil).GetPeers))
}
