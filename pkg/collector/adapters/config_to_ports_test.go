// Copyright The OpenTelemetry Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package adapters_test

import (
	"errors"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/open-telemetry/opentelemetry-operator/pkg/collector/adapters"
	"github.com/open-telemetry/opentelemetry-operator/pkg/collector/parser"
)

var logger = logf.Log.WithName("unit-tests")

func TestExtractPortsFromConfig(t *testing.T) {
	configStr := `receivers:
  examplereceiver:
    endpoint: "0.0.0.0:12345"
  examplereceiver/settings:
    endpoint: "0.0.0.0:12346"
  examplereceiver/invalid-ignored:
    endpoint: "0.0.0.0"
  examplereceiver/invalid-not-number:
    endpoint: "0.0.0.0:not-number"
  examplereceiver/without-endpoint:
    notendpoint: "0.0.0.0:12347"
  jaeger:
    protocols:
      grpc:
      thrift_compact:
      thrift_binary:
        endpoint: 0.0.0.0:6833
  jaeger/custom:
    protocols:
      thrift_http:
        endpoint: 0.0.0.0:15268
  otlp:
    protocols:
      grpc:
      http:
  otlp/2:
    protocols:
      grpc:
        endpoint: 0.0.0.0:55555
  zipkin:
  zipkin/2:
    endpoint: 0.0.0.0:33333
service:
  pipelines:
    metrics:
      receivers: [examplereceiver, examplereceiver/settings]
      exporters: [logging]
    metrics/1:
      receivers: [jaeger, jaeger/custom]
      exporters: [logging]
    metrics/2:
      receivers: [otlp, otlp/2, zipkin]
      exporters: [logging]
`

	// prepare
	config, err := adapters.ConfigFromString(configStr)
	require.NoError(t, err)
	require.NotEmpty(t, config)

	// test
	ports, err := adapters.ConfigToReceiverPorts(logger, config)
	assert.NoError(t, err)
	assert.Len(t, ports, 11)

	// verify
	httpAppProtocol := "http"
	grpcAppProtocol := "grpc"
	targetPortZero := intstr.IntOrString{Type: 0, IntVal: 0, StrVal: ""}
	targetPort4317 := intstr.IntOrString{Type: 0, IntVal: 4317, StrVal: ""}
	targetPort4318 := intstr.IntOrString{Type: 0, IntVal: 4318, StrVal: ""}

	assert.Len(t, ports, 11)
	assert.Equal(t, corev1.ServicePort{Name: "examplereceiver", Port: int32(12345)}, ports[0])
	assert.Equal(t, corev1.ServicePort{Name: "examplereceiver-settings", Port: int32(12346)}, ports[1])
	assert.Equal(t, corev1.ServicePort{Name: "jaeger-custom-thrift-http", AppProtocol: &httpAppProtocol, Protocol: "TCP", Port: int32(15268), TargetPort: targetPortZero}, ports[2])
	assert.Equal(t, corev1.ServicePort{Name: "jaeger-grpc", AppProtocol: &grpcAppProtocol, Protocol: "TCP", Port: int32(14250)}, ports[3])
	assert.Equal(t, corev1.ServicePort{Name: "jaeger-thrift-binary", Protocol: "UDP", Port: int32(6833)}, ports[4])
	assert.Equal(t, corev1.ServicePort{Name: "jaeger-thrift-compact", Protocol: "UDP", Port: int32(6831)}, ports[5])
	assert.Equal(t, corev1.ServicePort{Name: "otlp-2-grpc", AppProtocol: &grpcAppProtocol, Protocol: "TCP", Port: int32(55555)}, ports[6])
	assert.Equal(t, corev1.ServicePort{Name: "otlp-grpc", AppProtocol: &grpcAppProtocol, Port: int32(4317), TargetPort: targetPort4317}, ports[7])
	assert.Equal(t, corev1.ServicePort{Name: "otlp-http", AppProtocol: &httpAppProtocol, Port: int32(4318), TargetPort: targetPort4318}, ports[8])
	assert.Equal(t, corev1.ServicePort{Name: "otlp-http-legacy", AppProtocol: &httpAppProtocol, Port: int32(55681), TargetPort: targetPort4318}, ports[9])
	assert.Equal(t, corev1.ServicePort{Name: "zipkin", AppProtocol: &httpAppProtocol, Protocol: "TCP", Port: int32(9411)}, ports[10])
}

func TestNoPortsParsed(t *testing.T) {
	for _, tt := range []struct {
		expected  error
		desc      string
		configStr string
	}{
		{
			expected:  adapters.ErrNoReceivers,
			desc:      "empty",
			configStr: "",
		},
		{
			expected:  adapters.ErrReceiversNotAMap,
			desc:      "not a map",
			configStr: "receivers: some-string",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			// prepare
			config, err := adapters.ConfigFromString(tt.configStr)
			require.NoError(t, err)

			// test
			ports, err := adapters.ConfigToReceiverPorts(logger, config)

			// verify
			assert.Nil(t, ports)
			assert.Equal(t, tt.expected, err)
		})
	}
}

func TestInvalidReceivers(t *testing.T) {
	for _, tt := range []struct {
		desc      string
		configStr string
	}{
		{
			"receiver isn't a map",
			"receivers:\n  some-receiver: string\nservice:\n  pipelines:\n    metrics:\n      receivers: [some-receiver]",
		},
		{
			"receiver's endpoint isn't string",
			"receivers:\n  some-receiver:\n    endpoint: 123\nservice:\n  pipelines:\n    metrics:\n      receivers: [some-receiver]",
		},
	} {
		t.Run(tt.desc, func(t *testing.T) {
			// prepare
			config, err := adapters.ConfigFromString(tt.configStr)
			require.NoError(t, err)

			// test
			ports, err := adapters.ConfigToReceiverPorts(logger, config)

			// verify
			assert.NoError(t, err)
			assert.Len(t, ports, 0)
		})
	}
}

func TestParserFailed(t *testing.T) {
	// prepare
	mockParserCalled := false
	mockParser := &mockParser{
		portsFunc: func() ([]corev1.ServicePort, error) {
			mockParserCalled = true
			return nil, errors.New("mocked error")
		},
	}
	parser.Register("mock", func(logger logr.Logger, name string, config map[interface{}]interface{}) parser.ReceiverParser {
		return mockParser
	})

	config := map[interface{}]interface{}{
		"receivers": map[interface{}]interface{}{
			"mock": map[string]interface{}{},
		},
		"service": map[interface{}]interface{}{
			"pipelines": map[interface{}]interface{}{
				"metrics": map[interface{}]interface{}{
					"receivers": []interface{}{"mock"},
				},
			},
		},
	}

	// test
	ports, err := adapters.ConfigToReceiverPorts(logger, config)

	// verify
	assert.Len(t, ports, 0)
	assert.NoError(t, err)
	assert.True(t, mockParserCalled)
}

type mockParser struct {
	portsFunc func() ([]corev1.ServicePort, error)
}

func (m *mockParser) Ports() ([]corev1.ServicePort, error) {
	if m.portsFunc != nil {
		return m.portsFunc()
	}

	return nil, nil
}

func (m *mockParser) ParserName() string {
	return "__mock-adapters"
}
